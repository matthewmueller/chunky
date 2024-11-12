package caches

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/matthewmueller/chunky/internal/commits"
	"github.com/matthewmueller/chunky/repos"
	"github.com/matthewmueller/text"
	"github.com/matthewmueller/virt"
)

// Directory returns the cache directory for a repo
func Directory(repo repos.Repo) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, "chunky", text.Slug(repo.Key())), nil
}

// Download the cache to the local filesystem
func Download(ctx context.Context, from repos.Repo, to virt.FS) (*Local, error) {
	cache, err := Load(to)
	if err != nil {
		return nil, err
	}
	if err := cache.Download(ctx, from); err != nil {
		return nil, err
	}
	return cache, nil
}

func Load(fsys virt.FS) (*Local, error) {
	cache := &Local{
		fsys,
		map[string]*commits.File{},
		map[string]*commits.Commit{},
	}

	// Load the cache
	des, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	for _, de := range des {
		if de.IsDir() {
			continue
		}
		name := de.Name()
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		commit, err := commits.Unpack(data)
		if err != nil {
			if isCacheInvalid(err) {
				// Remove the invalid cache file
				if err := fsys.RemoveAll(name); err != nil {
					return nil, err
				}
				continue
			}
			return nil, err
		}
		for _, file := range commit.Files() {
			cache.files[file.Id] = file
		}
		cache.commits[name] = commit
	}

	return cache, nil
}

func isCacheInvalid(err error) bool {
	if err == nil {
		return false
	}
	return strings.HasPrefix(err.Error(), "gob: ")
}

type Local struct {
	fsys    virt.FS
	files   map[string]*commits.File   // file_hash -> pack_file
	commits map[string]*commits.Commit // commit_id -> commit
}

var _ Cache = (*Local)(nil)

// Download the latest commits
func (c *Local) Download(ctx context.Context, repo repos.Repo) error {
	seen := map[string]bool{}
	for commitId := range c.commits {
		seen[commitId] = false
	}
	if err := repo.Walk(ctx, "commits", func(fpath string, de fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return fs.SkipAll
			}
			return err
		} else if de.IsDir() {
			return nil
		}

		commitId := filepath.Base(fpath)
		seen[commitId] = true

		// Skip if we already have the commit
		if _, ok := c.commits[commitId]; ok {
			return nil
		}

		// Download the commit
		commitFile, err := repos.Download(ctx, repo, fpath)
		if err != nil {
			return err
		}

		// Unpack the commit
		commit, err := commits.Unpack(commitFile.Data)
		if err != nil {
			return err
		}

		// Write the commit to the cache
		if err := c.fsys.WriteFile(commitId, commitFile.Data, 0644); err != nil {
			return err
		}

		// Add the files to the cache
		for _, file := range commit.Files() {
			c.files[file.Id] = file
		}

		// Mark the commit as downloaded
		c.commits[commitId] = commit

		return nil
	}); err != nil {
		return err
	}

	// Remove any commits that are no longer in the repo
	for commitId, ok := range seen {
		if ok {
			continue
		}
		if err := c.fsys.RemoveAll(commitId); err != nil {
			return err
		}
		commit := c.commits[commitId]
		for _, file := range commit.Files() {
			delete(c.files, file.Id)
		}
		delete(c.commits, commitId)
	}

	return nil
}

func (c *Local) Get(fileId string) (file *commits.File, ok bool) {
	file, ok = c.files[fileId]
	return file, ok
}

func (c *Local) Set(commitId string, commit *commits.Commit) error {
	// Skip if we already have the commit
	if _, ok := c.commits[commitId]; ok {
		return nil
	}

	// Pack the commit
	data, err := commit.Pack()
	if err != nil {
		return err
	}

	// Write the commit to the cache
	if err := c.fsys.WriteFile(commitId, data, 0644); err != nil {
		return err
	}

	// Add the files to the cache
	for _, file := range commit.Files() {
		c.files[file.Id] = file
	}
	c.commits[commitId] = commit
	return nil
}
