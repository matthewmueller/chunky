package cli

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/livebud/cli"
	"github.com/matthewmueller/chunky/internal/commits"
	"github.com/matthewmueller/chunky/internal/tags"
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
	repo, err := c.loadRepo(ctx, in.Repo)
	if err != nil {
		return err
	}

	// Write the commit
	tagMap, err := tags.ReadMap(ctx, repo)
	if err != nil {
		return err
	}
	commit, err := commits.Read(ctx, repo, in.Revision)
	if err != nil {
		return err
	}
	writer := tabwriter.NewWriter(c.Stdout, 0, 0, 2, ' ', 0)
	formatCommit(writer, c.Color, commit, tagMap)
	if err := writer.Flush(); err != nil {
		return err
	}

	// Write the file tree
	fsys := virt.Map{}
	for _, file := range commit.Files() {
		fsys[file.Path] = ""
	}
	tree, err := virt.Print(fsys)
	if err != nil {
		return err
	}
	lines := strings.Split(tree, "\n")
	lines = lines[1:]
	tree = strings.Join(lines, "\n")
	fmt.Fprintln(c.Stdout, tree)
	return nil
}
