package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/livebud/cli"
	"github.com/matthewmueller/chunky/internal/commits"
	"github.com/matthewmueller/chunky/internal/tags"
)

type Tag struct {
	Repo     string
	Revision string
	Tag      string
}

func (t *Tag) command(cli cli.Command) cli.Command {
	cmd := cli.Command("tag", "tag a commit")
	cmd.Arg("repo", "repository to tag").String(&t.Repo)
	cmd.Arg("revision", "revision to tag").String(&t.Revision)
	cmd.Arg("tag", "tag to create").String(&t.Tag)
	return cmd
}

func (c *CLI) Tag(ctx context.Context, in *Tag) error {
	// Load the repository
	repo, err := c.loadRepo(in.Repo)
	if err != nil {
		return err
	}

	// Check that the commit exists
	commit, err := commits.Read(ctx, repo, in.Revision)
	if err != nil {
		return fmt.Errorf("cli: unable to read commit for %s: %w", in.Revision, err)
	}

	// If the tag already exists, append the commit to the end of the file
	tag, err := tags.Read(ctx, repo, in.Tag)
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
	return repo.Upload(ctx, tag.Tree())
}
