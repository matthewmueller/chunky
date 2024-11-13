package repos

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/matthewmueller/virt"
)

// Parse parses a repository path and returns a URL.
func Parse(repoPath string) (*url.URL, error) {
	// Handle SSH-like paths
	if !strings.Contains(repoPath, "://") && strings.Contains(repoPath, "@") {
		repoPath = "ssh://" + repoPath
	}

	// Handle connection strings
	if strings.Contains(repoPath, "://") {
		url, err := url.Parse(repoPath)
		if err != nil {
			return url, err
		}
		if url.Scheme == "" {
			return nil, fmt.Errorf("unsupported repository %q. Relative path is not allowed as a repository path", repoPath)
		}
		return url, nil
	}

	// Fallback to local file paths
	return &url.URL{
		Scheme: "file",
		Path:   filepath.Clean(repoPath),
	}, nil
}

// Repo is a repository interface for uploading and downloading files
type Repo interface {
	// Upload from a filesystem to the repository
	Upload(ctx context.Context, from fs.FS) error
	// Download paths from the repository to a filesystem
	Download(ctx context.Context, to virt.FS, paths ...string) error
	// Walk the repository
	Walk(ctx context.Context, dir string, fn fs.WalkDirFunc) error
	// Close the repository
	Close() error
}

// Download a single file from the repository.
func Download(ctx context.Context, repo Repo, path string) (*virt.File, error) {
	fsys := virt.Tree{}
	if err := repo.Download(ctx, fsys, path); err != nil {
		return nil, fmt.Errorf("repos: unable to download file %q: %w", path, err)
	}
	return fsys[path], nil
}
