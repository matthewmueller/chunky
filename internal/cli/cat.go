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
	LimitDownload string
}

func (c *Cat) command(cli cli.Command) cli.Command {
	cmd := cli.Command("cat", "show a file")
	cmd.Arg("repo", "repository to show").String(&c.Repo)
	cmd.Arg("path", "path to the file").String(&c.Path)
	cmd.Flag("revision", "revision to show").String(&c.Revision).Default("latest")
	cmd.Flag("limit-download", "limit bytes per second").String(&c.LimitDownload).Default("")
	return cmd
}

func (c *CLI) Cat(ctx context.Context, in *Cat) error {
	repo, err := c.loadRepo(in.Repo)
	if err != nil {
		return err
	}

	limiter := rate.New(0)
	if in.LimitDownload != "" {
		limitDownload, err := humanize.ParseBytes(in.LimitDownload)
		if err != nil {
			return fmt.Errorf("invalid limit-download: %s", err)
		}
		limiter = rate.New(int(limitDownload))
	}

	pr := packs.NewCachedReader(lru.New[*packs.Pack](512 * mib))
	pr.Limiter = limiter
	download := downloads.New(pr)

	return download.Cat(ctx, c.Stdout, repo, in.Revision, in.Path)
}
