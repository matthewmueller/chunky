package cli

import (
	"context"

	"github.com/livebud/cli"
	"github.com/matthewmueller/chunky/internal/commits"
)

type CatCommit struct {
	Repo     string
	Revision string
}

func (c *CatCommit) command(cli cli.Command) cli.Command {
	cmd := cli.Command("cat-commit", "show a commit").Advanced()
	cmd.Arg("repo", "repository to show").String(&c.Repo)
	cmd.Arg("revision", "commit or tag to show").String(&c.Revision)
	return cmd
}

func (c *CLI) CatCommit(ctx context.Context, in *CatCommit) error {
	repo, err := c.loadRepo(in.Repo)
	if err != nil {
		return err
	}

	commit, err := commits.Read(ctx, repo, in.Revision)
	if err != nil {
		return err
	}

	return commit.Encode(c.Stdout)
}
