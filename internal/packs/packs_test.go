package packs_test

import (
	"bytes"
	"crypto/rand"
	"io/fs"
	"testing"
	"time"

	"github.com/matryer/is"
	"github.com/matthewmueller/chunky/internal/packs"
	"github.com/matthewmueller/chunky/repos"
)

func newFile(path string, data []byte, mode fs.FileMode, modTime time.Time) *repos.File {
	return &repos.File{
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

// Pulled from: https://github.com/mathiasbynens/small
// Built with: xxd -i small.ico
var favicon = []byte{
	0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x01, 0x00, 0x00, 0x01, 0x00,
	0x18, 0x00, 0x30, 0x00, 0x00, 0x00, 0x16, 0x00, 0x00, 0x00, 0x28, 0x00,
	0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x01, 0x00,
	0x18, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0xff, 0x00, 0x00, 0x00, 0x00, 0x00,
}

// Small valid gif: https://github.com/mathiasbynens/small/blob/master/gif.gif
var gif = []byte{
	0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01,
	0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x3b,
}

func TestPackUnpackBinary(t *testing.T) {
	is := is.New(t)
	pack := packs.New()
	now := time.Now()
	favicon := newFile("favicon.ico", favicon, 0644, now)
	gif := newFile("small.gif", gif, 0644, now)
	is.NoErr(pack.Add(favicon))
	is.NoErr(pack.Add(gif))

	out, err := pack.Pack()
	is.NoErr(err)

	pack, err = packs.Unpack(out)
	is.NoErr(err)
	is.True(pack != nil)

	file, err := pack.Read(favicon.Path)
	is.NoErr(err)
	is.True(file != nil)
	is.Equal(file.Path, favicon.Path)
	is.Equal(file.Data, favicon.Data)

	file, err = pack.Read(gif.Path)
	is.NoErr(err)
	is.True(file != nil)
	is.Equal(file.Path, gif.Path)
	is.Equal(file.Data, gif.Data)
}

func TestChunking(t *testing.T) {
	is := is.New(t)
	// Allocate 46 MB
	input := make([]byte, 46*1024*1024)
	_, err := rand.Read(input)
	is.NoErr(err)

	pack := packs.New()
	now := time.Now()
	big := newFile("big.bin", input, 0644, now)
	is.NoErr(pack.Add(big))

	out, err := pack.Pack()
	is.NoErr(err)

	pack, err = packs.Unpack(out)
	is.NoErr(err)
	is.True(pack != nil)

	file, err := pack.Read(big.Path)
	is.NoErr(err)
	is.Equal(file.Path, big.Path)
	is.Equal(len(file.Data), len(big.Data))
	if !bytes.Equal(file.Data, big.Data) {
		t.Fatalf("data is not equal")
	}
}
