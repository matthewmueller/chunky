package packs_test

import (
	"io/fs"
	"testing"
	"time"

	"github.com/matryer/is"
	"github.com/matthewmueller/chunky/internal/packs"
)

func newFile(path string, data []byte, mode fs.FileMode, modTime time.Time) *packs.File {
	return &packs.File{
		Path:    path,
		Mode:    mode,
		ModTime: modTime,
		Data:    data,
	}
}

func TestPackUnpack(t *testing.T) {
	is := is.New(t)

	pack := packs.New()
	now := time.Now()
	a := newFile("a.txt", []byte("a file"), 0644, now)
	b := newFile("b.txt", []byte("b file"), 0644, now)
	c := newFile("c/c.txt", []byte("c files"), 0644, now)
	a2 := newFile("a2.txt", []byte("a file"), 0644, now)
	is.NoErr(pack.Add(a))
	is.NoErr(pack.Add(b))
	is.NoErr(pack.Add(c))
	is.NoErr(pack.Add(a2))

	out, err := pack.Pack()
	is.NoErr(err)

	pack, err = packs.Unpack(out)
	is.NoErr(err)
	is.True(pack != nil)

	file, err := pack.Read(a.Path)
	is.NoErr(err)
	is.True(file != nil)
	is.Equal(file.Path, a.Path)
	is.Equal(string(file.Data), string(a.Data))
	is.Equal(file.ModTime.Unix(), a.ModTime.Unix())
	is.Equal(file.Mode, a.Mode)

	file, err = pack.Read(a2.Path)
	is.NoErr(err)
	is.True(file != nil)
	is.Equal(file.Path, a2.Path)
	is.Equal(string(file.Data), string(a2.Data))
	is.Equal(file.ModTime.Unix(), a2.ModTime.Unix())
	is.Equal(file.Mode, a2.Mode)
}
