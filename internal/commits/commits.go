package commits

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"time"

	"github.com/matthewmueller/chunky/internal/repos"
	"github.com/matthewmueller/virt"
)

func New() *Commit {
	return &Commit{
		CreatedAt: time.Now(),
		Files:     map[string]*File{},
	}
}

type Commit struct {
	CreatedAt time.Time        `json:"created_at,omitempty"`
	Files     map[string]*File `json:"files,omitempty"`
	Checksum  string           `json:"checksum,omitempty"`
}

func (c *Commit) File(path string, info fs.FileInfo) *File {
	file, ok := c.Files[path]
	if !ok {
		file = &File{Path: path}
		c.Files[path] = file
	}
	file.Mode = info.Mode()
	file.ModTime = info.ModTime().UTC()
	file.Size = info.Size()
	return file
}

func (c *Commit) Size() (size uint64) {
	for _, file := range c.Files {
		size += uint64(file.Size)
	}
	return size
}

type File struct {
	Path    string      `json:"path,omitempty"`
	Mode    fs.FileMode `json:"mode,omitempty"`
	ModTime time.Time   `json:"modtime,omitempty"`
	Size    int64       `json:"size,omitempty"`
	Objects []string    `json:"objects,omitempty"`
}

func (f *File) Add(object string) {
	f.Objects = append(f.Objects, object)
}

func read(ctx context.Context, repo repos.Repo, path string) (*Commit, error) {
	commitFile, err := repos.Download(ctx, repo, path)
	if err != nil {
		return nil, err
	}
	var commit *Commit
	if err := json.Unmarshal(commitFile.Data, &commit); err != nil {
		return nil, fmt.Errorf("cli: unable to unmarshal commit: %w", err)
	}
	return commit, nil
}

func resolveRevision(ctx context.Context, repo repos.Repo, revision string) (sha string, err error) {
	// Try to download the commit directly
	if _, err := repos.Download(ctx, repo, "commits/"+revision); err == nil {
		return revision, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("cli: unable to download commit: %w", err)
	}
	// Try to download the tag
	tag, err := repos.Download(ctx, repo, "tags/"+revision)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("cli: revision not found: %s", revision)
		}
		return "", fmt.Errorf("cli: unable to download tag: %w", err)
	}
	return string(tag.Data), nil
}

func Read(ctx context.Context, repo repos.Repo, revision string) (*Commit, error) {
	commitSha, err := resolveRevision(ctx, repo, revision)
	if err != nil {
		return nil, err
	}
	return read(ctx, repo, path.Join("commits", commitSha))
}

func ReadAll(ctx context.Context, repo repos.Repo) (commits []*Commit, err error) {
	if err := repo.Walk(ctx, "commits", func(fpath string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		} else if de.IsDir() {
			return nil
		}
		commit, err := read(ctx, repo, fpath)
		if err != nil {
			return err
		}
		commits = append(commits, commit)
		return nil
	}); err != nil {
		return nil, err
	}
	return commits, nil
}

func ReadFile(ctx context.Context, repo repos.Repo, file *File) (*virt.File, error) {
	vfile := &virt.File{
		Path:    file.Path,
		Mode:    file.Mode,
		ModTime: file.ModTime,
	}
	for _, object := range file.Objects {
		objectPath := fmt.Sprintf("objects/%s/%s", object[:2], object[2:])
		dataFile, err := repos.Download(ctx, repo, objectPath)
		if err != nil {
			return nil, fmt.Errorf("cli: unable to download object %q: %w", object, err)
		}
		vfile.Data = append(vfile.Data, dataFile.Data...)
	}
	return vfile, nil
}
