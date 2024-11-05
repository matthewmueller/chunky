package cli

import (
	"context"
	"fmt"
	"path"

	"github.com/livebud/cli"
	"github.com/matthewmueller/chunky/internal/repos"
)

type CatTag struct {
	Repo string
	Tag  string
}

func (c *CatTag) command(cli cli.Command) cli.Command {
	cmd := cli.Command("cat-tag", "show a tag")
	cmd.Arg("repo", "repository to show").String(&c.Repo)
	cmd.Arg("tag", "tag to show").String(&c.Tag)
	return cmd
}

func (c *CLI) CatTag(ctx context.Context, in *CatTag) error {
	repo, err := c.loadRepo(in.Repo)
	if err != nil {
		return err
	}

	file, err := repos.Download(ctx, repo, path.Join("tags", in.Tag))
	if err != nil {
		return err
	}

	fmt.Fprintln(c.Stdout, string(file.Data))
	return nil
}
