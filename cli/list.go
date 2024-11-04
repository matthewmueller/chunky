package cli

import (
	"context"
	"fmt"

	"github.com/livebud/cli"
	"github.com/matthewmueller/chunky/internal/commits"
	"github.com/matthewmueller/chunky/internal/tags"
)

type List struct {
	Repo string
}

func (l *List) Command(cli cli.Command) cli.Command {
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
	for _, commit := range commits {
		commitId := commit.ID()
		tags := tagMap[commitId]
		fmt.Fprintf(c.Stdout, "%s %s %+v\n", commitId, commit.Size(), tags)
	}
	return nil
}
