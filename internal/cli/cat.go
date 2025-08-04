package cli

import (
	"context"

	"github.com/livebud/cli"
	"github.com/matthewmueller/chunky"
)

type Cat struct {
	Repo          string
	Revision      string
	Path          string
	LimitDownload *string
	Concurrency   *int
}

func (c *Cat) command(cli cli.Command) cli.Command {
	cmd := cli.Command("cat", "show a file")
	cmd.Arg("repo", "repository to show").String(&c.Repo)
	cmd.Arg("path", "path to the file").String(&c.Path)
	cmd.Flag("revision", "revision to show").String(&c.Revision).Default("latest")
	cmd.Flag("limit-download", "limit bytes per second").Optional().String(&c.LimitDownload)
	cmd.Flag("concurrency", "number of concurrent downloads").Optional().Int(&c.Concurrency)
	return cmd
}

func (c *CLI) Cat(ctx context.Context, in *Cat) error {
	repo, err := c.loadRepo(in.Repo)
	if err != nil {
		return err
	}

	// Set the download limit if provided
	limitDownload := ""
	if in.LimitDownload != nil {
		limitDownload = *in.LimitDownload
	}

	// Download a file
	return c.chunky.Cat(ctx, &chunky.Cat{
		From:          repo,
		To:            c.Stdout,
		Revision:      in.Revision,
		LimitDownload: limitDownload,
		Concurrency:   in.Concurrency,
		Path:          in.Path,
	})
}
