package local

import (
	"context"
	"io/fs"

	"github.com/matthewmueller/chunky/repos"
	"github.com/matthewmueller/virt"
)

func New(key string, fsys virt.FS) *Repo {
	return &Repo{key, fsys}
}

type Repo struct {
	key  string
	fsys virt.FS
}

var _ repos.Repo = (*Repo)(nil)

func (r *Repo) Key() string {
	return r.key
}

func (r *Repo) Upload(ctx context.Context, from fs.FS) error {
	return virt.WriteFS(from, r.fsys)
}

func (r *Repo) Download(ctx context.Context, to virt.FS, paths ...string) error {
	return virt.WriteFS(r.fsys, to, paths...)
}

func (r *Repo) Walk(ctx context.Context, dir string, fn fs.WalkDirFunc) error {
	return fs.WalkDir(r.fsys, dir, fn)
}

func (r *Repo) Close() error {
	return nil
}
