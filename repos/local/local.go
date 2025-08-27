package local

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/matthewmueller/chunky/repos"
	"github.com/matthewmueller/virt"
)

func New(fsys repos.FS) *Repo {
	return &Repo{fsys}
}

type Repo struct {
	fsys repos.FS
}

var _ repos.Repo = (*Repo)(nil)

func (r *Repo) Upload(ctx context.Context, file *repos.File) error {
	return r.uploadFile(file)
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

func (r *Repo) Download(ctx context.Context, path string) (*repos.File, error) {
	return virt.From(r.fsys, path)
}

func (r *Repo) ReadDir(dir string) ([]fs.DirEntry, error) {
	return fs.ReadDir(r.fsys, dir)
}

func (r *Repo) Close() error {
	return nil
}
