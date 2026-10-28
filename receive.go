package chunky

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"

	"github.com/matthewmueller/chunky/internal/commits"
	"github.com/matthewmueller/chunky/repos"
	"github.com/matthewmueller/chunky/repos/stream"
)

// ErrStale means the upload references a pack or file the repository
// doesn't have, usually because the uploader's cache is from another
// repository. Clear the cache and retry.
var ErrStale = errors.New("chunky: stale upload")

// Receive an upload streamed by repos/stream into a repository. Packs are
// written as they arrive, but the commit and its tags are only written once
// the stream ends with the latest tag, so a cut stream leaves at most orphan
// packs.
func (c *Client) Receive(ctx context.Context, from io.Reader, to repos.Repo) (revision string, err error) {
	var commit *repos.File
	var tags []*repos.File
	latest := false
	if err := stream.Read(from, func(file *repos.File) error {
		dir, name := path.Split(file.Path)
		switch {
		case dir == "packs/" && commit == nil:
			return to.Upload(ctx, file)
		case dir == "commits/" && commit == nil:
			data, err := resolve(ctx, to, file.Data)
			if err != nil {
				return err
			}
			file.Data = data
			commit, revision = file, name
			return nil
		case dir == "tags/" && commit != nil:
			if string(file.Data) != revision {
				return fmt.Errorf("tag %q doesn't name commit %s", name, revision)
			}
			tags = append(tags, file)
			latest = latest || name == "latest"
			return nil
		default:
			return fmt.Errorf("unexpected file %q", file.Path)
		}
	}); err != nil {
		return "", fmt.Errorf("unable to receive upload: %w", err)
	}
	if !latest {
		return "", errors.New("upload ended before tagging the commit as latest")
	}

	for _, file := range append([]*repos.File{commit}, tags...) {
		if err := to.Upload(ctx, file); err != nil {
			return "", fmt.Errorf("unable to upload %q: %w", file.Path, err)
		}
	}
	return revision, nil
}

// resolve the packs of the files the commit references without one, from the
// files the repository has, then check it has every pack the commit needs
func resolve(ctx context.Context, repo repos.Repo, commitData []byte) ([]byte, error) {
	commit, err := commits.Unpack(commitData)
	if err != nil {
		return nil, err
	}
	var have map[file]string
	for _, f := range commit.Files() {
		if f.PackId != "" {
			continue
		}
		// Only index when the commit references files, since it reads every commit
		if have == nil {
			if have, err = index(ctx, repo); err != nil {
				return nil, err
			}
		}
		packId, ok := have[file{f.Path, f.Id}]
		if !ok {
			return nil, fmt.Errorf("%w: missing file %s", ErrStale, f.Path)
		}
		f.PackId = packId
	}

	des, err := repo.ReadDir("packs")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("unable to read packs: %w", err)
	}
	packs := make(map[string]bool, len(des))
	for _, de := range des {
		packs[de.Name()] = true
	}
	for _, pack := range commit.Packs() {
		if !packs[pack.ID] {
			return nil, fmt.Errorf("%w: missing pack %s", ErrStale, pack.ID)
		}
	}
	return commit.Pack()
}
