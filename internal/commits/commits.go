package commits

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/matthewmueller/chunky/internal/packs"
	"github.com/matthewmueller/chunky/internal/repos"
	"github.com/matthewmueller/chunky/internal/timeid"
)

func New(createdAt time.Time) *Commit {
	return &Commit{
		createdAt: createdAt,
	}
}

type Commit struct {
	createdAt time.Time
	size      uint
	files     []*File
}

func (c *Commit) Files() (files []*File) {
	return c.files
}

type Pack struct {
	ID    string
	Files []*File
}

func (c *Commit) Packs() (packs []*Pack) {
	packMap := make(map[string]*Pack)
	for _, file := range c.files {
		pack, ok := packMap[file.PackId]
		if !ok {
			pack = &Pack{ID: file.PackId}
			packMap[file.PackId] = pack
		}
		pack.Files = append(pack.Files, file)
	}
	for _, pack := range packMap {
		packs = append(packs, pack)
	}
	sort.Slice(packs, func(i, j int) bool {
		return packs[i].ID < packs[j].ID
	})
	return packs
}

func (c *Commit) ID() string {
	return timeid.Encode(c.createdAt)
}

func (c *Commit) Size() string {
	return fmt.Sprintf("%d", c.size)
}

type commitState struct {
	CreatedAt time.Time `json:"created_at,omitempty"`
	Checksum  string    `json:"checksum,omitempty"`
	Size      uint      `json:"size,omitempty"`
	Files     []*File   `json:"files,omitempty"`
}

func (c *commitState) Verify() error {
	checksum := sha256.New()
	for _, file := range c.Files {
		checksum.Write([]byte(file.Id))
	}
	if c.Checksum != hex.EncodeToString(checksum.Sum(nil)) {
		return fmt.Errorf("commits: checksum mismatch")
	}
	return nil
}

type File struct {
	Path   string `json:"path,omitempty"`
	Size   uint   `json:"size,omitempty"`
	Id     string `json:"id,omitempty"`
	PackId string `json:"pack_id,omitempty"`
}

func findFile(commit *Commit, path string) *File {
	for _, file := range commit.files {
		if file.Path == path {
			return file
		}
	}
	return nil
}

func (c *Commit) Add(file *File) {
	if findFile(c, file.Path) != nil {
		return
	}
	c.files = append(c.files, file)
	c.size += file.Size
}

func (c *Commit) Pack() ([]byte, error) {
	out := new(bytes.Buffer)
	enc, err := zstd.NewWriter(out)
	if err != nil {
		return nil, err
	}
	checksum := sha256.New()
	files := c.Files()
	for _, file := range files {
		checksum.Write([]byte(file.Id))
	}
	if err := json.NewEncoder(enc).Encode(&commitState{
		CreatedAt: c.createdAt,
		Checksum:  hex.EncodeToString(checksum.Sum(nil)),
		Size:      c.size,
		Files:     files,
	}); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func Unpack(data []byte) (*Commit, error) {
	dec, err := zstd.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer dec.Close()
	var state commitState
	if err := json.NewDecoder(dec).Decode(&state); err != nil {
		return nil, err
	}
	if err := state.Verify(); err != nil {
		return nil, err
	}
	return &Commit{
		createdAt: state.CreatedAt,
		size:      state.Size,
		files:     state.Files,
	}, nil
}

func resolveRevision(ctx context.Context, repo repos.Repo, revision string) (sha string, err error) {
	// Try to download the commit directly
	if _, err := repos.Download(ctx, repo, path.Join("commits", revision)); err == nil {
		return revision, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("commits: unable to download commit: %w", err)
	}
	// Try to download the tag
	tag, err := repos.Download(ctx, repo, path.Join("tags", revision))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("commits: revision not found: %s", revision)
		}
		return "", fmt.Errorf("commits: unable to download tag: %w", err)
	}
	return string(tag.Data), nil
}

func read(ctx context.Context, repo repos.Repo, path string) (*Commit, error) {
	commitFile, err := repos.Download(ctx, repo, path)
	if err != nil {
		return nil, err
	}
	return Unpack(commitFile.Data)
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
		return commits[i].createdAt.After(commits[j].createdAt)
	})
	return commits, nil
}

func FindFile(commit *Commit, path string) (*File, error) {
	file := findFile(commit, path)
	if file == nil {
		return nil, fmt.Errorf("commits: %s %w", path, fs.ErrNotExist)
	}
	return file, nil
}

func ReadFile(ctx context.Context, repo repos.Repo, file *File) (*packs.File, error) {
	packFile, err := repos.Download(ctx, repo, path.Join("packs", file.PackId))
	if err != nil {
		return nil, fmt.Errorf("commits: unable to download pack %q: %w", file.PackId, err)
	}
	pack, err := packs.Unpack(packFile.Data)
	if err != nil {
		return nil, fmt.Errorf("commits: unable to unpack pack %q: %w", file.PackId, err)
	}
	return pack.Read(file.Path)
}
