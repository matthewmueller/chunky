package cli

import (
	"context"
	"fmt"

	"github.com/livebud/cli"
	"github.com/matthewmueller/chunky/internal/commits"
	"github.com/matthewmueller/virt"
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

	// Create the tag file
	tree := virt.Tree{
		fmt.Sprintf("tags/%s", in.Tag): &virt.File{
			Data: []byte(commit.ID()),
			Mode: 0644,
		},
	}

	// Upload the tag file
	return repo.Upload(ctx, tree)
}
