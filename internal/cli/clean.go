package cli

import (
	"context"
	"os"

	"github.com/livebud/cli"
	"github.com/matthewmueller/chunky/repos"
)

type Clean struct {
	Repo string
}

func (c *Clean) command(cli cli.Command) cli.Command {
	cmd := cli.Command("clean", "clean a repository and local cache").Advanced()
	cmd.Arg("repo", "repo path").String(&c.Repo)
	return cmd
}

func (c *CLI) Clean(ctx context.Context, in *Clean) error {
	repoUrl, err := repos.Parse(in.Repo)
	if err != nil {
		return err
	}

	cacheDir, err := c.cacheDir(repoUrl)
	if err != nil {
		return err
	}

	return os.RemoveAll(cacheDir)
}
