package chunky

import (
	"context"
	"errors"
	"log/slog"

	"github.com/matthewmueller/chunky/internal/commits"
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
