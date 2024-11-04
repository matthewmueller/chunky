package cli

import (
	"context"
	"fmt"

	"github.com/livebud/cli"
	"github.com/matthewmueller/chunky/internal/commits"
	"github.com/matthewmueller/virt"
)

type Show struct {
	Repo     string
	Revision string
}

func (s *Show) command(cli cli.Command) cli.Command {
	cmd := cli.Command("show", "show a revision")
	cmd.Arg("repo", "repository to show").String(&s.Repo)
	cmd.Arg("revision", "revision to show").String(&s.Revision)
	return cmd
}

func (c *CLI) Show(ctx context.Context, in *Show) error {
	repo, err := c.loadRepo(in.Repo)
	if err != nil {
		return err
	}
	commit, err := commits.Read(ctx, repo, in.Revision)
	if err != nil {
		return err
	}
	fsys := virt.Map{}
	for _, file := range commit.Files() {
		fsys[file.Path] = ""
	}
	tree, err := virt.Print(fsys)
	if err != nil {
		return err
	}
	fmt.Fprintln(c.Stdout, tree)
	return nil
}
