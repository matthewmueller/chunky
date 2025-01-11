package downloads

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/matthewmueller/chunky/internal/commits"
	"github.com/matthewmueller/chunky/internal/packs"
	"github.com/matthewmueller/chunky/internal/sha256"
	"github.com/matthewmueller/chunky/repos"
	"golang.org/x/sync/errgroup"
)

func New(pr packs.Reader) *Download {
	return &Download{
		pr: pr,
	}
}

type Download struct {
	pr packs.Reader
}

func (d *Download) Repo(ctx context.Context, from repos.Repo, revision string, to repos.FS) error {
	commit, err := commits.Read(ctx, from, revision)
	if err != nil {
		return fmt.Errorf("downloads: unable to load commit %q: %w", revision, err)
	}
	eg, ctx := errgroup.WithContext(ctx)
	for _, file := range commit.Files() {
		file := file
		eg.Go(func() error {
			return d.downloadFile(ctx, from, to, file)
		})
	}
	if err := eg.Wait(); err != nil {
		return fmt.Errorf("downloads: unable to download commit %q: %w", revision, err)
	}
	return nil
}

func (d *Download) downloadFile(ctx context.Context, from repos.Repo, to repos.FS, cf *commits.File) error {
	// Load the pack that contains the file chunk
	pack, err := d.pr.Read(ctx, from, cf.PackId)
	if err != nil {
		return fmt.Errorf("cli: unable to download pack %q: %w", cf.PackId, err)
	}

	// Find the file chunk within the pack
	fc, ok := pack.Chunk(cf.Path)
	if !ok {
		return fmt.Errorf("cli: unable to find file %q in pack %q", cf.Path, cf.PackId)
	}

	// Create the file
	file, err := to.OpenFile(fc.Path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, fc.Mode)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("cli: unable to create file %q: %w", fc.Path, err)
		}
		if err := to.MkdirAll(filepath.Dir(fc.Path), 0755); err != nil {
			return fmt.Errorf("cli: unable to create directory %q: %w", fc.Path, err)
		}
		file, err = to.OpenFile(fc.Path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, fc.Mode)
		if err != nil {
			return fmt.Errorf("cli: unable to create file %q: %w", fc.Path, err)
		}
	}
	defer file.Close()

	// If we have the data upfront, write it to the file and return early
	if fc.Data != nil || fc.Size == 0 {
		// Check the hash
		if sha256.Hash(fc.Data) != fc.Hash {
			return fmt.Errorf("cli: hash mismatch for file %q", fc.Path)
		}
		if _, err := file.Write(fc.Data); err != nil {
			return fmt.Errorf("cli: unable to write file %q: %w", fc.Path, err)
		}
		return nil
	}

	// Reconstruct the blob chunks into a single file
	hash := sha256.New()
	for _, ref := range fc.Refs {
		pack, err := d.pr.Read(ctx, from, ref.Pack)
		if err != nil {
			return fmt.Errorf("cli: unable to download pack %q: %w", ref.Pack, err)
		}
		bc, ok := pack.Chunk(ref.Hash)
		if !ok {
			return fmt.Errorf("cli: unable to find chunk %q in pack %q", ref.Hash, ref.Pack)
		}
		if _, err := file.Write(bc.Data); err != nil {
			return fmt.Errorf("cli: unable to write file %q: %w", fc.Path, err)
		}
		if _, err := hash.Write(bc.Data); err != nil {
			return fmt.Errorf("cli: unable to hash blob %q: %w", ref.Hash, err)
		}
	}

	// Check the hash
	if hex.EncodeToString(hash.Sum(nil)) != fc.Hash {
		return fmt.Errorf("cli: hash mismatch for file %q", fc.Path)
	}

	return nil
}

func (d *Download) StreamFile(ctx context.Context, w io.Writer, repo repos.Repo, revision, path string) error {
	// Download the commit
	commit, err := commits.Read(ctx, repo, revision)
	if err != nil {
		return fmt.Errorf("downloads: unable to load commit %q: %w", revision, err)
	}

	// Find the file within the commit
	cf, ok := commit.File(path)
	if !ok {
		return fmt.Errorf("downloads: unable to find file %q in commit %q", path, revision)
	}

	// Load the pack that contains the file chunk
	pack, err := d.pr.Read(ctx, repo, cf.PackId)
	if err != nil {
		return fmt.Errorf("cli: unable to download pack %q: %w", cf.PackId, err)
	}

	// Find the file chunk within the pack
	fc, ok := pack.Chunk(cf.Path)
	if !ok {
		return fmt.Errorf("cli: unable to find file %q in pack %q", cf.Path, cf.PackId)
	}

	// If we have the data upfront, return the file early
	if fc.Data != nil || fc.Size == 0 {
		// Check the hash
		if sha256.Hash(fc.Data) != fc.Hash {
			return fmt.Errorf("cli: hash mismatch for file %q", fc.Path)
		}
		if _, err := w.Write(fc.Data); err != nil {
			return fmt.Errorf("cli: unable to write file %q: %w", fc.Path, err)
		}
		return nil
	}

	// Write the chunks one-by-one to the writer
	hash := sha256.New()
	for _, ref := range fc.Refs {
		pack, err := d.pr.Read(ctx, repo, ref.Pack)
		if err != nil {
			return fmt.Errorf("cli: unable to download pack %q: %w", ref.Pack, err)
		}
		bc, ok := pack.Chunk(ref.Hash)
		if !ok {
			return fmt.Errorf("cli: unable to find chunk %q in pack %q", ref.Hash, ref.Pack)
		}
		if _, err := w.Write(bc.Data); err != nil {
			return fmt.Errorf("cli: unable to write file %q: %w", fc.Path, err)
		}
		if _, err := hash.Write(bc.Data); err != nil {
			return fmt.Errorf("cli: unable to hash blob %q: %w", ref.Hash, err)
		}
	}

	// Check the hash
	if hex.EncodeToString(hash.Sum(nil)) != fc.Hash {
		return fmt.Errorf("cli: hash mismatch for file %q", fc.Path)
	}

	return nil
}
