package cli

import (
	"context"
	"io/fs"

	"github.com/livebud/cli"
	"github.com/matthewmueller/chunky/repos"
	"github.com/matthewmueller/virt"
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
	fileCh := make(chan *virt.File, 3)
	fileCh <- &virt.File{
		Path: "commits",
		Mode: fs.ModeDir | 0755,
	}
	fileCh <- &virt.File{
		Path: "packs",
		Mode: fs.ModeDir | 0755,
	}
	fileCh <- &virt.File{
		Path: "tags",
		Mode: fs.ModeDir | 0755,
	}
	close(fileCh)
	return repo.Upload(ctx, fileCh)
}
