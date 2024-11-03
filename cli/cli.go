package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/livebud/cli"
	"github.com/matthewmueller/chunky/internal/prompt"
	"github.com/matthewmueller/chunky/internal/repos"
	"github.com/matthewmueller/chunky/internal/repos/local"
	"github.com/matthewmueller/chunky/internal/repos/sftp"
	"github.com/matthewmueller/logs"
	"github.com/matthewmueller/virt"
	"github.com/restic/chunker"
)

func Run() int {
	log := logs.Default()
	cli := Default(log)
	ctx := context.Background()
	err := cli.Parse(ctx, os.Args[1:]...)
	if err != nil {
		log.ErrorContext(ctx, err.Error())
		return 1
	}
	return 0
}

func Default(log *slog.Logger) *CLI {
	return &CLI{
		Dir:    ".",
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Env:    map[string]string{},
	}
}

type CLI struct {
	Dir    string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Env    map[string]string
}

func (c *CLI) loadRepo(path string) (repos.Repo, error) {
	url, err := repos.Parse(path)
	if err != nil {
		return nil, fmt.Errorf("cli: parsing repo path: %w", err)
	}
	switch url.Scheme {
	case "file":
		return local.New(url.Path), nil
	case "sftp", "ssh":
		signer, err := sftp.Parse(url, c.onPassword)
		if err != nil {
			return nil, err
		}
		return sftp.Dial(url, signer)
	default:
		return nil, fmt.Errorf("cli: unsupported repo scheme: %s", url.Scheme)
	}
}

func (c *CLI) onPassword() (string, error) {
retry:
	password, err := prompt.PasswordMasked("Enter your SSH key password: ")
	if err != nil {
		return "", err
	} else if password == "" {
		goto retry
	}
	return password, nil
}

func (c *CLI) loadFS(path string) (virt.FS, error) {
	return virt.OS(path), nil
}

var pol = chunker.Pol(0x3DA3358B4DC173)

func (c *CLI) chunker(data []byte) *chunker.Chunker {
	return chunker.New(bytes.NewReader(data), pol)
}

func (c *CLI) Parse(ctx context.Context, args ...string) error {
	cli := cli.New("chunky", "efficiently store versioned data")

	{ // new <repo>
		new := &Create{}
		cmd := new.Command(cli)
		cmd.Run(func(ctx context.Context) error {
			return c.Create(ctx, new)
		})
	}

	{ // upload [--tag=<tag> --message=<msg>] <from> <to> [subpaths...]
		upload := &Upload{}
		cmd := upload.Command(cli)
		cmd.Run(func(ctx context.Context) error {
			return c.Upload(ctx, upload)
		})

	}

	{ // download [--ref=<ref=latest>] <from> <to> [subpaths...]
		download := &Download{}
		cmd := download.Command(cli)
		cmd.Run(func(ctx context.Context) error {
			return c.Download(ctx, download)
		})
	}

	{ // list
		list := &List{}
		cmd := list.Command(cli)
		cmd.Run(func(ctx context.Context) error {
			return c.List(ctx, list)
		})
	}

	{ // show <repo> <revision>
		show := &Show{}
		cmd := show.Command(cli)
		cmd.Run(func(ctx context.Context) error {
			return c.Show(ctx, show)
		})
	}

	{ // cat <repo> <revision> <path>
		cat := &Cat{}
		cmd := cat.Command(cli)
		cmd.Run(func(ctx context.Context) error {
			return c.Cat(ctx, cat)
		})
	}

	{ // tag <repo> <commit> <tag>
		tag := &Tag{}
		cmd := tag.Command(cli)
		cmd.Run(func(ctx context.Context) error {
			return c.Tag(ctx, tag)
		})
	}

	return cli.Parse(ctx, args...)
}
