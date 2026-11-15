package chunky_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/matryer/is"
	"github.com/matthewmueller/chunky"
	"github.com/matthewmueller/chunky/repos"
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
		From: from,
		To:   to,
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
		From: from,
		To:   to,
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

func TestFindCommitMissing(t *testing.T) {
	is := is.New(t)
	chky := chunky.New(logs.Default())
	commit, err := chky.FindCommit(context.Background(), &chunky.FindCommit{Repo: local.New(virt.OS(t.TempDir())), Revision: "latest"})
	is.True(errors.Is(err, fs.ErrNotExist))
	is.Equal(commit, nil)
}

func tree(a, b string) virt.Tree {
	return virt.Tree{
		"a.txt": &virt.File{Data: []byte(a), Mode: 0644},
		"b.txt": &virt.File{Data: []byte(b), Mode: 0644},
	}
}

// deploy uploads to host and returns the revision
func deploy(t testing.TB, host repos.Repo, in *chunky.Upload) (string, error) {
	t.Helper()
	in.To = host
	return chunky.New(logs.Discard()).Upload(context.Background(), in)
}

func countPacks(t testing.TB, dir string) int {
	t.Helper()
	is := is.New(t)
	des, err := os.ReadDir(filepath.Join(dir, "packs"))
	is.True(err == nil || errors.Is(err, fs.ErrNotExist))
	return len(des)
}

func readLatest(t testing.TB, host repos.Repo) map[string]string {
	t.Helper()
	is := is.New(t)
	dir := t.TempDir()
	is.NoErr(chunky.New(logs.Discard()).Download(context.Background(), &chunky.Download{
		From:     host,
		To:       virt.OS(dir),
		Revision: "latest",
	}))
	a, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	is.NoErr(err)
	b, err := os.ReadFile(filepath.Join(dir, "b.txt"))
	is.NoErr(err)
	return map[string]string{"a.txt": string(a), "b.txt": string(b)}
}

func TestFindCommitTag(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	chky := chunky.New(logs.Discard())
	host := local.New(virt.OS(t.TempDir()))
	first, err := deploy(t, host, &chunky.Upload{From: tree("a", "b")})
	is.NoErr(err)
	second, err := deploy(t, host, &chunky.Upload{From: tree("a", "b2")})
	is.NoErr(err)

	// A tag lists every commit it has named, and resolves to the newest
	is.NoErr(chky.TagRevision(ctx, &chunky.TagRevision{Repo: host, Tag: "v1", Revision: first}))
	commit, err := chky.FindCommit(ctx, &chunky.FindCommit{Repo: host, Revision: "v1"})
	is.NoErr(err)
	is.Equal(commit.ID(), first)
	is.NoErr(chky.TagRevision(ctx, &chunky.TagRevision{Repo: host, Tag: "v1", Revision: second}))
	commit, err = chky.FindCommit(ctx, &chunky.FindCommit{Repo: host, Revision: "v1"})
	is.NoErr(err)
	is.Equal(commit.ID(), second)
}

// findTag returns the named tag from ListTags
func findTag(t testing.TB, host repos.Repo, name string) *chunky.Tag {
	t.Helper()
	is := is.New(t)
	allTags, err := chunky.New(logs.Discard()).ListTags(context.Background(), &chunky.ListTags{Repo: host})
	is.NoErr(err)
	for _, tag := range allTags {
		if tag.Name == name {
			return tag
		}
	}
	t.Fatalf("tag %q not found", name)
	return nil
}

func TestTagHistory(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	chky := chunky.New(logs.Discard())
	host := local.New(virt.OS(t.TempDir()))
	first, err := deploy(t, host, &chunky.Upload{From: tree("a", "b")})
	is.NoErr(err)
	second, err := deploy(t, host, &chunky.Upload{From: tree("a", "b2")})
	is.NoErr(err)

	// Release first, then second, then roll back to first
	before := time.Now().Add(-time.Second)
	is.NoErr(chky.TagRevision(ctx, &chunky.TagRevision{Repo: host, Tag: "hello", Revision: first, User: "Alice"}))
	is.NoErr(chky.TagRevision(ctx, &chunky.TagRevision{Repo: host, Tag: "hello", Revision: second, User: "Bob"}))
	is.NoErr(chky.TagRevision(ctx, &chunky.TagRevision{Repo: host, Tag: "hello", Revision: first, User: "Carol Smith"}))

	history := findTag(t, host, "hello").History
	is.Equal(len(history), 3)
	is.Equal(history[0].Commit, first)
	is.Equal(history[0].User, "Alice")
	is.Equal(history[1].Commit, second)
	is.Equal(history[1].User, "Bob")
	is.Equal(history[2].Commit, first)
	is.Equal(history[2].User, "Carol Smith")
	is.True(history[0].At.After(before))
	is.True(!history[1].At.Before(history[0].At))
	is.True(!history[2].At.Before(history[1].At))

	// The tag resolves to the rollback
	commit, err := chky.FindCommit(ctx, &chunky.FindCommit{Repo: host, Revision: "hello"})
	is.NoErr(err)
	is.Equal(commit.ID(), first)

	// A newline in the user can't forge another entry
	is.NoErr(chky.TagRevision(ctx, &chunky.TagRevision{Repo: host, Tag: "hello", Revision: second, User: "Eve\n" + first}))
	history = findTag(t, host, "hello").History
	is.Equal(len(history), 4)
	is.Equal(history[3].Commit, second)
	is.Equal(history[3].User, "Eve\n"+first)
}

func TestTagNewerFields(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	chky := chunky.New(logs.Discard())
	hostDir := t.TempDir()
	host := local.New(virt.OS(hostDir))
	first, err := deploy(t, host, &chunky.Upload{From: tree("a", "b")})
	is.NoErr(err)
	second, err := deploy(t, host, &chunky.Upload{From: tree("a", "b2")})
	is.NoErr(err)

	// A newer chunky added a field this version doesn't know about
	newer := `{"commit":"` + first + `","at":"2026-11-14T18:02:11Z","user":"Alice","reason":"rollback"}` + "\n"
	is.NoErr(host.Upload(ctx, &repos.File{Path: "tags/hello", Data: []byte(newer), Mode: 0644}))
	history := findTag(t, host, "hello").History
	is.Equal(len(history), 1)
	is.Equal(history[0].Commit, first)
	is.Equal(history[0].User, "Alice")

	// Appending keeps the field
	is.NoErr(chky.TagRevision(ctx, &chunky.TagRevision{Repo: host, Tag: "hello", Revision: second, User: "Bob"}))
	data, err := os.ReadFile(filepath.Join(hostDir, "tags", "hello"))
	is.NoErr(err)
	is.True(strings.HasPrefix(string(data), newer))
	is.Equal(findTag(t, host, "hello").Newest(), second)
}

func TestTagOldFormat(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	chky := chunky.New(logs.Discard())
	hostDir := t.TempDir()
	host := local.New(virt.OS(hostDir))
	first, err := deploy(t, host, &chunky.Upload{From: tree("a", "b")})
	is.NoErr(err)
	second, err := deploy(t, host, &chunky.Upload{From: tree("a", "b2")})
	is.NoErr(err)

	// Tags written before they recorded their history are only commit ids,
	// including timeids from before commits had ksuids. Their time is when the
	// commit was created, which both kinds of id record.
	old := "20241105044435\n" + first + "\n" + second + "\n"
	is.NoErr(host.Upload(ctx, &repos.File{Path: "tags/old", Data: []byte(old), Mode: 0644}))
	history := findTag(t, host, "old").History
	is.Equal(len(history), 3)
	is.Equal(history[0].Commit, "20241105044435")
	is.Equal(history[0].At, time.Date(2024, 11, 5, 4, 44, 35, 0, time.UTC))
	is.Equal(history[2].Commit, second)
	is.Equal(history[2].User, "")
	commit, err := chky.FindCommit(ctx, &chunky.FindCommit{Repo: host, Revision: "old"})
	is.NoErr(err)
	is.Equal(commit.ID(), second)
	is.True(history[2].At.Sub(commit.CreatedAt()).Abs() <= time.Second)

	// Appending keeps the old lines as they were
	is.NoErr(chky.TagRevision(ctx, &chunky.TagRevision{Repo: host, Tag: "old", Revision: first, User: "Alice"}))
	data, err := os.ReadFile(filepath.Join(hostDir, "tags", "old"))
	is.NoErr(err)
	is.True(strings.HasPrefix(string(data), old))
	is.Equal(len(findTag(t, host, "old").History), 4)
}

func TestUploadAppendsTags(t *testing.T) {
	is := is.New(t)
	host := local.New(virt.OS(t.TempDir()))
	first, err := deploy(t, host, &chunky.Upload{From: tree("a", "b"), Tags: []string{"v2"}, User: "Alice"})
	is.NoErr(err)
	second, err := deploy(t, host, &chunky.Upload{From: tree("a", "b2"), Tags: []string{"v2"}, User: "Bob"})
	is.NoErr(err)
	history := findTag(t, host, "v2").History
	is.Equal(len(history), 2)
	is.Equal(history[0].Commit, first)
	is.Equal(history[0].User, "Alice")
	is.Equal(history[1].Commit, second)
	is.Equal(history[1].User, "Bob")
	is.True(!history[1].At.IsZero())

	// latest only names the newest upload
	history = findTag(t, host, "latest").History
	is.Equal(len(history), 1)
	is.Equal(history[0].Commit, second)
	is.Equal(history[0].User, "Bob")
	is.True(!history[0].At.IsZero())
}

func TestDeleteTag(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	chky := chunky.New(logs.Discard())
	hostDir := t.TempDir()
	host := local.New(virt.OS(hostDir))

	// A preview is tagged, then main moves on
	_, err := deploy(t, host, &chunky.Upload{From: tree("a", "b"), Tags: []string{"hello--pr-42"}})
	is.NoErr(err)
	_, err = deploy(t, host, &chunky.Upload{From: tree("a", "b2")})
	is.NoErr(err)
	is.Equal(countPacks(t, hostDir), 2)

	// Deleting the preview's tag removes it
	is.NoErr(chky.DeleteTag(ctx, &chunky.DeleteTag{Repo: host, Tag: "hello--pr-42"}))
	_, err = chky.FindCommit(ctx, &chunky.FindCommit{Repo: host, Revision: "hello--pr-42"})
	is.True(errors.Is(err, fs.ErrNotExist))
	allTags, err := chky.ListTags(ctx, &chunky.ListTags{Repo: host})
	is.NoErr(err)
	is.Equal(len(allTags), 1)
	is.Equal(allTags[0].Name, "latest")

	// Uploads stop reading its commit, so its files are sent again
	_, err = deploy(t, host, &chunky.Upload{From: tree("a", "b")})
	is.NoErr(err)
	is.Equal(countPacks(t, hostDir), 3)

	// Deleting a missing tag is not an error
	is.NoErr(chky.DeleteTag(ctx, &chunky.DeleteTag{Repo: host, Tag: "hello--pr-42"}))

	// A tag can't name anything outside tags/
	for _, tag := range []string{"", ".", "..", "../commits"} {
		is.True(chky.DeleteTag(ctx, &chunky.DeleteTag{Repo: host, Tag: tag}) != nil)
	}
	_, err = os.Stat(filepath.Join(hostDir, "commits"))
	is.NoErr(err)
}

func TestUploadChanged(t *testing.T) {
	is := is.New(t)
	hostDir := t.TempDir()
	host := local.New(virt.OS(hostDir))

	revision, err := deploy(t, host, &chunky.Upload{From: tree("a", "b"), Tags: []string{"v1"}})
	is.NoErr(err)
	is.True(revision != "")
	is.Equal(readLatest(t, host), map[string]string{"a.txt": "a", "b.txt": "b"})
	is.Equal(countPacks(t, hostDir), 1)

	// Only the changed file is sent
	revision, err = deploy(t, host, &chunky.Upload{From: tree("a", "b2")})
	is.NoErr(err)
	is.True(revision != "")
	is.Equal(readLatest(t, host), map[string]string{"a.txt": "a", "b.txt": "b2"})
	is.Equal(countPacks(t, hostDir), 2)

	// Nothing changed, so nothing is sent
	_, err = deploy(t, host, &chunky.Upload{From: tree("a", "b2")})
	is.NoErr(err)
	is.Equal(countPacks(t, hostDir), 2)

	// v1 still names the first version, so rolling back sends nothing
	_, err = deploy(t, host, &chunky.Upload{From: tree("a", "b")})
	is.NoErr(err)
	is.Equal(readLatest(t, host), map[string]string{"a.txt": "a", "b.txt": "b"})
	is.Equal(countPacks(t, hostDir), 2)

	// No tag names b3 once b4 is latest, so rolling back to it sends b3 again
	_, err = deploy(t, host, &chunky.Upload{From: tree("a", "b3")})
	is.NoErr(err)
	_, err = deploy(t, host, &chunky.Upload{From: tree("a", "b4")})
	is.NoErr(err)
	is.Equal(countPacks(t, hostDir), 4)
	_, err = deploy(t, host, &chunky.Upload{From: tree("a", "b3")})
	is.NoErr(err)
	is.Equal(readLatest(t, host), map[string]string{"a.txt": "a", "b.txt": "b3"})
	is.Equal(countPacks(t, hostDir), 5)
}
