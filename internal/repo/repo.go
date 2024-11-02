package repo

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

	// i := strings.Index(repoPath, ":")
	// if i > 0 {
	// 	return &url.URL{
	// 		Scheme: "ssh",
	// 		Host:   repoPath[0:i],
	// 		Path:   repoPath[i+1:],
	// 	}, nil
	// }

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
	Remove(ctx context.Context, paths ...string) error
	Stat(ctx context.Context, path string) (fs.FileInfo, error)
	Walk(ctx context.Context, dir string, fn fs.WalkDirFunc) error
}
