package cli

import (
	"context"
	"fmt"

	"github.com/livebud/cli"
	"github.com/matthewmueller/chunky/internal/commits"
	"github.com/matthewmueller/chunky/internal/packs"
	"github.com/matthewmueller/virt"
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

	// Load the commit
	commit, err := commits.Read(ctx, repo, in.Revision)
	if err != nil {
		return fmt.Errorf("cli: unable to load commit %q: %w", in.Revision, err)
	}

	// Download into a virtual tree
	tree := virt.Tree{}
	for _, commitPack := range commit.Packs() {
		pack, err := packs.Read(ctx, repo, commitPack.ID)
		if err != nil {
			return fmt.Errorf("cli: unable to download pack %q: %w", commitPack.ID, err)
		}
		for _, file := range commitPack.Files {
			packFile, err := pack.Read(file.Path)
			if err != nil {
				return fmt.Errorf("cli: unable to read file %q: %w", file.Path, err)
			}
			tree[file.Path] = &virt.File{
				Path:    file.Path,
				Data:    packFile.Data,
				Mode:    packFile.Mode,
				ModTime: packFile.ModTime,
			}
		}
	}

	// Write the virtual tree to the filesystem
	if in.Sync {
		return virt.SyncFS(tree, to)
	}
	return virt.WriteFS(tree, to)
}
