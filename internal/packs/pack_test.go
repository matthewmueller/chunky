package packs_test

import (
	"crypto/sha256"
	"io/fs"
	"testing"
	"time"

	"github.com/matryer/is"
	"github.com/matthewmueller/chunky/internal/packs"
)

func TestPackUnpackFileWithData(t *testing.T) {
	is := is.New(t)

	pack := packs.New()
	now := time.Now()
	data := []byte("a small file")

	pack.Add(&packs.Chunk{
		Path:    "small.txt",
		Hash:    hash(data),
		Mode:    0644,
		Size:    int64(len(data)),
		ModTime: now.Unix(),
		Data:    data,
	})

	packData, err := pack.Pack()
	is.NoErr(err)

	pack, err = packs.Unpack(packData)
	is.NoErr(err)
	is.True(pack != nil)

	is.Equal(len(pack.Chunks()), 1)

	chunk, ok := pack.Chunk("small.txt")
	is.True(ok)
	is.True(chunk != nil)
	is.Equal(chunk.Path, "small.txt")
	is.Equal(chunk.Hash, hash(data))
	is.Equal(chunk.Mode, fs.FileMode(0644))
	is.Equal(chunk.Size, int64(len(data)))
	is.Equal(chunk.ModTime, now.Unix())
	is.Equal(chunk.Data, data)
}

// Pulled from: https://github.com/mathiasbynens/small
// Built with: xxd -i small.ico
var faviconData = []byte{
	0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x01, 0x00, 0x00, 0x01, 0x00,
	0x18, 0x00, 0x30, 0x00, 0x00, 0x00, 0x16, 0x00, 0x00, 0x00, 0x28, 0x00,
	0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x01, 0x00,
	0x18, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0xff, 0x00, 0x00, 0x00, 0x00, 0x00,
}

// Small valid gifData: https://github.com/mathiasbynens/small/blob/master/gifData.gifData
var gifData = []byte{
	0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01,
	0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x3b,
}

func TestPackUnpackMultiple(t *testing.T) {
	is := is.New(t)
	pack := packs.New()
	now := time.Now()

	pack.Add(&packs.Chunk{
		Path:    "favicon.ico",
		Data:    faviconData,
		Mode:    0644,
		Size:    int64(len(faviconData)),
		ModTime: now.Unix(),
		Hash:    hash(faviconData),
	})

	pack.Add(&packs.Chunk{
		Path:    "small.gif",
		Data:    gifData,
		Mode:    0644,
		Size:    int64(len(gifData)),
		ModTime: now.Unix(),
		Hash:    hash(gifData),
	})

	out, err := pack.Pack()
	is.NoErr(err)

	pack, err = packs.Unpack(out)
	is.NoErr(err)
	is.True(pack != nil)

	is.Equal(len(pack.Chunks()), 2)

	chunk, ok := pack.Chunk("favicon.ico")
	is.True(ok)
	is.True(chunk != nil)
	is.Equal(chunk.Path, "favicon.ico")
	is.Equal(chunk.Data, faviconData)
	is.Equal(chunk.Kind(), "file")

	chunk, ok = pack.Chunk("small.gif")
	is.True(ok)
	is.NoErr(err)
	is.True(chunk != nil)
	is.Equal(chunk.Path, "small.gif")
	is.Equal(chunk.Data, gifData)
	is.Equal(chunk.Kind(), "file")
}

func segmentData(data []byte, n int) [][]byte {
	chunkSize := (len(data) + n - 1) / n
	chunks := make([][]byte, 0, n)
	for i := 0; i < len(data); i += chunkSize {
		end := min(i+chunkSize, len(data))
		chunks = append(chunks, data[i:end])
	}
	return chunks
}

func hash(data []byte) string {
	hash := sha256.Sum256(data)
	return string(hash[:])
}

func TestPackUnpackWithRefs(t *testing.T) {
	is := is.New(t)
	pack := packs.New()
	now := time.Now()

	faviconFile := &packs.Chunk{
		Path:    "favicon.ico",
		Mode:    0644,
		Size:    int64(len(faviconData)),
		ModTime: now.Unix(),
		Hash:    hash(faviconData),
	}
	pack.Add(faviconFile)

	blobs := segmentData(faviconData, 3)
	for _, blob := range blobs {
		chunk := &packs.Chunk{
			Hash: hash(blob),
			Data: blob,
		}
		pack.Add(chunk)
		faviconFile.Link("some-pack-id", chunk)
	}

	pack.Add(&packs.Chunk{
		Path:    "small.gif",
		Data:    gifData,
		Mode:    0644,
		Size:    int64(len(gifData)),
		ModTime: now.Unix(),
		Hash:    hash(gifData),
	})

	out, err := pack.Pack()
	is.NoErr(err)

	pack, err = packs.Unpack(out)
	is.NoErr(err)
	is.True(pack != nil)

	is.Equal(len(pack.Chunks()), 5)

	chunk, ok := pack.Chunk("favicon.ico")
	is.True(ok)
	is.True(chunk != nil)
	is.Equal(chunk.Path, "favicon.ico")
	is.Equal(chunk.Data, nil)
	is.Equal(chunk.Kind(), "file")
	is.Equal(len(chunk.Refs), 3)
	is.Equal(chunk.Refs[0].Pack, "some-pack-id")
	is.Equal(chunk.Refs[0].Hash, hash(blobs[0]))
	is.Equal(chunk.Refs[1].Pack, "some-pack-id")
	is.Equal(chunk.Refs[1].Hash, hash(blobs[1]))
	is.Equal(chunk.Refs[2].Pack, "some-pack-id")
	is.Equal(chunk.Refs[2].Hash, hash(blobs[2]))
}
