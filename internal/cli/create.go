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
	if err := repo.Upload(ctx, &repos.File{
		Path: "commits",
		Mode: fs.ModeDir | 0755,
	}); err != nil {
		return err
	}
	if err := repo.Upload(ctx, &repos.File{
		Path: "packs",
		Mode: fs.ModeDir | 0755,
	}); err != nil {
		return err
	}
	if err := repo.Upload(ctx, &repos.File{
		Path: "tags",
		Mode: fs.ModeDir | 0755,
	}); err != nil {
		return err
	}
	// files := []*repos.File{
	// 	{
	// 	},
	// 	{
	// 		Path: "packs",
	// 		Mode: fs.ModeDir | 0755,
	// 	},
	// 	{
	// 		Path: "tags",
	// 		Mode: fs.ModeDir | 0755,
	// 	},
	// }
	// fileCh := make(chan *repos.File, 3)
	// fileCh <- &repos.File
	// fileCh <- &repos.File
	// fileCh <- &repos.File
	// close(fileCh)
	return nil
}
