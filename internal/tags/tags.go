package tags

import (
	"context"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/matthewmueller/chunky/repos"
)

type Tag struct {
	Name    string
	Commits []string
}

func (t *Tag) Newest() string {
	return t.Commits[len(t.Commits)-1]
}

// Tree returns a virtual filesystem tree for uploading
func (t *Tag) Tree() repos.Tree {
	return repos.Tree{
		path.Join("tags", t.Name): &repos.File{
			Mode: 0644,
			Data: []byte(strings.Join(t.Commits, "\n") + "\n"),
		},
	}
}

func Latest(ref string) *repos.File {
	return &repos.File{
		Path: filepath.Join("tags", "latest"),
		Mode: 0644,
		Data: []byte(ref),
	}
}

func New(name, ref string) *repos.File {
	return &repos.File{
		Path: filepath.Join("tags", name),
		Mode: 0644,
		Data: []byte(ref),
	}
}

// Read a tag by name
func Read(ctx context.Context, repo repos.Repo, name string) (*Tag, error) {
	tagFile, err := repos.Download(ctx, repo, filepath.Join("tags", name))
	if err != nil {
		return nil, err
	}
	commits := strings.Split(strings.TrimSpace(string(tagFile.Data)), "\n")
	return &Tag{
		Name:    name,
		Commits: commits,
	}, nil
}

// ReadAll reads all tags
func ReadAll(ctx context.Context, repo repos.Repo) (tags []*Tag, err error) {
	if err := repo.Walk(ctx, "tags", func(fpath string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		} else if de.IsDir() {
			return nil
		}
		tag, err := Read(ctx, repo, filepath.Base(fpath))
		if err != nil {
			return err
		}
		tags = append(tags, tag)
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Slice(tags, func(i, j int) bool {
		return tags[i].Name < tags[j].Name
	})
	return tags, nil
}

func ReadMap(ctx context.Context, repo repos.Repo) (map[string][]*Tag, error) {
	tags := map[string][]*Tag{}
	if err := repo.Walk(ctx, "tags", func(fpath string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		} else if de.IsDir() {
			return nil
		}
		tag, err := Read(ctx, repo, filepath.Base(fpath))
		if err != nil {
			return err
		}
		commit := tag.Newest()
		tags[commit] = append(tags[commit], tag)
		return nil
	}); err != nil {
		return nil, err
	}
	return tags, nil
}
