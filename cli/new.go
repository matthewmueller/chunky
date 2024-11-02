package cli

import (
	"context"
	"io/fs"

	"github.com/livebud/cli"
	"github.com/matthewmueller/virt"
)

type New struct {
	Repo string
}

func (n *New) Command(cli cli.Command) cli.Command {
	cmd := cli.Command("new", "new repository")
	cmd.Arg("path", "path to the new repository").String(&n.Repo)
	return cmd
}

func (c *CLI) New(ctx context.Context, in *New) error {
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
