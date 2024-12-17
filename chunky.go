package chunky

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"

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
	tags, err := tags.ReadAll(ctx, in.Repo)
	if err != nil {
		return nil, err
	}
	for _, tag := range tags {
		allTags = append(allTags, &Tag{
			Name:    tag.Name,
			Commits: tag.Commits,
		})
	}
	return allTags, nil
}

type TagRevision struct {
	Repo     repos.Repo
	Tag      string
	Revision string
}

// TagRevision tags a revision
func (c *Client) TagRevision(ctx context.Context, in *TagRevision) error {
	// Check that the commit exists
	commit, err := commits.Read(ctx, in.Repo, in.Revision)
	if err != nil {
		return fmt.Errorf("cli: unable to read commit for %s: %w", in.Revision, err)
	}

	// If the tag already exists, append the commit to the end of the file
	tag, err := tags.Read(ctx, in.Repo, in.Tag)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("cli: unable to read tag for %s: %w", in.Tag, err)
		}
		tag = &tags.Tag{
			Name: in.Tag,
		}
	}

	// Append the commit to the tag
	tag.Commits = append(tag.Commits, commit.ID())

	// Upload the tag file
	return in.Repo.Upload(ctx, tag.Tree())
}
