package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"text/tabwriter"

	"github.com/livebud/cli"
	"github.com/matthewmueller/chunky/internal/tags"
)

type Tags struct {
	Repo string
}

func (in *Tags) command(cli cli.Command) cli.Command {
	cmd := cli.Command("tags", "list tags")
	cmd.Arg("repo", "repo to list from").String(&in.Repo)
	return cmd
}

func (c *CLI) Tags(ctx context.Context, in *Tags) error {
	repo, err := c.loadRepo(in.Repo)
	if err != nil {
		return err
	}
	tags, err := tags.ReadAll(ctx, repo)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("cli: repo %q not found", in.Repo)
	} else if err != nil {
		return err
	}
	writer := tabwriter.NewWriter(c.Stdout, 0, 0, 1, ' ', 0)
	for _, tag := range tags {
		formatTag(writer, c.Color, tag)
	}
	return writer.Flush()
}
