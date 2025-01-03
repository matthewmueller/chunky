package chunky

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os/user"
	"path"
	"time"

	"github.com/matthewmueller/chunky/internal/caches"
	"github.com/matthewmueller/chunky/internal/chunkyignore"
	"github.com/matthewmueller/chunky/internal/commits"
	"github.com/matthewmueller/chunky/internal/packs"
	"github.com/matthewmueller/chunky/internal/sha256"
	"github.com/matthewmueller/chunky/repos"
	"golang.org/x/sync/errgroup"
)

type Upload struct {
	From     fs.FS
	To       repos.Repo
	Cache    repos.FS
	User     string
	Tags     []string
	Ignore   func(path string) bool
	ReadFile func(path string) ([]byte, error)
}

func (in *Upload) validate() (err error) {
	// Required fields
	if in.From == nil {
		err = errors.Join(err, errors.New("missing from filesystem"))
	}
	if in.To == nil {
		err = errors.Join(err, errors.New("missing to repository"))
	}
	if in.Cache == nil {
		err = errors.Join(err, errors.New("missing cache"))
	}

	// Default to the current user
	if in.User == "" {
		user, err := user.Current()
		if err != nil {
			return errors.Join(err, fmt.Errorf("missing user and getting current user failed with: %w", err))
		}
		in.User = user.Username
	}

	// Validate the tags
	for _, tag := range in.Tags {
		if tag == "latest" {
			err = errors.Join(err, errors.New("tag cannot be 'latest'"))
		} else if tag == "" {
			err = errors.Join(err, errors.New("tag cannot be empty"))
		}
	}

	// Default to the .chunkyignore file
	if in.Ignore == nil {
		in.Ignore = chunkyignore.FromFS(in.From)
	}

	// Default to reading files from the 'from' filesystem
	if in.ReadFile == nil {
		in.ReadFile = func(path string) ([]byte, error) {
			return fs.ReadFile(in.From, path)
		}
	}

	return err
}

// Upload a directory to a repository
func (c *Client) Upload(ctx context.Context, in *Upload) error {
	if err := in.validate(); err != nil {
		return err
	}

	// Download the latest commits from the cache
	cache, err := caches.Download(ctx, in.To, in.Cache)
	if err != nil {
		return err
	}

	ignore := in.Ignore
	createdAt := time.Now().UTC()
	commit := commits.New(in.User, createdAt)
	commitId := commit.ID()
	pack := packs.New()

	// Walk over the files, chunk them and add them to the file system we're going
	// to upload. We'll also add each file to the commit object.
	if err := fs.WalkDir(in.From, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		} else if d.IsDir() || ignore(path) {
			return nil
		}

		file, err := in.From.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		data, err := in.ReadFile(path)
		if err != nil {
			return err
		}
		fileHash := sha256.Hash(data)

		info, err := file.Stat()
		if err != nil {
			return err
		}

		// Check if the file is already in the pack
		// TODO: right now this will duplicate content when the file path in the
		// pack is different from the file path in the commit. We should add a
		// way to alias files in the pack to other packs.
		if commitFile, ok := cache.Get(fileHash); ok && commitFile.Path == path {
			commit.Add(&commits.File{
				Path:   path,
				Size:   uint64(len(data)),
				Id:     fileHash,
				PackId: commitFile.PackId,
			})
			return nil
		}

		entry := &repos.File{
			Path:    path,
			Mode:    info.Mode(),
			ModTime: info.ModTime(),
			Data:    data,
		}

		// Add the entry to the pack
		if err := pack.Add(entry); err != nil {
			return err
		}

		// Add the file to the commit
		commit.Add(&commits.File{
			Path:   path,
			Id:     fileHash,
			PackId: commitId,
			Size:   uint64(info.Size()),
		})

		return nil
	}); err != nil {
		return err
	}

	// pack file + commit file + latest ref + tags
	capacity := 1 + 1 + 1 + len(in.Tags)
	fromCh := make(chan *repos.File, capacity)

	eg := new(errgroup.Group)
	eg.Go(func() error {
		return in.To.Upload(ctx, fromCh)
	})

	// Add the pack to the tree
	packData, err := pack.Pack()
	if err != nil {
		close(fromCh)
		return err
	}
	if len(packData) > 0 {
		fromCh <- &repos.File{
			Path:    path.Join("packs", commitId),
			Data:    packData,
			Mode:    0644,
			ModTime: createdAt,
		}
	}

	// Add the commit to the tree
	commitData, err := commit.Pack()
	if err != nil {
		close(fromCh)
		return err
	}
	fromCh <- &repos.File{
		Path:    path.Join("commits", commitId),
		Data:    commitData,
		Mode:    0644,
		ModTime: createdAt,
	}

	// Add the commit to the cache
	if err := cache.Set(commitId, commit); err != nil {
		close(fromCh)
		return err
	}

	// Add the latest ref
	fromCh <- &repos.File{
		Path: path.Join("tags", "latest"),
		Data: []byte(commitId),
		Mode: 0644,
	}

	// Tag the revision
	for _, tag := range in.Tags {
		fromCh <- &repos.File{
			Path: fmt.Sprintf("tags/%s", tag),
			Data: []byte(commitId),
			Mode: 0644,
		}
	}

	close(fromCh)
	return eg.Wait()
}
