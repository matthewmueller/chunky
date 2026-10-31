package commits

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/matthewmueller/chunky/internal/tags"
	"github.com/matthewmueller/chunky/internal/timeid"
	"github.com/matthewmueller/chunky/repos"
	"github.com/segmentio/ksuid"
)

func New(user string, createdAt time.Time) *Commit {
	return &Commit{
		id:        ksuid.New().String(),
		user:      user,
		createdAt: createdAt,
	}
}

type Commit struct {
	id        string
	user      string
	createdAt time.Time
	size      uint64
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
	if c.id != "" {
		return c.id
	}
	return timeid.Encode(c.createdAt)
}

func (c *Commit) CreatedAt() time.Time {
	return c.createdAt
}

func (c *Commit) Size() uint64 {
	return uint64(c.size)
}

func (c *Commit) User() string {
	return c.user
}

type commitState struct {
	ID        string    `json:"id,omitempty"`
	User      string    `json:"user,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	Checksum  string    `json:"checksum,omitempty"`
	Size      uint64    `json:"size,omitempty"`
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
	Size   uint64 `json:"size,omitempty"`
	Id     string `json:"id,omitempty"`
	PackId string `json:"pack_id,omitempty"`
}

func (c *Commit) Add(file *File) {
	if _, ok := c.File(file.Path); ok {
		return
	}
	c.files = append(c.files, file)
	c.size += file.Size
}

func (c *Commit) state() *commitState {
	checksum := sha256.New()
	files := c.Files()
	for _, file := range files {
		checksum.Write([]byte(file.Id))
	}
	return &commitState{
		ID:        c.id,
		User:      c.user,
		CreatedAt: c.createdAt,
		Checksum:  hex.EncodeToString(checksum.Sum(nil)),
		Size:      c.size,
		Files:     c.files,
	}
}

func (c *Commit) Encode(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(c.state())
}

func (c *Commit) Pack() ([]byte, error) {
	out := new(bytes.Buffer)
	writer, err := zstd.NewWriter(out)
	if err != nil {
		return nil, err
	}
	enc := gob.NewEncoder(writer)
	if err := enc.Encode(c.state()); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func (c *Commit) File(path string) (*File, bool) {
	for _, file := range c.files {
		if file.Path == path {
			return file, true
		}
	}
	return nil, false
}

func Unpack(data []byte) (*Commit, error) {
	reader, err := zstd.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("commits: unable to create zstd reader: %w", err)
	}
	dec := gob.NewDecoder(reader)
	var state commitState
	if err := dec.Decode(&state); err != nil {
		return nil, fmt.Errorf("commits: unable to decode commit: %w", err)
	}
	if err := state.Verify(); err != nil {
		return nil, fmt.Errorf("commits: error verifying commit: %w", err)
	}
	return &Commit{
		id:        state.ID,
		user:      state.User,
		createdAt: state.CreatedAt,
		size:      state.Size,
		files:     state.Files,
	}, nil
}

func read(ctx context.Context, repo repos.Repo, path string) (*Commit, error) {
	commitFile, err := repos.Download(ctx, repo, path)
	if err != nil {
		return nil, err
	}
	return Unpack(commitFile.Data)
}

// Read a commit by its id or by a tag
func Read(ctx context.Context, repo repos.Repo, revision string) (*Commit, error) {
	commit, err := read(ctx, repo, path.Join("commits", revision))
	if nil == err {
		return commit, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("commits: unable to read commit %q: %w", revision, err)
	}
	// Not a commit id, so read the tag, which lists its commits with the newest
	// last
	tag, err := tags.Read(ctx, repo, revision)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("commits: revision not found: %s: %w", revision, err)
		}
		return nil, fmt.Errorf("commits: unable to read tag %q: %w", revision, err)
	}
	commit, err = read(ctx, repo, path.Join("commits", tag.Newest()))
	if err != nil {
		return nil, fmt.Errorf("commits: unable to read commit %q for tag %q: %w", tag.Newest(), revision, err)
	}
	return commit, nil
}

func ReadAll(ctx context.Context, repo repos.Repo) (commits []*Commit, err error) {
	des, err := repo.ReadDir("commits")
	if err != nil {
		return nil, fmt.Errorf("commits: unable to read commits directory: %w", err)
	}
	for _, de := range des {
		fpath := filepath.Join("commits", de.Name())
		commit, err := read(ctx, repo, fpath)
		if err != nil {
			return nil, fmt.Errorf("commits: unable to read commit %q: %w", fpath, err)
		}
		commits = append(commits, commit)
	}
	// Sort by newest first
	sort.Slice(commits, func(i, j int) bool {
		return commits[i].createdAt.After(commits[j].createdAt)
	})
	return commits, nil
}
