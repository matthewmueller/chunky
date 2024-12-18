package cli

import (
	"context"

	"github.com/livebud/cli"
	"github.com/matthewmueller/chunky"
	"github.com/matthewmueller/chunky/repos"
)

type Upload struct {
	From  string
	To    string
	Tags  []string
	Cache bool
}

func (u *Upload) command(cli cli.Command) cli.Command {
	cmd := cli.Command("upload", "upload a directory to a repository")
	cmd.Arg("from", "directory to upload").String(&u.From)
	cmd.Arg("repo", "repository to upload to").String(&u.To)
	cmd.Flag("tags", "tag the revision").Short('t').Optional().Strings(&u.Tags)
	return cmd
}

func (c *CLI) Upload(ctx context.Context, in *Upload) error {
	repoUrl, err := repos.Parse(in.To)
	if err != nil {
		return err
	}

	repo, err := c.loadRepoFromUrl(repoUrl)
	if err != nil {
		return err
	}

	cache, err := c.loadCache(repoUrl)
	if err != nil {
		return err
	}

	fsys, err := c.loadFS(in.From)
	if err != nil {
		return err
	}

	user, err := c.getUser()
	if err != nil {
		return err
	}

	return c.Chunky.Upload(ctx, &chunky.Upload{
		From:  fsys,
		To:    repo,
		Tags:  in.Tags,
		User:  user,
		Cache: cache,
	})
}
