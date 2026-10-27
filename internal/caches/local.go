package caches

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/matthewmueller/chunky/internal/commits"
	"github.com/matthewmueller/chunky/repos"
)

// Download the cache to the local filesystem
func Download(ctx context.Context, log *slog.Logger, from repos.Repo, to repos.FS) (*Local, error) {
	// Load currently cached files
	cache, err := Load(log, to)
	if err != nil {
		return nil, err
	}
	// Download any new commits
	if err := cache.Download(ctx, from); err != nil {
		return nil, err
	}
	return cache, nil
}

// Load from existing cache
func Load(log *slog.Logger, fsys repos.FS) (*Local, error) {
	cache := &Local{
		log,
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
		commitId := de.Name()
		data, err := fs.ReadFile(fsys, commitId)
		if err != nil {
			return nil, err
		}
		commit, err := commits.Unpack(data)
		if err != nil {
			if isCacheInvalid(err) {
				log.Debug("removing invalid cache file", "file", commitId)
				// Remove the invalid cache file
				if err := fsys.RemoveAll(commitId); err != nil {
					return nil, err
				}
				continue
			}
			return nil, err
		}
		for _, file := range commit.Files() {
			cache.files[cacheKey(file.Path, file.Id)] = file
		}
		cache.localCommits[commitId] = commit
	}

	return cache, nil
}

func isCacheInvalid(err error) bool {
	if err == nil {
		return false
	}
	return strings.HasPrefix(err.Error(), "gob: ")
}

func isRemoteInvalid(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "unable to decode commit")
}

type Local struct {
	log          *slog.Logger
	fsys         repos.FS
	files        map[string]*commits.File   // path:hash -> pack_file
	localCommits map[string]*commits.Commit // commitId -> commit
}

var _ Cache = (*Local)(nil)

// Download the latest commits
func (c *Local) Download(ctx context.Context, repo repos.Repo) error {
	remoteCommits := map[string]bool{}
	for commitId := range c.localCommits {
		remoteCommits[commitId] = false
	}

	des, err := repo.ReadDir("commits")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("caches: unable to read commits directory: %w", err)
	}
	for _, de := range des {
		fpath := filepath.Join("commits", de.Name())
		commitId := filepath.Base(fpath)
		remoteCommits[commitId] = true

		// Skip if we already have the commit
		if _, ok := c.localCommits[commitId]; ok {
			continue
		}

		// Download the commit
		commitFile, err := repos.Download(ctx, repo, fpath)
		if err != nil {
			return err
		}
		commitData := commitFile.Data

		// Unpack the commit
		commit, err := commits.Unpack(commitData)
		if err != nil {
			if !isRemoteInvalid(err) {
				return fmt.Errorf("caches: unable to unpack commit %s: %w", commitId, err)
			}
			// Skip the invalid commit file
			c.log.Debug("skipping invalid remote commit file", "file", commitId)
			commit = commits.Invalid(commitId)
			commitData, err = commit.Pack()
			if err != nil {
				return fmt.Errorf("caches: unable to pack invalid commit %s: %w", commitId, err)
			}
		}

		// Write the commit to the cache
		if err := c.fsys.WriteFile(commitId, commitData, 0644); err != nil {
			return err
		}

		// Add the files to the cache
		for _, file := range commit.Files() {
			c.files[cacheKey(file.Path, file.Id)] = file
		}

		// Mark the commit as downloaded
		c.localCommits[commitId] = commit
	}

	// Remove any commits that are no longer in the repo
	for commitId, ok := range remoteCommits {
		if ok {
			continue
		}
		if err := c.fsys.RemoveAll(commitId); err != nil {
			return err
		}
		commit := c.localCommits[commitId]
		for _, file := range commit.Files() {
			delete(c.files, cacheKey(file.Path, file.Id))
		}
		delete(c.localCommits, commitId)
	}

	return nil
}

func (c *Local) Get(path, hash string) (file *commits.File, ok bool) {
	file, ok = c.files[cacheKey(path, hash)]
	return file, ok
}

func cacheKey(path, hash string) string {
	return path + ":" + hash
}
