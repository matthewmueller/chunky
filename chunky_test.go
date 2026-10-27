package chunky_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/matryer/is"
	"github.com/matthewmueller/chunky"
	"github.com/matthewmueller/chunky/repos/local"
	"github.com/matthewmueller/logs"
	"github.com/matthewmueller/virt"
)

func TestSymlink(t *testing.T) {
	is := is.New(t)
	log := logs.Default()
	chky := chunky.New(log)
	ctx := context.Background()

	from := virt.OS("testdata/symlink")
	to := local.New(virt.OS(t.TempDir()))

	revision, err := chky.Upload(ctx, &chunky.Upload{
		From:  from,
		To:    to,
		Cache: virt.OS(t.TempDir()),
	})
	is.NoErr(err)
	is.True(revision != "")

	dir := t.TempDir()
	err = chky.Download(ctx, &chunky.Download{
		From:     to,
		To:       virt.OS(dir),
		Revision: "latest",
	})
	is.NoErr(err)

	info, err := os.Lstat(filepath.Join(dir, "from.txt"))
	is.NoErr(err)
	is.Equal(info.Name(), "from.txt")
	is.Equal(info.Mode(), fs.FileMode(0755|fs.ModeSymlink))
	is.Equal(info.Size(), int64(len("to.txt")))
}

const kib = 1024
const mib = 1024 * kib

func makeData(amount int) []byte {
	data := make([]byte, amount)
	for i := range amount {
		data[i] = byte(i % 256)
	}
	return data
}

func TestLargeSmallFile(t *testing.T) {
	is := is.New(t)
	log := logs.Default()
	chky := chunky.New(log)
	ctx := context.Background()

	largeData := makeData(10 * mib)
	smallData := []byte("This is a small file.")
	modTime := time.Now()
	from := virt.Tree{
		"large.txt": &virt.File{
			Data:    largeData,
			Mode:    0644,
			ModTime: modTime,
		},
		"small.txt": &virt.File{
			Data:    smallData,
			Mode:    0644,
			ModTime: modTime,
		},
	}
	to := local.New(virt.OS(t.TempDir()))

	revision, err := chky.Upload(ctx, &chunky.Upload{
		From:  from,
		To:    to,
		Cache: virt.OS(t.TempDir()),
	})
	is.NoErr(err)
	is.True(revision != "")

	dir := t.TempDir()
	err = chky.Download(ctx, &chunky.Download{
		From:     to,
		To:       virt.OS(dir),
		Revision: "latest",
	})
	is.NoErr(err)

	data, err := os.ReadFile(filepath.Join(dir, "large.txt"))
	is.NoErr(err)
	is.Equal(data, largeData)
	stat, err := os.Stat(filepath.Join(dir, "large.txt"))
	is.NoErr(err)
	is.Equal(stat.Size(), int64(len(largeData)))
	is.Equal(stat.Mode(), fs.FileMode(0644))
	is.True(stat.ModTime().After(modTime))

	data, err = os.ReadFile(filepath.Join(dir, "small.txt"))
	is.NoErr(err)
	is.Equal(data, smallData)
	stat, err = os.Stat(filepath.Join(dir, "small.txt"))
	is.NoErr(err)
	is.Equal(stat.Size(), int64(len(smallData)))
	is.Equal(stat.Mode(), fs.FileMode(0644))
	is.True(stat.ModTime().After(modTime))
}

func TestPaths(t *testing.T) {
	is := is.New(t)
	log := logs.Default()
	chky := chunky.New(log)
	ctx := context.Background()

	modTime := time.Now()
	from := virt.Tree{
		"sub/a.txt": &virt.File{
			Data:    []byte("a"),
			Mode:    0644,
			ModTime: modTime,
		},
		"sub/b.txt": &virt.File{
			Data:    []byte("b"),
			Mode:    0644,
			ModTime: modTime,
		},
		"c.txt": &virt.File{
			Data:    []byte("c"),
			Mode:    0644,
			ModTime: modTime,
		},
		"d.txt": &virt.File{
			Data:    []byte("d"),
			Mode:    0644,
			ModTime: modTime,
		},
	}
	to := local.New(virt.OS(t.TempDir()))

	revision, err := chky.Upload(ctx, &chunky.Upload{
		From:  from,
		To:    to,
		Cache: virt.OS(t.TempDir()),
		Paths: []string{"sub", "d.txt"},
	})
	is.NoErr(err)
	is.True(revision != "")

	dir := t.TempDir()
	err = chky.Download(ctx, &chunky.Download{
		From:     to,
		To:       virt.OS(dir),
		Revision: "latest",
	})
	is.NoErr(err)

	data, err := os.ReadFile(filepath.Join(dir, "sub", "a.txt"))
	is.NoErr(err)
	is.Equal(string(data), "a")

	data, err = os.ReadFile(filepath.Join(dir, "sub", "b.txt"))
	is.NoErr(err)
	is.Equal(string(data), "b")

	data, err = os.ReadFile(filepath.Join(dir, "c.txt"))
	is.True(os.IsNotExist(err))
	is.Equal(data, nil)

	data, err = os.ReadFile(filepath.Join(dir, "d.txt"))
	is.NoErr(err)
	is.Equal(string(data), "d")

}

func TestUploadBase(t *testing.T) {
	is := is.New(t)
	log := logs.Default()
	chky := chunky.New(log)
	ctx := context.Background()

	repoDir := t.TempDir()
	to := local.New(virt.OS(repoDir))
	_, err := chky.Upload(ctx, &chunky.Upload{
		From: virt.Tree{
			"a.txt": &virt.File{Data: []byte("a"), Mode: 0644},
			"b.txt": &virt.File{Data: []byte("b"), Mode: 0644},
		},
		To:    to,
		Cache: virt.OS(t.TempDir()),
	})
	is.NoErr(err)

	// The base travels as bytes, like a client reading it from a host
	latest, err := chky.FindCommit(ctx, &chunky.FindCommit{Repo: to, Revision: "latest"})
	is.NoErr(err)
	data, err := latest.Pack()
	is.NoErr(err)
	base, err := chunky.UnpackCommit(data)
	is.NoErr(err)
	is.Equal(base.ID(), latest.ID())

	// A fresh cache only sends the changed file
	revision, err := chky.Upload(ctx, &chunky.Upload{
		From: virt.Tree{
			"a.txt": &virt.File{Data: []byte("a"), Mode: 0644},
			"b.txt": &virt.File{Data: []byte("b2"), Mode: 0644},
		},
		To:    to,
		Cache: virt.OS(t.TempDir()),
		Base:  base,
	})
	is.NoErr(err)
	packs, err := os.ReadDir(filepath.Join(repoDir, "packs"))
	is.NoErr(err)
	is.Equal(len(packs), 2)
	commit, err := chky.FindCommit(ctx, &chunky.FindCommit{Repo: to, Revision: revision})
	is.NoErr(err)
	a, ok := commit.File("a.txt")
	is.True(ok)
	baseA, ok := base.File("a.txt")
	is.True(ok)
	is.Equal(a.PackId, baseA.PackId)

	dir := t.TempDir()
	err = chky.Download(ctx, &chunky.Download{
		From:     to,
		To:       virt.OS(dir),
		Revision: "latest",
	})
	is.NoErr(err)
	b, err := os.ReadFile(filepath.Join(dir, "b.txt"))
	is.NoErr(err)
	is.Equal(string(b), "b2")
}

func TestFindCommitMissing(t *testing.T) {
	is := is.New(t)
	chky := chunky.New(logs.Default())
	_, err := chky.FindCommit(context.Background(), &chunky.FindCommit{
		Repo:     local.New(virt.OS(t.TempDir())),
		Revision: "latest",
	})
	is.True(errors.Is(err, fs.ErrNotExist))
}
