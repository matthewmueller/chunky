package chunky

import (
	"context"
	"errors"
	"fmt"

	"github.com/matthewmueller/chunky/internal/commits"
	"github.com/matthewmueller/chunky/internal/packs"
	"github.com/matthewmueller/chunky/internal/repos"
	"github.com/matthewmueller/virt"
)

type Download struct {
	From     repos.Repo
	To       virt.FS
	Revision string
	Sync     bool
}

func (d *Download) validate() (err error) {
	// Required fields
	if d.From == nil {
		err = errors.Join(err, errors.New("missing 'from' repository"))
	}
	if d.To == nil {
		err = errors.Join(err, errors.New("missing 'to' writable filesystem"))
	}
	if d.Revision == "" {
		err = errors.Join(err, errors.New("missing 'revision'"))
	}

	return err
}

// Download a directory from a repository at a specific revision
func (c *Client) Download(ctx context.Context, in *Download) error {
	if err := in.validate(); err != nil {
		return err
	}

	// Load the commit
	commit, err := commits.Read(ctx, in.From, in.Revision)
	if err != nil {
		return fmt.Errorf("cli: unable to load commit %q: %w", in.Revision, err)
	}

	// Download into a virtual tree
	tree := virt.Tree{}
	for _, commitPack := range commit.Packs() {
		pack, err := packs.Read(ctx, in.From, commitPack.ID)
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
		return virt.SyncFS(tree, in.To)
	}
	return virt.WriteFS(tree, in.To)
}
