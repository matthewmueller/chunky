package stream_test

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"testing"

	"github.com/matryer/is"
	"github.com/matthewmueller/chunky/repos"
	"github.com/matthewmueller/chunky/repos/stream"
)

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }

// readAll reads a stream into a map of path to data
func readAll(r io.Reader) (map[string]string, error) {
	files := map[string]string{}
	if err := stream.Read(r, func(file *repos.File) error {
		files[file.Path] = string(file.Data)
		return nil
	}); err != nil {
		return nil, err
	}
	return files, nil
}

// write a hand-written tar with one entry
func write(t testing.TB, header *tar.Header) []byte {
	t.Helper()
	is := is.New(t)
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)
	is.NoErr(tw.WriteHeader(header))
	is.NoErr(tw.Close())
	return buf.Bytes()
}

func TestRoundTrip(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	buf := new(bytes.Buffer)
	w := stream.New(nopCloser{buf})
	is.NoErr(w.Upload(ctx, &repos.File{Path: "packs/a", Data: []byte("a"), Mode: 0644}))
	is.NoErr(w.Upload(ctx, &repos.File{Path: "tags/latest", Data: []byte("b"), Mode: 0644}))
	is.NoErr(w.Close())
	files, err := readAll(buf)
	is.NoErr(err)
	is.Equal(files, map[string]string{"packs/a": "a", "tags/latest": "b"})
}

func TestInvalid(t *testing.T) {
	is := is.New(t)
	files, err := readAll(bytes.NewReader(write(t, &tar.Header{Typeflag: tar.TypeReg, Name: "../x"})))
	is.True(err != nil)
	is.Equal(files, nil)
	files, err = readAll(bytes.NewReader(write(t, &tar.Header{Typeflag: tar.TypeReg, Name: "/x"})))
	is.True(err != nil)
	is.Equal(files, nil)
	files, err = readAll(bytes.NewReader(write(t, &tar.Header{Typeflag: tar.TypeReg, Name: "packs/../../x"})))
	is.True(err != nil)
	is.Equal(files, nil)
	files, err = readAll(bytes.NewReader(write(t, &tar.Header{Typeflag: tar.TypeSymlink, Name: "packs/x", Linkname: "/etc/passwd"})))
	is.True(err != nil)
	is.Equal(files, nil)
}

func TestModeMasked(t *testing.T) {
	is := is.New(t)
	data := write(t, &tar.Header{Typeflag: tar.TypeReg, Name: "packs/x", Mode: 0o4755})
	is.NoErr(stream.Read(bytes.NewReader(data), func(file *repos.File) error {
		is.Equal(file.Mode, fs.FileMode(0755))
		return nil
	}))
}

// archive/tar reads a stream cut between files as a clean end, and one cut
// mid-file as an unexpected EOF
func TestCut(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	buf := new(bytes.Buffer)
	w := stream.New(nopCloser{buf})
	is.NoErr(w.Upload(ctx, &repos.File{Path: "packs/a", Data: []byte("a"), Mode: 0644}))
	boundary := buf.Len() + 511 // the next header writes the padding
	is.NoErr(w.Upload(ctx, &repos.File{Path: "packs/b", Data: []byte("b"), Mode: 0644}))
	full := buf.Bytes()

	files, err := readAll(bytes.NewReader(full[:boundary]))
	is.NoErr(err)
	is.Equal(files, map[string]string{"packs/a": "a"})

	files, err = readAll(bytes.NewReader(full[:boundary+100]))
	is.True(errors.Is(err, io.ErrUnexpectedEOF))
	is.Equal(files, nil)
}
