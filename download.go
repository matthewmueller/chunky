package chunky

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/matthewmueller/chunky/internal/commits"
	"github.com/matthewmueller/chunky/internal/packs"
	"github.com/matthewmueller/chunky/internal/sha256"
	"github.com/matthewmueller/chunky/repos"
	"golang.org/x/sync/errgroup"
)

type Download struct {
	From         repos.Repo
	To           repos.FS
	Revision     string
	MaxCacheSize int64
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
	if d.MaxCacheSize < 0 {
		err = errors.Join(err, errors.New("invalid max cache size"))
	} else if d.MaxCacheSize == 0 {
		d.MaxCacheSize = 512 * miB
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

	pr := packs.NewReader(in.From, in.MaxCacheSize)

	eg, ictx := errgroup.WithContext(ctx)
	for _, file := range commit.Files() {
		file := file
		eg.Go(func() error {
			return c.downloadFile(ictx, in, pr, file)
		})
	}

	// Wait for all the files to finish downloading
	if err := eg.Wait(); err != nil {
		return fmt.Errorf("cli: unable to download files: %w", err)
	}

	return nil
}

func (c *Client) downloadFile(ctx context.Context, in *Download, pr *packs.Reader, cf *commits.File) (err error) {
	// Load the pack that contains the file chunk
	pack, err := pr.Read(ctx, cf.PackId)
	if err != nil {
		return fmt.Errorf("cli: unable to download pack %q: %w", cf.PackId, err)
	}
	// pack, ok := cache.Get(cf.PackId)
	// if !ok {
	// 	pack, err = packs.Read(ctx, in.From, cf.PackId)
	// 	if err != nil {
	// 		return fmt.Errorf("cli: unable to download pack %q: %w", cf.PackId, err)
	// 	}
	// 	cache.Add(cf.PackId, pack)
	// }

	// Find the file chunk within the pack
	fc := pack.Chunk(cf.Path)
	if fc == nil {
		return fmt.Errorf("cli: unable to find file %q in pack %q", cf.Path, cf.PackId)
	}

	// Create the file
	file, err := in.To.OpenFile(fc.Path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, fc.Mode)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("cli: unable to create file %q: %w", fc.Path, err)
		}
		if err := in.To.MkdirAll(filepath.Dir(fc.Path), 0755); err != nil {
			return fmt.Errorf("cli: unable to create directory %q: %w", fc.Path, err)
		}
		file, err = in.To.OpenFile(fc.Path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, fc.Mode)
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

	hash := sha256.New()
	for _, ref := range fc.Refs {
		pack, err := pr.Read(ctx, ref.Pack)
		if err != nil {
			return fmt.Errorf("cli: unable to download pack %q: %w", ref.Pack, err)
		}
		bc := pack.Chunk(ref.Hash)
		if bc == nil {
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
