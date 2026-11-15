package chunky

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/user"
	"path"
	"runtime"
	"strings"
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
type DeleteTag struct {
	Repo repos.Repo
	Tag  string
}

func (in *DeleteTag) validate() (err error) {
	if in.Repo == nil {
		err = errors.Join(err, errors.New("missing 'repo'"))
	}
	// The tag names a file in tags/, so it can't be a path to anything else
	if in.Tag == "" || in.Tag == "." || in.Tag == ".." || strings.Contains(in.Tag, "/") {
		err = errors.Join(err, fmt.Errorf("invalid tag %q", in.Tag))
	}
	return err
}

// DeleteTag removes a tag and its history. The commits it named stay in the
// repository, but uploads stop reading them. Deleting a missing tag is not an
// error.
func (c *Client) DeleteTag(ctx context.Context, in *DeleteTag) error {
	if err := in.validate(); err != nil {
		return err
	}
	return in.Repo.RemoveAll(path.Join("tags", in.Tag))
}

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
