package chunky

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/user"
	"runtime"
	"time"

	"github.com/matthewmueller/chunky/internal/commits"
	"github.com/matthewmueller/chunky/internal/tags"
	"github.com/matthewmueller/chunky/repos"
)

const kiB = 1024
const miB = 1024 * kiB

// DefaultConcurrency returns the default number of concurrent downloads
var DefaultConcurrency = runtime.NumCPU() * 2

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

// FindCommit finds a commit in a repository by a commit id or tag
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

// Tag lists every commit it has named, oldest first
type Tag = tags.Tag

// Tagged is one entry in a tag's history: the commit, and when and by whom
// it was tagged
type Tagged = tags.Tagged

func (c *Client) ListTags(ctx context.Context, in *ListTags) ([]*Tag, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	return tags.ReadAll(ctx, in.Repo)
}

type TagRevision struct {
	Repo     repos.Repo
	Tag      string
	Revision string
	// User who tagged the revision (default: the current user)
	User string
}

// TagRevision appends a revision to a tag's history
func (c *Client) TagRevision(ctx context.Context, in *TagRevision) (err error) {
	if in.User == "" {
		if in.User, err = currentUser(); err != nil {
			return err
		}
	}

	// Check that the commit exists
	commit, err := commits.Read(ctx, in.Repo, in.Revision)
	if err != nil {
		return fmt.Errorf("unable to read commit for %s: %w", in.Revision, err)
	}

	return tags.Append(ctx, in.Repo, in.Tag, &tags.Tagged{
		Commit: commit.ID(),
		At:     time.Now().UTC(),
		User:   in.User,
	})
}

// currentUser returns the current user's full name, or their username when
// the name isn't set
func currentUser() (string, error) {
	user, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("missing user and getting current user failed with: %w", err)
	}
	if user.Name != "" {
		return user.Name, nil
	}
	return user.Username, nil
}
