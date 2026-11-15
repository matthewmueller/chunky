package tags

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/matthewmueller/chunky/internal/timeid"
	"github.com/matthewmueller/chunky/repos"
	"github.com/segmentio/ksuid"
)

// Tag lists every commit it has named, oldest first
type Tag struct {
	Name    string
	History []*Tagged
}

// Tagged is one entry in a tag's history, stored as a line of JSON. New fields
// can be added without breaking older readers, which ignore them. Bare commit
// ids, written before tags recorded their history, take At from when the
// commit was created and have an empty User.
type Tagged struct {
	Commit string    `json:"commit"`
	At     time.Time `json:"at"`
	User   string    `json:"user"`
}

func (t *Tag) Newest() string {
	return t.History[len(t.History)-1].Commit
}

// Read a tag by name
func Read(ctx context.Context, repo repos.Repo, name string) (*Tag, error) {
	tagFile, err := repos.Download(ctx, repo, filepath.Join("tags", name))
	if err != nil {
		return nil, err
	}
	tag := &Tag{Name: name}
	for _, line := range strings.Split(strings.TrimSpace(string(tagFile.Data)), "\n") {
		entry := &Tagged{Commit: line, At: createdAt(line)}
		if strings.HasPrefix(line, "{") {
			if err := json.Unmarshal([]byte(line), entry); err != nil {
				return nil, fmt.Errorf("tags: invalid entry in tag %q: %w", name, err)
			}
		}
		tag.History = append(tag.History, entry)
	}
	return tag, nil
}

// createdAt returns when a commit was created from its id. Commit ids are
// ksuids, or timeids for commits from before ksuids. Other ids get a zero time.
func createdAt(id string) time.Time {
	if k, err := ksuid.Parse(id); err == nil {
		return k.Time().UTC()
	}
	if t, err := timeid.Decode(id); err == nil {
		return t
	}
	return time.Time{}
}

// Append an entry to a tag's history, creating the tag if it doesn't exist.
// The entry is added to the end of the file as-is, so existing lines, and any
// fields this version doesn't know about, are kept byte for byte.
func Append(ctx context.Context, repo repos.Repo, name string, entry *Tagged) error {
	fpath := path.Join("tags", name)
	var data []byte
	file, err := repos.Download(ctx, repo, fpath)
	if nil == err {
		data = file.Data
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("tags: unable to read tag %q: %w", name, err)
	}
	// Tags written before v0.8.0 may not end in a newline
	if len(data) > 0 && data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	line, err := encode(entry)
	if err != nil {
		return fmt.Errorf("tags: unable to encode entry for tag %q: %w", name, err)
	}
	return repo.Upload(ctx, &repos.File{Path: fpath, Mode: 0644, Data: append(data, line...)})
}

// Set replaces a tag's history with a single entry
func Set(ctx context.Context, repo repos.Repo, name string, entry *Tagged) error {
	line, err := encode(entry)
	if err != nil {
		return fmt.Errorf("tags: unable to encode entry for tag %q: %w", name, err)
	}
	return repo.Upload(ctx, &repos.File{Path: path.Join("tags", name), Mode: 0644, Data: line})
}

// encode an entry as a line of JSON
func encode(entry *Tagged) ([]byte, error) {
	// Whole seconds keep the lines short
	line, err := json.Marshal(&Tagged{
		Commit: entry.Commit,
		At:     entry.At.UTC().Truncate(time.Second),
		User:   entry.User,
	})
	if err != nil {
		return nil, err
	}
	return append(line, '\n'), nil
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
