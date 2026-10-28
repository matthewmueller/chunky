package caches_test

import (
	"context"
	"errors"
	"io/fs"
	"path"
	"testing"
	"time"

	"github.com/matryer/is"
	"github.com/matthewmueller/chunky/internal/caches"
	"github.com/matthewmueller/chunky/internal/commits"
	"github.com/matthewmueller/chunky/repos/local"
	"github.com/matthewmueller/logs"
	"github.com/matthewmueller/virt"
)

func packCommit(t testing.TB, filePath string) (id string, data []byte) {
	t.Helper()
	is := is.New(t)
	commit := commits.New("test", time.Now().UTC())
	commit.Add(&commits.File{Path: filePath, Id: filePath + "-hash", PackId: "pack"})
	data, err := commit.Pack()
	is.NoErr(err)
	return commit.ID(), data
}

func TestDownloadPrunesStale(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	repoFS := virt.OS(t.TempDir())
	cacheFS := virt.OS(t.TempDir())

	// The repo and the cache share one commit, and the cache also holds a
	// commit that's no longer in the repo
	keptId, keptData := packCommit(t, "kept.txt")
	staleId, staleData := packCommit(t, "stale.txt")
	is.NoErr(repoFS.MkdirAll("commits", 0755))
	is.NoErr(repoFS.WriteFile(path.Join("commits", keptId), keptData, 0644))
	is.NoErr(cacheFS.WriteFile(keptId, keptData, 0644))
	is.NoErr(cacheFS.WriteFile(staleId, staleData, 0644))

	cache, err := caches.Load(logs.Discard(), cacheFS)
	is.NoErr(err)
	is.NoErr(cache.Download(ctx, local.New(repoFS)))

	kept, ok := cache.Get("kept.txt", "kept.txt-hash")
	is.True(ok)
	is.True(kept != nil)
	stale, ok := cache.Get("stale.txt", "stale.txt-hash")
	is.True(!ok)
	is.Equal(stale, nil)
	info, err := fs.Stat(cacheFS, staleId)
	is.True(errors.Is(err, fs.ErrNotExist))
	is.Equal(info, nil)
}
