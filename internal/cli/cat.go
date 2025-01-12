package cli

import (
	"context"
	"fmt"

	"github.com/dustin/go-humanize"
	"github.com/livebud/cli"
	"github.com/matthewmueller/chunky/internal/downloads"
	"github.com/matthewmueller/chunky/internal/lru"
	"github.com/matthewmueller/chunky/internal/packs"
	"github.com/matthewmueller/chunky/internal/rate"
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

	pr := packs.NewCachedReader(c.log, lru.New[*packs.Pack](c.log, 512*mib))

	// Set the download limit if provided
	if in.LimitDownload != nil {
		limitDownload, err := humanize.ParseBytes(*in.LimitDownload)
		if err != nil {
			return fmt.Errorf("invalid limit-download: %s", err)
		}
		pr.Limiter = rate.New(int(limitDownload))
	}

	download := downloads.New(pr)

	// Set the concurrency if provided
	if in.Concurrency != nil {
		download.Concurrency = *in.Concurrency
	}

	return download.Cat(ctx, c.Stdout, repo, in.Revision, in.Path)
}
