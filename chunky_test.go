package chunky_test

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
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
	"github.com/matthewmueller/chunky/repos/stream"
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

func TestFindCommitMissing(t *testing.T) {
	is := is.New(t)
	chky := chunky.New(logs.Default())
	_, err := chky.FindCommit(context.Background(), local.New(virt.OS(t.TempDir())), "latest")
	is.True(errors.Is(err, fs.ErrNotExist))
}

func tree(a, b string) virt.Tree {
	return virt.Tree{
		"a.txt": &virt.File{Data: []byte(a), Mode: 0644},
		"b.txt": &virt.File{Data: []byte(b), Mode: 0644},
	}
}

// deploy uploads through a pipe into Receive, like a client writing to a
// host's stdin, and returns the received revision
func deploy(t testing.TB, host repos.Repo, in *chunky.Upload) (string, error) {
	t.Helper()
	is := is.New(t)
	ctx := context.Background()
	chky := chunky.New(logs.Discard())
	pr, pw := io.Pipe()
	type result struct {
		revision string
		err      error
	}
	done := make(chan result, 1)
	go func() {
		revision, err := chky.Receive(ctx, pr, host)
		// Unblock the client, like a host exiting
		pr.CloseWithError(errors.Join(err, io.ErrClosedPipe))
		done <- result{revision, err}
	}()
	w := stream.New(pw)
	in.To = w
	uploaded, upErr := chky.Upload(ctx, in)
	upErr = errors.Join(upErr, w.Close())
	res := <-done
	if res.err != nil {
		is.True(upErr != nil) // the upload fails with the receive
		return "", res.err
	}
	is.NoErr(upErr)
	is.Equal(uploaded, res.revision)
	return res.revision, nil
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

func TestReceive(t *testing.T) {
	is := is.New(t)
	hostDir := t.TempDir()
	host := local.New(virt.OS(hostDir))
	cache := virt.OS(t.TempDir())

	_, err := deploy(t, host, &chunky.Upload{From: tree("a", "b"), Cache: cache})
	is.NoErr(err)
	is.Equal(readLatest(t, host), map[string]string{"a.txt": "a", "b.txt": "b"})
	is.Equal(countPacks(t, hostDir), 1)

	// A warm cache only sends the changed file
	_, err = deploy(t, host, &chunky.Upload{From: tree("a", "b2"), Cache: cache})
	is.NoErr(err)
	is.Equal(readLatest(t, host), map[string]string{"a.txt": "a", "b.txt": "b2"})
	is.Equal(countPacks(t, hostDir), 2)
}

func TestReceiveBase(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	hostDir := t.TempDir()
	host := local.New(virt.OS(hostDir))
	_, err := deploy(t, host, &chunky.Upload{From: tree("a", "b"), Cache: virt.OS(t.TempDir())})
	is.NoErr(err)

	// A fresh cache reads the base from the host, then only sends the changed
	// file
	base, err := chunky.New(logs.Discard()).FindCommit(ctx, host, "latest")
	is.NoErr(err)
	_, err = deploy(t, host, &chunky.Upload{From: tree("a", "b2"), Cache: virt.OS(t.TempDir()), Base: base})
	is.NoErr(err)
	is.Equal(readLatest(t, host), map[string]string{"a.txt": "a", "b.txt": "b2"})
	is.Equal(countPacks(t, hostDir), 2)
}

func TestReceiveStale(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	cacheDir := t.TempDir()
	_, err := deploy(t, local.New(virt.OS(t.TempDir())), &chunky.Upload{From: tree("a", "b"), Cache: virt.OS(cacheDir)})
	is.NoErr(err)

	// The cache is from another host, so a.txt points at a pack this host
	// doesn't have
	hostDir := t.TempDir()
	host := local.New(virt.OS(hostDir))
	_, err = deploy(t, host, &chunky.Upload{From: tree("a", "b2"), Cache: virt.OS(cacheDir)})
	is.True(errors.Is(err, chunky.ErrStale))
	_, err = os.Stat(filepath.Join(hostDir, "commits"))
	is.True(errors.Is(err, fs.ErrNotExist))

	// Retry with a fresh cache, after the base read finds nothing
	_, err = chunky.New(logs.Discard()).FindCommit(ctx, host, "latest")
	is.True(errors.Is(err, fs.ErrNotExist))
	_, err = deploy(t, host, &chunky.Upload{From: tree("a", "b2"), Cache: virt.OS(t.TempDir())})
	is.NoErr(err)
	is.Equal(readLatest(t, host), map[string]string{"a.txt": "a", "b.txt": "b2"})
}

func TestReceiveMissingPack(t *testing.T) {
	is := is.New(t)
	hostDir := t.TempDir()
	host := local.New(virt.OS(hostDir))
	cache := virt.OS(t.TempDir())
	first, err := deploy(t, host, &chunky.Upload{From: tree("a", "b"), Cache: cache})
	is.NoErr(err)
	is.NoErr(os.RemoveAll(filepath.Join(hostDir, "packs")))

	_, err = deploy(t, host, &chunky.Upload{From: tree("a", "b2"), Cache: cache})
	is.True(errors.Is(err, chunky.ErrStale))
	des, err := os.ReadDir(filepath.Join(hostDir, "commits"))
	is.NoErr(err)
	is.Equal(len(des), 1)
	latest, err := os.ReadFile(filepath.Join(hostDir, "tags", "latest"))
	is.NoErr(err)
	is.Equal(string(latest), first)
}

// receive a hand-written stream of files, each "path" or "path=data"
func receive(t testing.TB, host repos.Repo, files ...string) error {
	t.Helper()
	is := is.New(t)
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)
	for _, file := range files {
		name, data, _ := strings.Cut(file, "=")
		is.NoErr(tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: name, Mode: 0644, Size: int64(len(data))}))
		_, err := tw.Write([]byte(data))
		is.NoErr(err)
	}
	is.NoErr(tw.Close())
	_, err := chunky.New(logs.Discard()).Receive(context.Background(), buf, host)
	return err
}

func TestReceiveInvalid(t *testing.T) {
	is := is.New(t)
	hostDir := t.TempDir()
	host := local.New(virt.OS(hostDir))
	is.True(receive(t, host, "x") != nil)
	is.True(receive(t, host, "other/x") != nil)
	is.True(receive(t, host, "packs/a/b") != nil)
	is.True(receive(t, host, "tags/latest=x") != nil)
	_, err := os.Stat(filepath.Join(hostDir, "tags"))
	is.True(errors.Is(err, fs.ErrNotExist))
}

// cutBefore rewrites a stream up to the first file with prefix, ending
// between files without a trailer
func cutBefore(t testing.TB, data []byte, prefix string) []byte {
	t.Helper()
	is := is.New(t)
	tr := tar.NewReader(bytes.NewReader(data))
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)
	for {
		header, err := tr.Next()
		is.NoErr(err)
		if strings.HasPrefix(header.Name, prefix) {
			break
		}
		is.NoErr(tw.WriteHeader(header))
		_, err = io.Copy(tw, tr)
		is.NoErr(err)
	}
	is.NoErr(tw.Flush())
	return buf.Bytes()
}

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }

func TestReceiveCut(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	chky := chunky.New(logs.Discard())
	buf := new(bytes.Buffer)
	w := stream.New(nopCloser{buf})
	_, err := chky.Upload(ctx, &chunky.Upload{
		From:  tree("a", strings.Repeat("b", 4096)),
		To:    w,
		Cache: virt.OS(t.TempDir()),
	})
	is.NoErr(err)
	is.NoErr(w.Close())
	full := buf.Bytes()

	// Cut in half
	hostDir := t.TempDir()
	_, err = chky.Receive(ctx, bytes.NewReader(full[:len(full)/2]), local.New(virt.OS(hostDir)))
	is.True(err != nil)
	_, err = os.Stat(filepath.Join(hostDir, "commits"))
	is.True(errors.Is(err, fs.ErrNotExist))

	// Cut between the commit and the tags
	hostDir = t.TempDir()
	_, err = chky.Receive(ctx, bytes.NewReader(cutBefore(t, full, "tags/")), local.New(virt.OS(hostDir)))
	is.True(err != nil)
	is.Equal(countPacks(t, hostDir), 1)
	_, err = os.Stat(filepath.Join(hostDir, "commits"))
	is.True(errors.Is(err, fs.ErrNotExist))

	// The whole stream
	hostDir = t.TempDir()
	_, err = chky.Receive(ctx, bytes.NewReader(full), local.New(virt.OS(hostDir)))
	is.NoErr(err)
}
