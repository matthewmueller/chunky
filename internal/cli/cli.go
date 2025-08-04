package cli

import (
	"bytes"
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
	"github.com/matthewmueller/chunky"
	"github.com/matthewmueller/chunky/internal/commits"
	"github.com/matthewmueller/chunky/internal/humanize"
	"github.com/matthewmueller/chunky/internal/tags"
	"github.com/matthewmueller/chunky/repos"
	"github.com/matthewmueller/chunky/repos/local"
	"github.com/matthewmueller/chunky/repos/sftp"
	"github.com/matthewmueller/logs"
	"github.com/matthewmueller/prompter"
	"github.com/matthewmueller/text"
	"github.com/matthewmueller/virt"
)

func Run() int {
	cli := Default()
	ctx := context.Background()
	err := cli.Parse(ctx, os.Args[1:]...)
	if err != nil {
		logs.ErrorContext(ctx, err.Error())
		return 1
	}
	return 0
}

func Default() *CLI {
	return &CLI{
		os.Stdout,
		".",
		prompter.Default(),
		color.Default(),
		"info",
		nil,
		nil,
	}
}

type CLI struct {
	Stdout io.Writer
	Dir    string
	Prompt *prompter.Prompt
	Color  color.Writer

	// global flag
	logLevel string

	// Set after parsing
	log    *slog.Logger
	chunky *chunky.Client
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
		return local.New(virt.OS(url.Path)), nil
	case "sftp", "ssh":
		return sftp.Dial(url)
	default:
		return nil, fmt.Errorf("cli: unsupported repo scheme: %s", url.Scheme)
	}
}

func (c *CLI) loadFS(path string) (repos.FS, error) {
	url, err := repos.Parse(path)
	if err != nil {
		return nil, fmt.Errorf("cli: parsing repo path: %w", err)
	}
	switch url.Scheme {
	case "file":
		return virt.OS(filepath.Join(c.Dir, url.Path)), nil
	case "sftp", "ssh":
		return sftp.Dial(url)
	default:
		return nil, fmt.Errorf("cli: unsupported repo scheme: %s", url.Scheme)
	}
}

func cacheDir() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("cli: getting user cache dir: %w", err)
	}
	dir := filepath.Join(cacheDir, "chunky")
	return dir, nil
}

func cacheName(repoUrl *url.URL) string {
	return text.Slug(repoUrl.String())
}

func (c *CLI) cacheDir(repoUrl *url.URL) (string, error) {
	cacheDir, err := cacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cacheDir, cacheName(repoUrl))
	return dir, nil
}

func (c *CLI) loadCache(repoUrl *url.URL) (repos.FS, error) {
	cacheDir, err := c.cacheDir(repoUrl)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("cli: creating cache dir: %w", err)
	}
	return virt.OS(cacheDir), nil
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

func formatTags(tags []*tags.Tag) string {
	if len(tags) == 0 {
		return ""
	}
	tagNames := make([]string, len(tags))
	for i, tag := range tags {
		tagNames[i] = tag.Name
	}
	return "[" + strings.Join(tagNames, ", ") + "]"
}

func formatCommit(writer io.Writer, color color.Writer, commit *commits.Commit, tagMap map[string][]*tags.Tag) {
	commitId := commit.ID()
	relTime := humanize.Time(commit.CreatedAt())
	tags := tagMap[commitId]
	size := humanize.Bytes(commit.Size())
	writer.Write([]byte(fmt.Sprintf("%s\t%s\t%s\t%s\t%+v\n", color.Green(commitId), color.Green(formatTags(tags)), size, commit.User(), color.Dim(relTime))))
}

func formatTag(writer io.Writer, color color.Writer, tag *tags.Tag, newest *commits.Commit) {
	b := new(bytes.Buffer)
	b.WriteString(color.Green(tag.Name))
	if len(tag.Commits) == 0 {
		writer.Write(b.Bytes())
		return
	}

	// Show the relative time of the newest commit
	b.WriteString("\t")
	relTime := humanize.Time(newest.CreatedAt())
	b.WriteString(color.Dim(relTime))

	// List each commit, up to 5
	b.WriteString("\t[")
	commitIds := tag.Commits
	// If more than 5, only show last 5
	if len(tag.Commits) > 5 {
		commitIds = tag.Commits[len(tag.Commits)-5:]
	}
	last := len(commitIds) - 1
	for i := last; i >= 0; i-- {
		commitId := commitIds[i]
		if i == last {
			b.WriteString(commitId)
		} else {
			b.WriteString(", ")
			b.WriteString(color.Dim(commitId))
		}
	}
	if len(tag.Commits) > 5 {
		b.WriteString(", ")
		b.WriteString(color.Dim("..."))
	}
	b.WriteString("]")
	b.WriteByte('\n')
	writer.Write(b.Bytes())
}

func (c *CLI) logger(logLevel string) (*slog.Logger, error) {
	level, err := logs.ParseLevel(logLevel)
	if err != nil {
		return nil, fmt.Errorf("cli: parsing log level: %w", err)
	}
	log := logs.New(logs.Filter(level, logs.Console(c.Stdout)))
	return log, nil
}

func (c *CLI) wrap(fn func(ctx context.Context) error) func(ctx context.Context) error {
	return func(ctx context.Context) (err error) {
		c.log, err = c.logger(c.logLevel)
		if err != nil {
			return err
		}
		c.chunky = chunky.New(c.log)
		return fn(ctx)
	}
}

func (c *CLI) Parse(ctx context.Context, args ...string) error {
	cli := cli.New("chunky", "efficiently store versioned data")
	cli.Flag("log", "log configures the log level").Enum(&c.logLevel, "debug", "info", "warn", "error").Default("info")

	{ // create <repo>
		in := &Create{}
		cmd := in.command(cli)
		cmd.Run(c.wrap(func(ctx context.Context) error {
			return c.Create(ctx, in)
		}))
	}

	{ // upload [--tag=<tag>] <from> <to>
		in := &Upload{}
		cmd := in.command(cli)
		cmd.Run(c.wrap(func(ctx context.Context) error {
			return c.Upload(ctx, in)
		}))

	}

	{ // download <from> <to> <revision> [subpaths...]
		in := &Download{}
		cmd := in.command(cli)
		cmd.Run(c.wrap(func(ctx context.Context) error {
			return c.Download(ctx, in)
		}))
	}

	{ // versions <repo>
		in := &List{}
		cmd := in.command(cli)
		cmd.Run(c.wrap(func(ctx context.Context) error {
			return c.List(ctx, in)
		}))
	}

	{ // show <repo> <revision>
		in := &Show{}
		cmd := in.command(cli)
		cmd.Run(c.wrap(func(ctx context.Context) error {
			return c.Show(ctx, in)
		}))
	}

	{ // cat <repo> <revision> <path>
		in := &Cat{}
		cmd := in.command(cli)
		cmd.Run(c.wrap(func(ctx context.Context) error {
			return c.Cat(ctx, in)
		}))
	}

	{ // cat-pack <repo> <pack>
		in := &CatPack{}
		cmd := in.command(cli)
		cmd.Run(c.wrap(func(ctx context.Context) error {
			return c.CatPack(ctx, in)
		}))
	}

	{ // cat-commit <repo> <commit>
		in := &CatCommit{}
		cmd := in.command(cli)
		cmd.Run(c.wrap(func(ctx context.Context) error {
			return c.CatCommit(ctx, in)
		}))
	}

	{ // cat-tag <repo> <tag>
		in := &CatTag{}
		cmd := in.command(cli)
		cmd.Run(c.wrap(func(ctx context.Context) error {
			return c.CatTag(ctx, in)
		}))
	}

	{ // tag <repo> <revision> <tag>
		in := &Tag{}
		cmd := in.command(cli)
		cmd.Run(c.wrap(func(ctx context.Context) error {
			return c.Tag(ctx, in)
		}))
	}

	{ // tags <repo>
		in := &Tags{}
		cmd := in.command(cli)
		cmd.Run(c.wrap(func(ctx context.Context) error {
			return c.Tags(ctx, in)
		}))
	}

	{ // cache prune <repo>
		in := &CachePrune{}
		cmd := in.command(cli)
		cmd.Run(c.wrap(func(ctx context.Context) error {
			return c.CachePrune(ctx, in)
		}))
	}

	{ // cache size [repo]
		in := &CacheSize{}
		cmd := in.command(cli)
		cmd.Run(c.wrap(func(ctx context.Context) error {
			return c.CacheSize(ctx, in)
		}))
	}

	return cli.Parse(ctx, args...)
}
