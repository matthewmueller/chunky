package cli

import (
	"context"

	"github.com/livebud/cli"
	"github.com/matthewmueller/chunky"
)

type Download struct {
	From     string
	To       string
	Revision string
	Sync     bool
}

func (d *Download) command(cli cli.Command) cli.Command {
	cmd := cli.Command("download", "download a directory from a repository")
	cmd.Arg("from", "repository to download from").String(&d.From)
	cmd.Arg("revision", "revision to download").String(&d.Revision)
	cmd.Arg("to", "directory to download to").String(&d.To)
	cmd.Flag("sync", "sync the directory").Bool(&d.Sync).Default(false)
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
	return c.Chunky.Download(ctx, &chunky.Download{
		From:     repo,
		To:       to,
		Revision: in.Revision,
		// Sync:     in.Sync,
	})
}
