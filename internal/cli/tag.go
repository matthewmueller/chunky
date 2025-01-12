package cli

import (
	"context"

	"github.com/livebud/cli"
	"github.com/matthewmueller/chunky"
)

type Tag struct {
	Repo     string
	Revision string
	Tag      string
}

func (t *Tag) command(cli cli.Command) cli.Command {
	cmd := cli.Command("tag", "tag a commit")
	cmd.Arg("repo", "repository to tag").String(&t.Repo)
	cmd.Arg("tag", "tag to create").String(&t.Tag)
	cmd.Flag("revision", "revision to tag").String(&t.Revision).Default("latest")
	return cmd
}

func (c *CLI) Tag(ctx context.Context, in *Tag) error {
	// Load the repository
	repo, err := c.loadRepo(in.Repo)
	if err != nil {
		return err
	}
	// Tag the revision
	return c.chunky.TagRevision(ctx, &chunky.TagRevision{
		Repo:     repo,
		Revision: in.Revision,
		Tag:      in.Tag,
	})
}
