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

type Tree = virt.Tree
type File = virt.File
type ReadFS = virt.FromFS
type FS = virt.FS

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

// Writer is a repository that can only be written to, like a one-way stream
type Writer interface {
	// Upload from a filesystem to the repository
	Upload(ctx context.Context, file *File) error
	// Close the repository
	Close() error
}

// Repo is a repository interface for uploading and downloading files
type Repo interface {
	Writer
	// Download paths from the repository to a filesystem
	Download(ctx context.Context, path string) (*File, error)
	// ReadDir reads a directory from the repository
	ReadDir(dir string) ([]fs.DirEntry, error)
}

// Download a single file from the repository.
func Download(ctx context.Context, repo Repo, path string) (*File, error) {
	return repo.Download(ctx, path)
}
