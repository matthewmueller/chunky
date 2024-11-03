package tags

import (
	"context"
	"io/fs"
	"path"
	"path/filepath"

	"github.com/matthewmueller/chunky/internal/repos"
	"github.com/matthewmueller/virt"
)

func Latest(ref string) *virt.File {
	return &virt.File{
		Path: filepath.Join("tags", "latest"),
		Mode: 0644,
		Data: []byte(ref),
	}
}

func New(name, ref string) *virt.File {
	return &virt.File{
		Path: filepath.Join("tags", name),
		Mode: 0644,
		Data: []byte(ref),
	}
}

func ReadMap(ctx context.Context, repo repos.Repo) (map[string][]string, error) {
	tags := map[string][]string{}
	if err := repo.Walk(ctx, "tags", func(fpath string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		} else if de.IsDir() {
			return nil
		}
		tagFile, err := repos.Download(ctx, repo, fpath)
		if err != nil {
			return err
		}
		tags[string(tagFile.Data)] = append(tags[string(tagFile.Data)], path.Base(fpath))
		return nil
	}); err != nil {
		return nil, err
	}
	return tags, nil
}
