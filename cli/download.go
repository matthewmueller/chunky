package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/livebud/cli"
	"github.com/matthewmueller/chunky/internal/commits"
	"github.com/matthewmueller/virt"
)

type Download struct {
	From     string
	To       string
	Subpaths []string
	Revision string
	Sync     bool
}

func (d *Download) Command(cli cli.Command) cli.Command {
	cmd := cli.Command("download", "download a directory from a repository")
	cmd.Arg("from", "repository to download from").String(&d.From)
	cmd.Arg("revision", "revision to download").String(&d.Revision)
	cmd.Arg("to", "directory to download to").String(&d.To)
	cmd.Args("subpaths", "subpaths to download").Strings(&d.Subpaths).Default()
	cmd.Flag("sync", "sync the repository before downloading").Bool(&d.Sync).Default(false)
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
	checksum := sha256.New()
	for _, file := range commit.Files {
		vfile, err := commits.ReadFile(ctx, repo, file)
		if err != nil {
			return fmt.Errorf("cli: unable to download file %q: %w", file.Path, err)
		}
		tree[file.Path] = vfile
		checksum.Write(vfile.Data)
	}

	// Verify the checksum
	if commit.Checksum != hex.EncodeToString(checksum.Sum(nil)) {
		return fmt.Errorf("cli: checksum mismatch for commit %q", in.Revision)
	}

	// Write the virtual tree to the filesystem
	return virt.WriteFS(tree, to)
}
