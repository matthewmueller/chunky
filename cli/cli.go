package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/livebud/cli"
	"github.com/livebud/color"
	"github.com/matthewmueller/chunky/internal/commits"
	"github.com/matthewmueller/chunky/internal/humanize"
	"github.com/matthewmueller/chunky/internal/repos"
	"github.com/matthewmueller/chunky/internal/repos/local"
	"github.com/matthewmueller/chunky/internal/repos/sftp"
	"github.com/matthewmueller/logs"
	"github.com/matthewmueller/prompter"
	"github.com/matthewmueller/virt"
)

func Run() int {
	log := logs.Default()
	cli := &CLI{
		log,
		os.Stdout,
		".",
		prompter.Default(),
		color.Default(),
	}
	ctx := context.Background()
	err := cli.Parse(ctx, os.Args[1:]...)
	if err != nil {
		log.ErrorContext(ctx, err.Error())
		return 1
	}
	return 0
}

func Default() *CLI {
	return &CLI{
		Log:    logs.Default(),
		Stdout: os.Stdout,
		Dir:    ".",
		Prompt: prompter.Default(),
		Color:  color.Default(),
	}
}

type CLI struct {
	Log    *slog.Logger
	Stdout io.Writer
	Dir    string
	Prompt *prompter.Prompt
	Color  color.Writer
}

func (c *CLI) loadRepo(path string) (repos.Repo, error) {
	url, err := repos.Parse(path)
	if err != nil {
		return nil, fmt.Errorf("cli: parsing repo path: %w", err)
	}
	return c.loadRepoFromUrl(url)
}

func (c *CLI) loadRepoFromUrl(url *url.URL) (repos.Repo, error) {
	switch url.Scheme {
	case "file":
		return local.New(url.Path), nil
	case "sftp", "ssh":
		return sftp.Load(url)
	default:
		return nil, fmt.Errorf("cli: unsupported repo scheme: %s", url.Scheme)
	}
}

func (c *CLI) loadFS(path string) (virt.FS, error) {
	return virt.OS(filepath.Join(c.Dir, path)), nil
}

func (c *CLI) getUser() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("cli: getting current user: %w", err)
	}
	if u.Name != "" {
		return u.Name, nil
	}
	return u.Username, nil
}

func formatTags(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	return "(" + strings.Join(tags, ", ") + ")"
}

func formatCommit(writer io.Writer, color color.Writer, commit *commits.Commit, tagMap map[string][]string) {
	commitId := commit.ID()
	relTime := humanize.Time(commit.CreatedAt())
	tags := tagMap[commitId]
	size := humanize.Bytes(commit.Size())
	writer.Write([]byte(fmt.Sprintf("%s\t%s\t%s\t%s\t%+v\n", color.Green(commitId), color.Green(formatTags(tags)), size, commit.User(), color.Dim(relTime))))
}

func (c *CLI) Parse(ctx context.Context, args ...string) error {
	cli := cli.New("chunky", "efficiently store versioned data")

	{ // new <repo>
		new := &Create{}
		cmd := new.command(cli)
		cmd.Run(func(ctx context.Context) error {
			return c.Create(ctx, new)
		})
	}

	{ // upload [--tag=<tag>] <from> <to>
		upload := &Upload{}
		cmd := upload.command(cli)
		cmd.Run(func(ctx context.Context) error {
			return c.Upload(ctx, upload)
		})

	}

	{ // download <from> <to> <revision> [subpaths...]
		download := &Download{}
		cmd := download.command(cli)
		cmd.Run(func(ctx context.Context) error {
			return c.Download(ctx, download)
		})
	}

	{ // list <repo>
		list := &List{}
		cmd := list.command(cli)
		cmd.Run(func(ctx context.Context) error {
			return c.List(ctx, list)
		})
	}

	{ // show <repo> <revision>
		show := &Show{}
		cmd := show.command(cli)
		cmd.Run(func(ctx context.Context) error {
			return c.Show(ctx, show)
		})
	}

	{ // cat <repo> <revision> <path>
		cat := &Cat{}
		cmd := cat.command(cli)
		cmd.Run(func(ctx context.Context) error {
			return c.Cat(ctx, cat)
		})
	}

	{ // cat-pack <repo> <pack>
		catPack := &CatPack{}
		cmd := catPack.command(cli)
		cmd.Run(func(ctx context.Context) error {
			return c.CatPack(ctx, catPack)
		})
	}

	{ // cat-commit <repo> <commit>
		catCommit := &CatCommit{}
		cmd := catCommit.command(cli)
		cmd.Run(func(ctx context.Context) error {
			return c.CatCommit(ctx, catCommit)
		})
	}

	{ // cat-tag <repo> <tag>
		catTag := &CatTag{}
		cmd := catTag.command(cli)
		cmd.Run(func(ctx context.Context) error {
			return c.CatTag(ctx, catTag)
		})
	}

	{ // tag <repo> <revision> <tag>
		tag := &Tag{}
		cmd := tag.command(cli)
		cmd.Run(func(ctx context.Context) error {
			return c.Tag(ctx, tag)
		})
	}

	{ // clean <repo>
		clean := &Clean{}
		cmd := clean.command(cli)
		cmd.Run(func(ctx context.Context) error {
			return c.Clean(ctx, clean)
		})
	}

	return cli.Parse(ctx, args...)
}
