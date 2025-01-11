package cli

import (
	"context"

	"github.com/livebud/cli"
	"github.com/matthewmueller/chunky/internal/downloads"
	"github.com/matthewmueller/chunky/internal/lru"
	"github.com/matthewmueller/chunky/internal/packs"
)

type Cat struct {
	Repo     string
	Revision string
	Path     string
}

func (c *Cat) command(cli cli.Command) cli.Command {
	cmd := cli.Command("cat", "show a file")
	cmd.Arg("repo", "repository to show").String(&c.Repo)
	cmd.Arg("revision", "revision to show").String(&c.Revision)
	cmd.Arg("path", "path to the file").String(&c.Path)
	return cmd
}

func (c *CLI) Cat(ctx context.Context, in *Cat) error {
	repo, err := c.loadRepo(in.Repo)
	if err != nil {
		return err
	}
	pr := packs.NewReader(lru.New[*packs.Pack](0))
	download := downloads.New(pr)
	return download.StreamFile(ctx, c.Stdout, repo, in.Revision, in.Path)
}
