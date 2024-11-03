package cli

import (
	"context"
	"fmt"

	"github.com/livebud/cli"
	"github.com/matthewmueller/chunky/internal/commits"
)

type Cat struct {
	Repo     string
	Revision string
	Path     string
}

func (c *Cat) Command(cli cli.Command) cli.Command {
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

	commit, err := commits.Read(ctx, repo, in.Revision)
	if err != nil {
		return err
	}

	commitFile, ok := commit.Files[in.Path]
	if !ok {
		return fmt.Errorf("cli: file not found: %s", in.Path)
	}

	vfile, err := commits.ReadFile(ctx, repo, commitFile)
	if err != nil {
		return err
	}

	fmt.Fprintln(c.Stdout, string(vfile.Data))
	return nil
}
