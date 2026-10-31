package tags

import (
	"context"
	"fmt"
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

// File returns a repo file for uploading
func (t *Tag) File() *repos.File {
	return &repos.File{
		Path: path.Join("tags", t.Name),
		Mode: 0644,
		Data: []byte(strings.Join(t.Commits, "\n") + "\n"),
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
	des, err := repo.ReadDir("tags")
	if err != nil {
		return nil, fmt.Errorf("tags: unable to read tags directory: %w", err)
	}
	for _, de := range des {
		tag, err := Read(ctx, repo, de.Name())
		if err != nil {
			return nil, fmt.Errorf("tags: unable to read tag %q: %w", de.Name(), err)
		}
		tags = append(tags, tag)
	}
	sort.Slice(tags, func(i, j int) bool {
		return tags[i].Name < tags[j].Name
	})
	return tags, nil
}

func ReadMap(ctx context.Context, repo repos.Repo) (map[string][]*Tag, error) {
	tags := map[string][]*Tag{}
	des, err := repo.ReadDir("tags")
	if err != nil {
		return nil, fmt.Errorf("tags: unable to read tags directory: %w", err)
	}
	for _, de := range des {
		tag, err := Read(ctx, repo, de.Name())
		if err != nil {
			return nil, fmt.Errorf("tags: unable to read tag %q: %w", de.Name(), err)
		}
		commit := tag.Newest()
		tags[commit] = append(tags[commit], tag)
	}
	return tags, nil
}
