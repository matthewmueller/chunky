package cli

import (
	"context"
	"io/fs"

	"github.com/livebud/cli"
	"github.com/matthewmueller/chunky/repos"
)

type Create struct {
	Repo string
}

func (c *Create) command(cli cli.Command) cli.Command {
	cmd := cli.Command("create", "create a new repository")
	cmd.Arg("path", "path to the new repository").String(&c.Repo)
	return cmd
}

func (c *CLI) Create(ctx context.Context, in *Create) error {
	repoUrl, err := repos.Parse(in.Repo)
	if err != nil {
		return err
	}
	repo, err := c.loadRepoFromUrl(repoUrl)
	if err != nil {
		return err
	}
	// Create the repository
	tree := repos.Tree{}
	tree["commits"] = &repos.File{
		Mode: fs.ModeDir | 0755,
	}
	tree["packs"] = &repos.File{
		Mode: fs.ModeDir | 0755,
	}
	tree["tags"] = &repos.File{
		Mode: fs.ModeDir | 0755,
	}
	return repo.Upload(ctx, tree)
}
