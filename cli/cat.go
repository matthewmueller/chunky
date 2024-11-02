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

func findCommitFile(commit *commits.Commit, path string) *commits.File {
	for _, file := range commit.Files {
		if file.Path == path {
			return file
		}
	}
	return nil
}

func (c *CLI) Cat(ctx context.Context, in *Cat) error {
	repo, err := c.loadRepo(in.Repo)
	if err != nil {
		return err
	}
	commit, err := loadCommit(ctx, repo, in.Revision)
	if err != nil {
		return err
	}

	commitFile := findCommitFile(commit, in.Path)
	if commitFile == nil {
		return fmt.Errorf("cli: file not found: %s", in.Path)
	}

	vfile, err := downloadCommitFile(ctx, repo, commitFile)
	if err != nil {
		return err
	}

	fmt.Fprintln(c.Stdout, string(vfile.Data))
	return nil
}
