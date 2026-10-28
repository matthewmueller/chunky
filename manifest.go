package chunky

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"slices"
	"strings"

	"github.com/matthewmueller/chunky/internal/commits"
	"github.com/matthewmueller/chunky/repos"
)

// Manifest lists files by path and hash. A sender sends its manifest to a
// receiver, which answers with the files it's missing, so the sender only
// uploads those.
type Manifest struct {
	files map[string]string // path -> hash
}

// Has reports whether the manifest lists the file at path with hash
func (m *Manifest) Has(path, hash string) bool {
	return m.files[path] == hash
}

// WriteTo writes the manifest as "<hash> <path>" lines, sorted by path
func (m *Manifest) WriteTo(w io.Writer) (n int64, err error) {
	bw := bufio.NewWriter(w)
	for _, fpath := range slices.Sorted(maps.Keys(m.files)) {
		nn, err := fmt.Fprintf(bw, "%s %s\n", m.files[fpath], fpath)
		n += int64(nn)
		if err != nil {
			return n, err
		}
	}
	return n, bw.Flush()
}

// ReadManifest reads a manifest written by WriteTo
func ReadManifest(r io.Reader) (*Manifest, error) {
	m := &Manifest{map[string]string{}}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		hash, fpath, ok := strings.Cut(scanner.Text(), " ")
		if !ok || hash == "" || !fs.ValidPath(fpath) {
			return nil, fmt.Errorf("chunky: invalid manifest line %q", scanner.Text())
		}
		m.files[fpath] = hash
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("chunky: unable to read manifest: %w", err)
	}
	return m, nil
}

// Manifest hashes the files the upload would send. To and Cache are unused.
func (c *Client) Manifest(ctx context.Context, in *Upload) (*Manifest, error) {
	if err := in.validate(); err != nil {
		return nil, fmt.Errorf("invalid upload input: %w", err)
	}
	m := &Manifest{map[string]string{}}
	if err := in.walk(func(fpath, hash string) error {
		if strings.Contains(fpath, "\n") {
			return fmt.Errorf("chunky: unable to list %q in a manifest", fpath)
		}
		m.files[fpath] = hash
		return nil
	}); err != nil {
		return nil, err
	}
	return m, nil
}

// Missing returns the files in the manifest that repo doesn't have, for
// Upload.Missing
func (c *Client) Missing(ctx context.Context, repo repos.Repo, m *Manifest) (*Manifest, error) {
	have, err := index(ctx, repo)
	if err != nil {
		return nil, err
	}
	missing := &Manifest{map[string]string{}}
	for fpath, hash := range m.files {
		if _, ok := have[indexKey(fpath, hash)]; !ok {
			missing.files[fpath] = hash
		}
	}
	return missing, nil
}

// index the files in every commit of repo by path and hash, to the pack that
// holds them. Packs look files up by path, so the path is part of the key.
func index(ctx context.Context, repo repos.Repo) (map[string]string, error) {
	have := map[string]string{}
	all, err := commits.ReadAll(ctx, repo)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return have, nil
		}
		return nil, fmt.Errorf("chunky: unable to index the repository: %w", err)
	}
	for _, commit := range all {
		for _, file := range commit.Files() {
			have[indexKey(file.Path, file.Id)] = file.PackId
		}
	}
	return have, nil
}

func indexKey(fpath, hash string) string {
	return fpath + "\x00" + hash
}
