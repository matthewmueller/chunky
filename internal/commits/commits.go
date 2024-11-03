package commits

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"time"

	"github.com/matthewmueller/chunky/internal/chunker"
	"github.com/matthewmueller/chunky/internal/gzip"
	"github.com/matthewmueller/chunky/internal/repos"
	"github.com/matthewmueller/chunky/internal/timeid"
	"github.com/matthewmueller/virt"
)

func New() *Commit {
	return &Commit{
		CreatedAt: time.Now(),
		Files:     []*File{},
	}
}

type Commit struct {
	CreatedAt time.Time `json:"created_at,omitempty"`
	Checksum  string    `json:"checksum,omitempty"`
	Size      uint64    `json:"size,omitempty"`
	Files     []*File
}

func (c *Commit) File(path string, info fs.FileInfo) *File {
	file := findFile(c, path)
	if file == nil {
		file = &File{Path: path}
		c.Files = append(c.Files, file)
	}
	file.Mode = info.Mode()
	file.ModTime = info.ModTime().UTC()
	file.Size = info.Size()
	return file
}

func (c *Commit) Hash() string {
	return timeid.Encode(c.CreatedAt.UTC())
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

func resolveRevision(ctx context.Context, repo repos.Repo, revision string) (sha string, err error) {
	// Try to download the commit directly
	if _, err := repos.Download(ctx, repo, "commits/"+revision); err == nil {
		return revision, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("commits: unable to download commit: %w", err)
	}
	// Try to download the tag
	tag, err := repos.Download(ctx, repo, "tags/"+revision)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("commits: revision not found: %s", revision)
		}
		return "", fmt.Errorf("commits: unable to download tag: %w", err)
	}
	return string(tag.Data), nil
}

type writtenCommit struct {
	CreatedAt time.Time `json:"created_at,omitempty"`
	Checksum  string    `json:"checksum,omitempty"`
	Size      uint64    `json:"size,omitempty"`
	Objects   []string  `json:"objects,omitempty"`
}

func read(ctx context.Context, repo repos.Repo, path string) (*Commit, error) {
	commitFile, err := repos.Download(ctx, repo, path)
	if err != nil {
		return nil, err
	}
	var wc writtenCommit
	if err := json.Unmarshal(commitFile.Data, &wc); err != nil {
		return nil, fmt.Errorf("commits: unable to unmarshal commit: %w", err)
	}
	commit := &Commit{
		CreatedAt: wc.CreatedAt,
		Checksum:  wc.Checksum,
		Size:      wc.Size,
	}
	// Download all the files data
	var filesData []byte
	for _, object := range wc.Objects {
		objectPath := fmt.Sprintf("objects/%s/%s", object[:2], object[2:])
		dataFile, err := repos.Download(ctx, repo, objectPath)
		if err != nil {
			return nil, fmt.Errorf("commits: unable to download object %q: %w", object, err)
		}
		decompressed, err := gzip.Decompress(dataFile.Data)
		if err != nil {
			return nil, fmt.Errorf("commits: unable to decompress object %q: %w", object, err)
		}
		filesData = append(filesData, decompressed...)
	}
	// Unmarshal the files data into commit files
	if err := json.Unmarshal(filesData, &commit.Files); err != nil {
		return nil, fmt.Errorf("commits: unable to unmarshal files: %w", err)
	}
	return commit, nil
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
	sort.Slice(commits, func(i, j int) bool {
		return commits[i].CreatedAt.After(commits[j].CreatedAt)
	})
	return commits, nil
}

func findFile(commit *Commit, path string) *File {
	for _, file := range commit.Files {
		if file.Path == path {
			return file
		}
	}
	return nil
}

func FindFile(commit *Commit, path string) (*File, error) {
	file := findFile(commit, path)
	if file == nil {
		return nil, fmt.Errorf("commits: %s %w", path, fs.ErrNotExist)
	}
	return file, nil
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
			return nil, fmt.Errorf("commits: unable to download object %q: %w", object, err)
		}
		decompressed, err := gzip.Decompress(dataFile.Data)
		if err != nil {
			return nil, fmt.Errorf("commits: unable to decompress object %q: %w", object, err)
		}
		vfile.Data = append(vfile.Data, decompressed...)
	}
	return vfile, nil
}

func Write(ctx context.Context, to virt.FS, commit *Commit) error {
	wc := writtenCommit{
		CreatedAt: commit.CreatedAt.UTC(),
		Checksum:  commit.Checksum,
		Size:      commit.Size,
	}
	fileData, err := json.MarshalIndent(commit.Files, "", "  ")
	if err != nil {
		return fmt.Errorf("commits: unable to marshal files: %w", err)
	}
	chunker := chunker.New(fileData)
	for {
		chunk, err := chunker.Chunk()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("commits: unable to chunk files: %w", err)
		}
		hash := chunk.Hash()
		fpath := fmt.Sprintf("objects/%s/%s", hash[:2], hash[2:])
		compressed, err := gzip.Compress(chunk.Data)
		if err != nil {
			return fmt.Errorf("commits: unable to compress object %q: %w", hash, err)
		}
		if err := to.WriteFile(fpath, compressed, 0644); err != nil {
			return fmt.Errorf("commits: unable to write object %q: %w", hash, err)
		}
		wc.Objects = append(wc.Objects, hash)
	}
	data, err := json.MarshalIndent(wc, "", "  ")
	if err != nil {
		return fmt.Errorf("commits: unable to marshal commit: %w", err)
	}
	commitPath := path.Join("commits", commit.Hash())
	return to.WriteFile(commitPath, data, 0644)
}
