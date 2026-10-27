package stream_test

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

	"github.com/matryer/is"
	"github.com/matthewmueller/chunky"
	"github.com/matthewmueller/chunky/repos"
	"github.com/matthewmueller/chunky/repos/local"
	"github.com/matthewmueller/chunky/repos/stream"
	"github.com/matthewmueller/logs"
	"github.com/matthewmueller/virt"
)

func tree(a, b string) virt.Tree {
	return virt.Tree{
		"a.txt": &virt.File{Data: []byte(a), Mode: 0644},
		"b.txt": &virt.File{Data: []byte(b), Mode: 0644},
	}
}

// deploy uploads through a pipe into Receive, like a client writing to a
// host's stdin. It returns the uploaded and received commit ids.
func deploy(t testing.TB, host repos.Repo, in *chunky.Upload) (uploaded, received string, err error) {
	t.Helper()
	ctx := context.Background()
	pr, pw := io.Pipe()
	type result struct {
		id  string
		err error
	}
	done := make(chan result, 1)
	go func() {
		id, err := stream.Receive(ctx, pr, host)
		// Unblock the client, like a host exiting
		pr.CloseWithError(errors.Join(err, io.ErrClosedPipe))
		done <- result{id, err}
	}()
	w := stream.New(pw)
	in.To = w
	uploaded, upErr := chunky.New(logs.Discard()).Upload(ctx, in)
	upErr = errors.Join(upErr, w.Close())
	res := <-done
	if res.err != nil {
		if upErr == nil {
			t.Fatalf("receive failed but upload succeeded: %v", res.err)
		}
		return "", "", res.err
	}
	if upErr != nil {
		t.Fatalf("receive succeeded but upload failed: %v", upErr)
	}
	return uploaded, res.id, nil
}

func countPacks(t testing.TB, dir string) int {
	t.Helper()
	des, err := os.ReadDir(filepath.Join(dir, "packs"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	return len(des)
}

func readLatest(t testing.TB, host repos.Repo) virt.Map {
	t.Helper()
	dir := t.TempDir()
	if err := chunky.New(logs.Discard()).Download(context.Background(), &chunky.Download{
		From:     host,
		To:       virt.OS(dir),
		Revision: "latest",
	}); err != nil {
		t.Fatal(err)
	}
	a, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "b.txt"))
	if err != nil {
		t.Fatal(err)
	}
	return virt.Map{"a.txt": string(a), "b.txt": string(b)}
}

func TestRoundTrip(t *testing.T) {
	is := is.New(t)
	hostDir := t.TempDir()
	host := local.New(virt.OS(hostDir))
	cache := virt.OS(t.TempDir())

	uploaded, received, err := deploy(t, host, &chunky.Upload{From: tree("a", "b"), Cache: cache})
	is.NoErr(err)
	is.Equal(uploaded, received)
	is.Equal(readLatest(t, host), virt.Map{"a.txt": "a", "b.txt": "b"})
	is.Equal(countPacks(t, hostDir), 1)

	// A warm client defaults to its last upload as the base
	uploaded, received, err = deploy(t, host, &chunky.Upload{From: tree("a", "b2"), Cache: cache})
	is.NoErr(err)
	is.Equal(uploaded, received)
	is.Equal(readLatest(t, host), virt.Map{"a.txt": "a", "b.txt": "b2"})
	is.Equal(countPacks(t, hostDir), 2)
}

func TestFreshBase(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	hostDir := t.TempDir()
	host := local.New(virt.OS(hostDir))
	_, _, err := deploy(t, host, &chunky.Upload{From: tree("a", "b"), Cache: virt.OS(t.TempDir())})
	is.NoErr(err)

	// A fresh client reads the base as bytes, then sends only the changed file
	latest, err := chunky.New(logs.Discard()).FindCommit(ctx, &chunky.FindCommit{Repo: host, Revision: "latest"})
	is.NoErr(err)
	data, err := latest.Pack()
	is.NoErr(err)
	base, err := chunky.UnpackCommit(data)
	is.NoErr(err)
	_, _, err = deploy(t, host, &chunky.Upload{From: tree("a", "b2"), Cache: virt.OS(t.TempDir()), Base: base})
	is.NoErr(err)
	is.Equal(readLatest(t, host), virt.Map{"a.txt": "a", "b.txt": "b2"})
	is.Equal(countPacks(t, hostDir), 2)
}

func TestMissingBase(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	cacheDir := t.TempDir()
	_, _, err := deploy(t, local.New(virt.OS(t.TempDir())), &chunky.Upload{From: tree("a", "b"), Cache: virt.OS(cacheDir)})
	is.NoErr(err)

	// The cached base is from another host, so it's refused before any packs
	hostDir := t.TempDir()
	host := local.New(virt.OS(hostDir))
	_, _, err = deploy(t, host, &chunky.Upload{From: tree("a", "b2"), Cache: virt.OS(cacheDir)})
	is.True(errors.Is(err, stream.ErrStale))
	_, err = os.Stat(filepath.Join(hostDir, "packs"))
	is.True(errors.Is(err, fs.ErrNotExist))

	// Retry like dot: wipe the cache and read the base, which is missing too
	_, err = chunky.New(logs.Discard()).FindCommit(ctx, &chunky.FindCommit{Repo: host, Revision: "latest"})
	is.True(errors.Is(err, fs.ErrNotExist))
	_, _, err = deploy(t, host, &chunky.Upload{From: tree("a", "b2"), Cache: virt.OS(t.TempDir())})
	is.NoErr(err)
	is.Equal(readLatest(t, host), virt.Map{"a.txt": "a", "b.txt": "b2"})
}

func TestMissingPack(t *testing.T) {
	is := is.New(t)
	hostDir := t.TempDir()
	host := local.New(virt.OS(hostDir))
	cache := virt.OS(t.TempDir())
	first, _, err := deploy(t, host, &chunky.Upload{From: tree("a", "b"), Cache: cache})
	is.NoErr(err)
	is.NoErr(os.RemoveAll(filepath.Join(hostDir, "packs")))

	// a.txt still points at the deleted pack
	_, _, err = deploy(t, host, &chunky.Upload{From: tree("a", "b2"), Cache: cache})
	is.True(errors.Is(err, stream.ErrStale))
	des, err := os.ReadDir(filepath.Join(hostDir, "commits"))
	is.NoErr(err)
	is.Equal(len(des), 1)
	latest, err := os.ReadFile(filepath.Join(hostDir, "tags", "latest"))
	is.NoErr(err)
	is.Equal(string(latest), first)
}

// receive a hand-written tar of entries, each "name" or "name=data"
func receive(t testing.TB, host repos.Repo, typeflag byte, entries ...string) error {
	t.Helper()
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)
	for _, entry := range entries {
		name, data, _ := strings.Cut(entry, "=")
		if err := tw.WriteHeader(&tar.Header{Typeflag: typeflag, Name: name, Mode: 0644, Size: int64(len(data))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	_, err := stream.Receive(context.Background(), buf, host)
	return err
}

func TestInvalidEntries(t *testing.T) {
	is := is.New(t)
	root := t.TempDir()
	hostDir := filepath.Join(root, "host")
	host := local.New(virt.OS(hostDir))
	is.True(receive(t, host, tar.TypeReg, "../x") != nil)
	is.True(receive(t, host, tar.TypeReg, "/x") != nil)
	is.True(receive(t, host, tar.TypeReg, "packs/../../x") != nil)
	is.True(receive(t, host, tar.TypeReg, "packs/a/b") != nil)
	is.True(receive(t, host, tar.TypeReg, "other/x") != nil)
	is.True(receive(t, host, tar.TypeReg, "x") != nil)
	is.True(receive(t, host, tar.TypeReg, "base=../x") != nil)
	is.True(receive(t, host, tar.TypeSymlink, "packs/x") != nil)
	is.True(receive(t, host, tar.TypeDir, "packs/") != nil)
	is.True(receive(t, host, tar.TypeReg, "tags/latest=x") != nil)
	is.True(receive(t, host, tar.TypeReg, "packs/x", "base") != nil)
	_, err := os.Stat(filepath.Join(root, "x"))
	is.True(errors.Is(err, fs.ErrNotExist))
	_, err = os.Stat(filepath.Join(hostDir, "tags"))
	is.True(errors.Is(err, fs.ErrNotExist))
}

func TestModeMasked(t *testing.T) {
	is := is.New(t)
	hostDir := t.TempDir()
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)
	is.NoErr(tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: "packs/x", Mode: 0o4755}))
	is.NoErr(tw.Close())
	_, err := stream.Receive(context.Background(), buf, local.New(virt.OS(hostDir)))
	is.True(err != nil) // no commit
	info, err := os.Stat(filepath.Join(hostDir, "packs", "x"))
	is.NoErr(err)
	is.Equal(info.Mode(), fs.FileMode(0755))
}

// rewrite a stream, stopping before the first entry with prefix. It ends at
// an entry boundary without a trailer, which archive/tar reads as a clean
// io.EOF.
func cutBefore(t testing.TB, data []byte, prefix string) []byte {
	t.Helper()
	tr := tar.NewReader(bytes.NewReader(data))
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)
	for {
		header, err := tr.Next()
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(header.Name, prefix) {
			break
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(tw, tr); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Flush(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }

func TestCut(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	buf := new(bytes.Buffer)
	w := stream.New(nopCloser{buf})
	_, err := chunky.New(logs.Discard()).Upload(ctx, &chunky.Upload{
		From:  tree("a", strings.Repeat("b", 4096)),
		To:    w,
		Cache: virt.OS(t.TempDir()),
	})
	is.NoErr(err)
	is.NoErr(w.Close())
	full := buf.Bytes()

	// Cut mid-entry
	hostDir := t.TempDir()
	_, err = stream.Receive(ctx, bytes.NewReader(full[:len(full)/2]), local.New(virt.OS(hostDir)))
	is.True(errors.Is(err, io.ErrUnexpectedEOF))
	_, err = os.Stat(filepath.Join(hostDir, "commits"))
	is.True(errors.Is(err, fs.ErrNotExist))

	// Cut cleanly between the packs and the commit
	hostDir = t.TempDir()
	_, err = stream.Receive(ctx, bytes.NewReader(cutBefore(t, full, "commits/")), local.New(virt.OS(hostDir)))
	is.True(err != nil)
	is.Equal(countPacks(t, hostDir), 1)
	_, err = os.Stat(filepath.Join(hostDir, "commits"))
	is.True(errors.Is(err, fs.ErrNotExist))

	// Cut cleanly between the commit and the tags
	hostDir = t.TempDir()
	_, err = stream.Receive(ctx, bytes.NewReader(cutBefore(t, full, "tags/")), local.New(virt.OS(hostDir)))
	is.True(err != nil)
	_, err = os.Stat(filepath.Join(hostDir, "commits"))
	is.True(errors.Is(err, fs.ErrNotExist))

	// The whole stream succeeds
	hostDir = t.TempDir()
	_, err = stream.Receive(ctx, bytes.NewReader(full), local.New(virt.OS(hostDir)))
	is.NoErr(err)
}
