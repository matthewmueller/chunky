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
	"github.com/matthewmueller/chunky/internal/sha256"
	"github.com/matthewmueller/chunky/internal/uploads"
	"github.com/matthewmueller/chunky/repos"
	"golang.org/x/sync/errgroup"
)

const kiB = 1024
const miB = 1024 * kiB

type Upload struct {
	From         fs.FS
	To           repos.Repo
	Cache        repos.FS
	User         string
	Tags         []string
	Ignore       func(path string) bool
	MaxPackSize  int64
	MinChunkSize int64
	MaxChunkSize int64
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

	if in.MaxPackSize < 0 {
		err = errors.Join(err, errors.New("max pack size cannot be negative"))
	} else if in.MaxPackSize == 0 {
		in.MaxPackSize = 32 * miB
	}
	if in.MinChunkSize < 0 {
		err = errors.Join(err, errors.New("min chunk size cannot be negative"))
	} else if in.MinChunkSize == 0 {
		in.MinChunkSize = 512 * kiB
	}
	if in.MaxChunkSize < 0 {
		err = errors.Join(err, errors.New("max chunk size cannot be negative"))
	} else if in.MaxChunkSize == 0 {
		in.MaxChunkSize = 8 * miB
	}
	if in.MinChunkSize > in.MaxChunkSize {
		err = errors.Join(err, errors.New("min chunk size cannot be greater than max chunk size"))
	}
	if in.MaxPackSize < in.MaxChunkSize {
		err = errors.Join(err, errors.New("max pack size cannot be less than max chunk size"))
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

	uploadCh := make(chan *repos.File)
	eg := new(errgroup.Group)
	eg.Go(func() error {
		return in.To.Upload(ctx, uploadCh)
	})

	upload := uploads.New(uploadCh, in.MaxPackSize, in.MinChunkSize, in.MaxChunkSize)

	// Walk over the files, chunk them and add them to the file system we're going
	// to upload. We'll also add each file to the commit object.
	if err := fs.WalkDir(in.From, ".", func(fpath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		} else if d.IsDir() || ignore(fpath) {
			return nil
		}

		// Hash the file into a sha256 hash, this reads the file in chunks, rather
		// than loading the entire file into memory.
		fileHash, err := sha256.HashFile(in.From, fpath, in.MaxChunkSize)
		if err != nil {
			return fmt.Errorf("unable to hash file %q: %w", fpath, err)
		}

		// Check if the file is already in the pack. This will duplicate content
		// if the file path in the pack is different from the file path in the
		// commit. To fix this, we also ensure the file paths are the same.
		// TODO: We should add a way to alias files in the pack to other packs.
		if commitFile, ok := cache.Get(fpath, fileHash); ok {
			commit.Add(commitFile)
			return nil
		}

		// Open the file
		file, err := in.From.Open(fpath)
		if err != nil {
			return err
		}
		defer file.Close()

		// Get the file info
		info, err := file.Stat()
		if err != nil {
			return err
		}

		// Add the file to the pack
		packId, err := upload.Add(&uploads.File{
			Reader:  file,
			Path:    fpath,
			Hash:    fileHash,
			Mode:    info.Mode(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
		if err != nil {
			return err
		}

		// Add the file to the commit
		commit.Add(&commits.File{
			Path:   fpath,
			Id:     fileHash,
			PackId: packId,
			Size:   uint64(info.Size()),
		})

		return nil
	}); err != nil {
		close(uploadCh)
		return err
	}

	// Upload any remaining files in the pack
	if err := upload.Flush(); err != nil {
		close(uploadCh)
		return err
	}

	// Add the commit to the tree
	commitData, err := commit.Pack()
	if err != nil {
		close(uploadCh)
		return err
	}
	uploadCh <- &repos.File{
		Path:    path.Join("commits", commitId),
		Data:    commitData,
		Mode:    0644,
		ModTime: createdAt,
	}

	// Add the commit to the cache
	if err := cache.Set(commitId, commit); err != nil {
		close(uploadCh)
		return err
	}

	// Add the latest ref
	uploadCh <- &repos.File{
		Path: path.Join("tags", "latest"),
		Data: []byte(commitId),
		Mode: 0644,
	}

	// Tag the revision
	for _, tag := range in.Tags {
		uploadCh <- &repos.File{
			Path: fmt.Sprintf("tags/%s", tag),
			Data: []byte(commitId),
			Mode: 0644,
		}
	}

	close(uploadCh)
	return eg.Wait()
}
