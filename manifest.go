package chunky

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/matthewmueller/chunky/internal/commits"
	"github.com/matthewmueller/chunky/repos"
)

// Manifest maps file paths to their hashes. A sender sends its manifest to a
// receiver, which answers with the files it's missing, so the sender only
// uploads those.
type Manifest map[string]string

// Manifest hashes the files the upload would send, without sending them
func (c *Client) Manifest(ctx context.Context, in *Upload) (Manifest, error) {
	if err := in.validate(); err != nil {
		return nil, fmt.Errorf("invalid upload input: %w", err)
	}
	m := Manifest{}
	if err := in.walk(func(fpath, hash string) error {
		m[fpath] = hash
		return nil
	}); err != nil {
		return nil, err
	}
	return m, nil
}

// Missing returns the files in the manifest that repo doesn't have, for
// Upload.Missing
func (c *Client) Missing(ctx context.Context, repo repos.Repo, m Manifest) (Manifest, error) {
	have, err := index(ctx, repo)
	if err != nil {
		return nil, err
	}
	missing := Manifest{}
	for fpath, hash := range m {
		if _, ok := have[file{fpath, hash}]; !ok {
			missing[fpath] = hash
		}
	}
	return missing, nil
}

// file is a file's path and hash. Packs look files up by path, so the hash
// alone doesn't find a file.
type file struct{ path, hash string }

// index the files in every commit of repo to the pack that holds them
func index(ctx context.Context, repo repos.Repo) (map[file]string, error) {
	have := map[file]string{}
	all, err := commits.ReadAll(ctx, repo)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return have, nil
		}
		return nil, fmt.Errorf("chunky: unable to index the repository: %w", err)
	}
	for _, commit := range all {
		for _, f := range commit.Files() {
			have[file{f.Path, f.Id}] = f.PackId
		}
	}
	return have, nil
}
