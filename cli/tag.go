package cli

import (
	"context"
	"fmt"

	"github.com/livebud/cli"
	"github.com/matthewmueller/virt"
)

type Tag struct {
	Repo   string
	Commit string
	Tag    string
}

func (t *Tag) Command(cli cli.Command) cli.Command {
	cmd := cli.Command("tag", "tag a commit")
	cmd.Arg("repo", "repository to tag").String(&t.Repo)
	cmd.Arg("commit", "commit to tag").String(&t.Commit)
	cmd.Arg("tag", "tag to create").String(&t.Tag)
	return cmd
}

func (c *CLI) Tag(ctx context.Context, in *Tag) error {
	// Load the repository
	repo, err := c.loadRepo(in.Repo)
	if err != nil {
		return err
	}

	// Check that the commit exists
	if err := repo.Download(ctx, virt.Tree{}, fmt.Sprintf("commits/%s", in.Commit)); err != nil {
		return err
	}

	// Create the tag file
	tree := virt.Tree{
		fmt.Sprintf("tags/%s", in.Tag): &virt.File{
			Data: []byte(in.Commit),
			Mode: 0644,
		},
	}

	// Upload the tag file
	return repo.Upload(ctx, tree)
}
