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
// receiver, which answers with the files it already has, so the sender only
// uploads the rest.
type Manifest map[string]string

// File is a file in a commit, with the pack that holds it
type File = commits.File

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

// Matches returns the files in the manifest that repo already has, from any
// commit, for Upload.Matches
func (c *Client) Matches(ctx context.Context, repo repos.Repo, m Manifest) ([]*File, error) {
	all, err := commits.ReadAll(ctx, repo)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return []*File{}, nil
		}
		return nil, fmt.Errorf("chunky: unable to read commits: %w", err)
	}
	matches := []*File{}
	matched := map[string]bool{}
	for _, commit := range all {
		for _, file := range commit.Files() {
			if m[file.Path] == file.Id && !matched[file.Path] {
				matched[file.Path] = true
				matches = append(matches, file)
			}
		}
	}
	return matches, nil
}
