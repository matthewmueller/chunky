package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"slices"

	"github.com/livebud/cli"
	"github.com/matthewmueller/chunky"
	"github.com/matthewmueller/chunky/repos"
	"github.com/matthewmueller/chunky/repos/stream"
)

type Upload struct {
	From        string
	To          string
	Tags        []string
	Paths       []string
	Cache       bool
	LimitUpload string
	Concurrency *int
	Base        string
}

func (u *Upload) command(cli cli.Command) cli.Command {
	cmd := cli.Command("upload", "upload a directory to a repository")
	cmd.Arg("from", "directory to upload, or - to receive a stream from stdin").String(&u.From)
	cmd.Arg("repo", "repository to upload to, or - to stream to stdout").String(&u.To)
	cmd.Flag("tags", "tag the revision").Short('t').Optional().Strings(&u.Tags)
	cmd.Flag("paths", "subpaths to upload").Strings(&u.Paths).Default(".")
	cmd.Flag("limit-upload", "limit bytes per second").String(&u.LimitUpload).Default("")
	cmd.Flag("concurrency", "number of concurrent uploads").Optional().Int(&u.Concurrency)
	cmd.Flag("base", "repository to read the latest commit from, when the upload can't read it").String(&u.Base).Default("")
	return cmd
}

func (c *CLI) Upload(ctx context.Context, in *Upload) error {
	if in.From == "-" {
		return c.receive(ctx, in)
	}

	repoUrl, err := repos.Parse(in.To)
	if err != nil {
		return err
	}

	// Stream to stdout for another chunky upload to receive, e.g. over ssh
	var to repos.Writer
	if in.To == "-" {
		to = stream.New(nopCloser{c.Stdout})
	} else if to, err = c.loadRepoFromUrl(repoUrl); err != nil {
		return err
	}
	defer to.Close()

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

	// Upload on top of the latest commit, read from a repository we can read
	var base *chunky.Commit
	if in.Base != "" {
		baseRepo, err := c.loadRepo(in.Base)
		if err != nil {
			return err
		}
		defer baseRepo.Close()
		base, err = c.chunky.FindCommit(ctx, baseRepo, "latest")
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}

	revision, err := c.chunky.Upload(ctx, &chunky.Upload{
		From:        fsys,
		To:          to,
		Tags:        in.Tags,
		Paths:       in.Paths,
		User:        user,
		Cache:       cache,
		Base:        base,
		LimitUpload: in.LimitUpload,
		Concurrency: in.Concurrency,
	})
	if err != nil {
		return err
	}
	if err := to.Close(); err != nil {
		return err
	}

	// Stdout is taken by the stream
	if in.To != "-" {
		fmt.Fprintln(c.Stdout, revision)
	}
	return nil
}

// receive a stream from another chunky upload on stdin
func (c *CLI) receive(ctx context.Context, in *Upload) error {
	if in.To == "-" {
		return errors.New("cli: upload can't stream from stdin to stdout")
	}
	// The stream already carries its tags and was packed by the sender
	if len(in.Tags) > 0 || !slices.Equal(in.Paths, []string{"."}) || in.LimitUpload != "" || in.Concurrency != nil || in.Base != "" {
		return errors.New("cli: --tags, --paths, --limit-upload, --concurrency and --base don't apply when uploading from stdin")
	}

	repo, err := c.loadRepo(in.To)
	if err != nil {
		return err
	}
	defer repo.Close()

	revision, err := c.chunky.Receive(ctx, c.Stdin, repo)
	if errors.Is(err, chunky.ErrStale) {
		return fmt.Errorf("%w. Run `chunky cache-prune -` where you uploaded from and retry", err)
	} else if err != nil {
		return err
	}

	fmt.Fprintln(c.Stdout, revision)
	return nil
}

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }
