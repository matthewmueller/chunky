package cli

import (
	"context"

	"github.com/livebud/cli"
	"github.com/matthewmueller/chunky/internal/packs"
)

type CatPack struct {
	Repo string
	Pack string
}

func (c *CatPack) command(cli cli.Command) cli.Command {
	cmd := cli.Command("cat-pack", "show a pack").Advanced()
	cmd.Arg("repo", "repository to show").String(&c.Repo)
	cmd.Arg("pack", "pack to show").String(&c.Pack)
	return cmd
}

func (c *CLI) CatPack(ctx context.Context, in *CatPack) error {
	repo, err := c.loadRepo(in.Repo)
	if err != nil {
		return err
	}

	pack, err := packs.Read(ctx, repo, in.Pack)
	if err != nil {
		return err
	}

	return pack.Encode(c.Stdout)
}
