package downloads

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/matthewmueller/chunky/internal/commits"
	"github.com/matthewmueller/chunky/internal/packs"
	"github.com/matthewmueller/chunky/internal/sha256"
	"github.com/matthewmueller/chunky/repos"
	"golang.org/x/sync/errgroup"
)

func New(pr packs.Reader) *Downloader {
	return &Downloader{
		pr: pr,
	}
}

type Downloader struct {
	pr          packs.Reader
	Concurrency int
}

// Download a revision from a repo to a filesystem
func (d *Downloader) Download(ctx context.Context, from repos.Repo, revision string, to repos.FS) error {
	commit, err := commits.Read(ctx, from, revision)
	if err != nil {
		return fmt.Errorf("downloads: unable to load commit %q: %w", revision, err)
	}
	// Download the files concurrently in batches based on the number of CPUs
	// TODO: consider simplifying with buffered channels
	for _, files := range splitFiles(commit.Files(), d.Concurrency) {
		eg, ctx := errgroup.WithContext(ctx)
		for _, file := range files {
			eg.Go(func() error {
				return d.downloadFile(ctx, from, to, file)
			})
		}
		if err := eg.Wait(); err != nil {
			return fmt.Errorf("downloads: unable to download revision %q: %w", revision, err)
		}
	}
	return nil
}

func splitFiles(files []*commits.File, size int) [][]*commits.File {
	if size == 0 {
		return [][]*commits.File{files}
	}
	var buckets [][]*commits.File
	for i := 0; i < len(files); i += size {
		buckets = append(buckets, files[i:min(i+size, len(files))])
	}
	return buckets
}

func (d *Downloader) downloadFile(ctx context.Context, from repos.Repo, to repos.FS, cf *commits.File) error {
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

	if fc.Mode&fs.ModeSymlink != 0 {
		if err := to.WriteFile(fc.Path, []byte(fc.Data), fc.Mode); err != nil {
			if errors.Is(err, fs.ErrExist) {
				if err := to.RemoveAll(fc.Path); err != nil {
					return fmt.Errorf("cli: unable to remove existing symlink %q: %w", fc.Path, err)
				}
			} else if !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("cli: unable to create symlink %q: %w", fc.Path, err)
			}
			if err := to.MkdirAll(filepath.Dir(fc.Path), 0755); err != nil {
				return fmt.Errorf("cli: unable to create directory %q: %w", fc.Path, err)
			}
			if err := to.WriteFile(fc.Path, []byte(fc.Data), fc.Mode); err != nil {
				return fmt.Errorf("cli: unable to create symlink %q: %w", fc.Path, err)
			}
		}
		return nil
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

	return d.writeFile(ctx, from, file, fc)
}

// Cat file data from a repo to a writer
func (d *Downloader) Cat(ctx context.Context, w io.Writer, repo repos.Repo, revision, path string) error {
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

	return d.writeFile(ctx, repo, w, fc)
}

// Write file data to a writer, downloading chunks as necessary and checking hashes
func (d *Downloader) writeFile(ctx context.Context, repo repos.Repo, w io.Writer, fc *packs.Chunk) error {
	hash := sha256.New(fc)

	// If we have the data upfront, return the file early
	if fc.Data != nil || fc.Size == 0 {
		// Check the hash
		if hash.String() != fc.Hash {
			return fmt.Errorf("cli: hash mismatch for file %q: expected %s, got %s", fc.Path, fc.Hash, hash.String())
		}
		if _, err := w.Write(fc.Data); err != nil {
			return fmt.Errorf("cli: unable to write file %q: %w", fc.Path, err)
		}
		return nil
	}

	// Write the chunks one-by-one to the writer
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
		if _, err := hash.Write(bc); err != nil {
			return fmt.Errorf("cli: unable to hash blob %q: %w", ref.Hash, err)
		}
	}

	// Check the hash
	if hash.String() != fc.Hash {
		return fmt.Errorf("cli: hash mismatch for chunked file %q: expected %s, got %s", fc.Path, fc.Hash, hash.String())
	}

	return nil
}
