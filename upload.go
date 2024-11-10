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
	"github.com/matthewmueller/chunky/internal/repos"
	"github.com/matthewmueller/chunky/internal/sha256"
	"github.com/matthewmueller/virt"
)

type Upload struct {
	From  fs.FS
	To    repos.Repo
	Cache caches.Cache
	User  string
	Tags  []string
}

func (u *Upload) validate() (err error) {
	// Required fields
	if u.From == nil {
		err = errors.Join(err, errors.New("missing from filesystem"))
	}
	if u.To == nil {
		err = errors.Join(err, errors.New("missing to repository"))
	}

	// Default to the current user
	if u.User == "" {
		user, err := user.Current()
		if err != nil {
			return errors.Join(err, fmt.Errorf("missing user and getting current user failed with: %w", err))
		}
		u.User = user.Username
	}

	// Default the cache to None
	if u.Cache == nil {
		u.Cache = caches.None
	}

	// Validate the tags
	for _, tag := range u.Tags {
		if tag == "latest" {
			err = errors.Join(err, errors.New("tag cannot be 'latest'"))
		} else if tag == "previous" {
			err = errors.Join(err, errors.New("tag cannot be 'previous'"))
		} else if tag == "" {
			err = errors.Join(err, errors.New("tag cannot be empty"))
		}
	}

	return err
}

// Upload a directory to a repository
func (c *Client) Upload(ctx context.Context, in *Upload) error {
	if err := in.validate(); err != nil {
		return err
	}

	ignore := chunkyignore.FromFS(in.From)
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

		data, err := fs.ReadFile(in.From, path)
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
		if commitFile, ok := in.Cache.Get(fileHash); ok && commitFile.Path == path {
			commit.Add(&commits.File{
				Path:   path,
				Size:   uint64(info.Size()),
				Id:     fileHash,
				PackId: commitFile.PackId,
			})
			return nil
		}

		entry := &packs.File{
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

	tree := virt.Tree{}

	// Add the pack to the tree
	packData, err := pack.Pack()
	if err != nil {
		return err
	}
	if len(packData) > 0 {
		tree[path.Join("packs", commitId)] = &virt.File{
			Data:    packData,
			Mode:    0644,
			ModTime: createdAt,
		}
	}

	// Add the commit to the tree
	commitData, err := commit.Pack()
	if err != nil {
		return err
	}
	tree[path.Join("commits", commitId)] = &virt.File{
		Data:    commitData,
		Mode:    0644,
		ModTime: createdAt,
	}

	// Add the commit to the cache
	if err := in.Cache.Set(commitId, commit); err != nil {
		return err
	}

	// Add the latest ref
	tree["tags/latest"] = &virt.File{
		Data: []byte(commitId),
		Mode: 0644,
	}

	// Tag the revision
	for _, tag := range in.Tags {
		tree[fmt.Sprintf("tags/%s", tag)] = &virt.File{
			Data: []byte(commitId),
			Mode: 0644,
		}
	}

	return in.To.Upload(ctx, tree)
}
