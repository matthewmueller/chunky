package cli

import (
	"context"
	"os"

	"github.com/livebud/cli"
	"github.com/matthewmueller/chunky/repos"
)

type CachePrune struct {
	Repo string
}

func (c *CachePrune) command(cli cli.Command) cli.Command {
	cmd := cli.Command("cache-prune", "prune a repository and local cache").Advanced()
	cmd.Arg("repo", "repo path").String(&c.Repo)
	return cmd
}

func (c *CLI) CachePrune(ctx context.Context, in *CachePrune) error {
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
