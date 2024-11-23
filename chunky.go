package chunky

import (
	"context"
	"errors"
	"log/slog"
	"sort"

	"github.com/matthewmueller/chunky/internal/commits"
	"github.com/matthewmueller/chunky/internal/tags"
	"github.com/matthewmueller/chunky/repos"
)

func New(log *slog.Logger) *Client {
	return &Client{log}
}

type Client struct {
	log *slog.Logger
}

type FindCommit struct {
	Repo     repos.Repo
	Revision string
}

func (in *FindCommit) validate() (err error) {
	if in.Repo == nil {
		err = errors.Join(err, errors.New("missing 'repo'"))
	}
	if in.Revision == "" {
		err = errors.Join(err, errors.New("missing 'revision'"))
	}
	return err
}

// Commit represents a commit
type Commit = commits.Commit

// FindCommit finds a commit by a revision
func (c *Client) FindCommit(ctx context.Context, in *FindCommit) (*Commit, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	return commits.Read(ctx, in.Repo, in.Revision)
}

type ListTags struct {
	Repo repos.Repo
}

func (in *ListTags) validate() (err error) {
	if in.Repo == nil {
		err = errors.Join(err, errors.New("missing 'repo'"))
	}
	return err
}

type Tag struct {
	Name    string
	Commits []string
}

func (c *Client) ListTags(ctx context.Context, in *ListTags) (allTags []*Tag, err error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	commitToTags, err := tags.ReadMap(ctx, in.Repo)
	if err != nil {
		return nil, err
	}
	tagMap := map[string][]string{}
	for commit, tag := range commitToTags {
		for _, tag := range tag {
			tagMap[tag] = append(tagMap[tag], commit)
		}
	}
	for tag, commits := range tagMap {
		allTags = append(allTags, &Tag{tag, commits})
	}
	sort.Slice(allTags, func(i, j int) bool {
		return allTags[i].Name < allTags[j].Name
	})
	return allTags, nil
}
