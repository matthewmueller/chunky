package local

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/matthewmueller/chunky/internal/repos"
	"github.com/matthewmueller/virt"
	"golang.org/x/sync/errgroup"
)

func New(dir string) *Repo {
	return &Repo{dir}
}

type Repo struct {
	dir string
}

var _ repos.Repo = (*Repo)(nil)

// Create a new repository.
func (r *Repo) Create(ctx context.Context) error {
	eg := errgroup.Group{}
	eg.Go(func() error {
		return os.MkdirAll(filepath.Join(r.dir, "commits"), 0755)
	})
	eg.Go(func() error {
		return os.MkdirAll(filepath.Join(r.dir, "objects"), 0755)
	})
	// Create every possible 2-character base16-encoded directory.
	for i := 0; i < 256; i++ {
		i := i
		eg.Go(func() error {
			return os.MkdirAll(filepath.Join(r.dir, "objects", fmt.Sprintf("%02x", i)), 0755)
		})
	}
	eg.Go(func() error {
		return os.MkdirAll(filepath.Join(r.dir, "tags"), 0755)
	})
	return eg.Wait()
}

func (r *Repo) Upload(ctx context.Context, from fs.FS) error {
	return virt.WriteFS(from, virt.OS(r.dir))
}

func (r *Repo) Download(ctx context.Context, to virt.FS, paths ...string) error {
	return virt.WriteFS(virt.OS(r.dir), to, paths...)
}

func (r *Repo) Walk(ctx context.Context, dir string, fn fs.WalkDirFunc) error {
	return fs.WalkDir(virt.OS(r.dir), dir, fn)
}

func (r *Repo) Close() error {
	return nil
}
