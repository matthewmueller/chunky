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
	if !strings.Contains(repoPath, "://") && strings.Contains(repoPath, "@") && strings.Contains(repoPath, ":") {
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
	repoPath, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, err
	}
	return &url.URL{
		Scheme: "file",
		Path:   repoPath,
	}, nil
}

type Repo interface {
	Upload(ctx context.Context, from fs.FS) error
	Download(ctx context.Context, to virt.FS, paths ...string) error
	Walk(ctx context.Context, dir string, fn fs.WalkDirFunc) error
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
