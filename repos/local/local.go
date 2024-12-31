package local

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/matthewmueller/chunky/repos"
	"github.com/matthewmueller/virt"
	"golang.org/x/sync/errgroup"
)

func New(fsys repos.FS) *Repo {
	return &Repo{fsys}
}

type Repo struct {
	fsys repos.FS
}

var _ repos.Repo = (*Repo)(nil)

func (r *Repo) Upload(ctx context.Context, fromCh <-chan *repos.File) error {
	eg := new(errgroup.Group)
	for file := range fromCh {
		eg.Go(func() error {
			return r.uploadFile(file)
		})
	}
	return eg.Wait()
}

func (r *Repo) uploadFile(file *repos.File) error {
	if file.IsDir() {
		if err := r.fsys.MkdirAll(file.Path, file.Mode); err != nil {
			return fmt.Errorf("repo: unable to create directory %q: %w", file.Path, err)
		}
		return nil
	}
	if err := r.fsys.WriteFile(file.Path, file.Data, file.Mode); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("repo: unable to write file %q: %w", file.Path, err)
		}
		if err := r.fsys.MkdirAll(filepath.Dir(file.Path), 0755); err != nil {
			return fmt.Errorf("repo: unable to create directory %q: %w", file.Path, err)
		}
		if err := r.fsys.WriteFile(file.Path, file.Data, file.Mode); err != nil {
			return fmt.Errorf("repo: unable to write file %q: %w", file.Path, err)
		}
	}
	return nil
}

func (r *Repo) Download(ctx context.Context, toCh chan<- *repos.File, paths ...string) error {
	target := path.Join(paths...)
	if target == "" {
		target = "."
	}
	return r.Walk(ctx, target, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		vfile, err := virt.From(r.fsys, path)
		if err != nil {
			return err
		}
		toCh <- vfile
		return nil
	})
}

func (r *Repo) Walk(ctx context.Context, dir string, fn fs.WalkDirFunc) error {
	return fs.WalkDir(r.fsys, dir, fn)
}

func (r *Repo) Close() error {
	return nil
}
