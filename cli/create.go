package cli

import (
	"context"
	"io/fs"

	"github.com/livebud/cli"
	"github.com/matthewmueller/virt"
)

type Create struct {
	Repo string
}

func (c *Create) Command(cli cli.Command) cli.Command {
	cmd := cli.Command("create", "create a new repository")
	cmd.Arg("path", "path to the new repository").String(&c.Repo)
	return cmd
}

func (c *CLI) Create(ctx context.Context, in *Create) error {
	repo, err := c.loadRepo(in.Repo)
	if err != nil {
		return err
	}
	tree := virt.Tree{}
	tree["commits"] = &virt.File{
		Mode: fs.ModeDir | 0755,
	}
	tree["objects"] = &virt.File{
		Mode: fs.ModeDir | 0755,
	}
	tree["tags"] = &virt.File{
		Mode: fs.ModeDir | 0755,
	}
	return repo.Upload(ctx, tree)
}
