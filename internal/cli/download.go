package cli

import (
	"context"

	"github.com/livebud/cli"
	"github.com/matthewmueller/chunky"
)

type Download struct {
	From          string
	To            string
	Revision      string
	LimitDownload string
	Concurrency   *int
}

func (d *Download) command(cli cli.Command) cli.Command {
	cmd := cli.Command("download", "download a directory from a repository")
	cmd.Arg("from", "repository to download from").String(&d.From)
	cmd.Arg("to", "directory to download to").String(&d.To)
	cmd.Flag("revision", "revision to download").String(&d.Revision).Default("latest")
	cmd.Flag("limit-download", "limit bytes per second").String(&d.LimitDownload).Default("")
	cmd.Flag("concurrency", "number of concurrent downloads").Optional().Int(&d.Concurrency)
	return cmd
}

func (c *CLI) Download(ctx context.Context, in *Download) error {
	// Load the repository to download from
	repo, err := c.loadRepo(in.From)
	if err != nil {
		return err
	}

	// Load the virtual filesystem to download into
	to, err := c.loadFS(in.To)
	if err != nil {
		return err
	}

	// Download the directory
	return c.chunky.Download(ctx, &chunky.Download{
		From:          repo,
		To:            to,
		Revision:      in.Revision,
		LimitDownload: in.LimitDownload,
		Concurrency:   in.Concurrency,
	})
}
