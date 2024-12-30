package chunky

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/matthewmueller/chunky/internal/commits"
	"github.com/matthewmueller/chunky/internal/packs"
	"github.com/matthewmueller/chunky/repos"
	"github.com/matthewmueller/virt"
)

type Download struct {
	From     repos.Repo
	To       virt.FS
	Revision string
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
			if err := in.To.WriteFile(file.Path, packFile.Data, packFile.Mode); err != nil {
				if !errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("cli: unable to write file %q: %w", file.Path, err)
				}
				// Create the directory if it doesn't exist
				if err := in.To.MkdirAll(filepath.Dir(file.Path), 0755); err != nil {
					return fmt.Errorf("cli: unable to create directory %q: %w", file.Path, err)
				}
				// Retry writing the file
				if err := in.To.WriteFile(file.Path, packFile.Data, packFile.Mode); err != nil {
					return fmt.Errorf("cli: unable to write file %q: %w", file.Path, err)
				}
			}
		}
	}

	return nil
}
