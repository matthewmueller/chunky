package cli

import (
	"context"
	"text/tabwriter"

	"github.com/livebud/cli"
	"github.com/matthewmueller/chunky/internal/commits"
	"github.com/matthewmueller/chunky/internal/tags"
)

type List struct {
	Repo string
}

func (l *List) command(cli cli.Command) cli.Command {
	cmd := cli.Command("list", "list repository")
	cmd.Arg("repo", "repo to list from").String(&l.Repo)
	return cmd
}

func (c *CLI) List(ctx context.Context, in *List) error {
	repo, err := c.loadRepo(in.Repo)
	if err != nil {
		return err
	}
	tagMap, err := tags.ReadMap(ctx, repo)
	if err != nil {
		return err
	}
	commits, err := commits.ReadAll(ctx, repo)
	if err != nil {
		return err
	}
	writer := tabwriter.NewWriter(c.Stdout, 0, 0, 1, ' ', 0)
	for _, commit := range commits {
		formatCommit(writer, c.Color, commit, tagMap)
	}
	return writer.Flush()
}
