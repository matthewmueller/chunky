package uploads_test

import (
	"bytes"
	"context"
	"io/fs"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/matryer/is"
	"github.com/matthewmueller/chunky/internal/packs"
	"github.com/matthewmueller/chunky/internal/sha256"
	"github.com/matthewmueller/chunky/internal/uploads"
	"github.com/matthewmueller/chunky/repos"
)

const kib = 1024

func makeData(amount int) []byte {
	data := make([]byte, amount)
	for i := 0; i < amount; i++ {
		data[i] = byte(i % 256)
	}
	return data
}

func pullPackFile(uploadCh <-chan *repos.File) (*repos.File, bool) {
	select {
	case file := <-uploadCh:
		return file, true
	default:
		return nil, false
	}
}

func TestEmpty(t *testing.T) {
	ctx := context.Background()
	is := is.New(t)
	uploadCh := make(chan *repos.File, 1)
	upload := uploads.New(uploadCh)
	upload.MaxPackSize = 8 * kib
	upload.MinChunkSize = 1 * kib
	upload.MaxChunkSize = 2 * kib
	is.NoErr(upload.Flush(ctx))
	file, ok := pullPackFile(uploadCh)
	is.True(!ok)
	is.Equal(file, nil)
}

func TestOneFileNoChunks(t *testing.T) {
	ctx := context.Background()
	is := is.New(t)
	uploadCh := make(chan *repos.File, 1)

	upload := uploads.New(uploadCh)
	upload.MaxPackSize = 8 * kib
	upload.MinChunkSize = 1 * kib
	upload.MaxChunkSize = 2 * kib

	data := makeData(1 * kib)
	modTime := time.Now()

	packId, err := upload.Add(ctx, &uploads.File{
		Reader:  bytes.NewReader(data),
		Path:    "test.txt",
		Hash:    sha256.Hash(data),
		Mode:    0644,
		Size:    int64(len(data)),
		ModTime: modTime,
	})
	is.NoErr(err)
	is.True(packId != "")

	// There shouldn't have been a file uploaded yet because the pack is not full
	file, ok := pullPackFile(uploadCh)
	is.True(!ok)
	is.Equal(file, nil)

	// Close the upload
	err = upload.Flush(ctx)
	is.NoErr(err)

	file, ok = pullPackFile(uploadCh)
	is.True(ok)
	is.Equal(file.Path, path.Join("packs", packId))
	is.Equal(file.Mode, fs.FileMode(0644))
	is.True(!file.ModTime.Equal(modTime))
	is.True(len(file.Data) < upload.MaxPackSize)
	pack, err := packs.Unpack(file.Data)
	is.NoErr(err)
	is.True(pack != nil)
	is.Equal(len(pack.Chunks()), 1)

	chunk, ok := pack.Chunk("test.txt")
	is.True(ok)
	is.True(chunk != nil)
	is.Equal(chunk.Path, "test.txt")
	is.Equal(chunk.Mode, fs.FileMode(0644))
	is.Equal(chunk.Size, int64(len(data)))
	is.Equal(chunk.ModTime, modTime.Unix())
	is.Equal(chunk.Hash, sha256.Hash(data))
	is.Equal(chunk.Data, data)
	is.Equal(len(chunk.Refs), 0)
}

func TestOneFileOneChunk(t *testing.T) {
	ctx := context.Background()
	is := is.New(t)
	uploadCh := make(chan *repos.File, 1)

	upload := uploads.New(uploadCh)
	upload.MaxPackSize = 8 * kib
	upload.MinChunkSize = 512
	upload.MaxChunkSize = 1 * kib

	data := makeData(1 * kib)
	modTime := time.Now()

	packId, err := upload.Add(ctx, &uploads.File{
		Reader:  bytes.NewReader(data),
		Path:    "test.txt",
		Hash:    sha256.Hash(data),
		Mode:    0644,
		Size:    int64(len(data)),
		ModTime: modTime,
	})
	is.NoErr(err)
	is.True(packId != "")

	// There shouldn't have been a file uploaded yet because the pack is not full
	file, ok := pullPackFile(uploadCh)
	is.True(!ok)
	is.Equal(file, nil)

	// Close the upload
	err = upload.Flush(ctx)
	is.NoErr(err)

	file, ok = pullPackFile(uploadCh)
	is.True(ok)
	is.Equal(file.Path, path.Join("packs", packId))
	is.Equal(file.Mode, fs.FileMode(0644))
	is.True(!file.ModTime.Equal(modTime))
	is.True(len(file.Data) < upload.MaxPackSize)
	pack, err := packs.Unpack(file.Data)
	is.NoErr(err)
	is.True(pack != nil)
	is.Equal(len(pack.Chunks()), 2)

	chunk, ok := pack.Chunk("test.txt")
	is.True(ok)
	is.True(chunk != nil)
	is.Equal(chunk.Path, "test.txt")
	is.Equal(chunk.Mode, fs.FileMode(0644))
	is.Equal(chunk.Size, int64(len(data)))
	is.Equal(chunk.ModTime, modTime.Unix())
	is.Equal(chunk.Hash, sha256.Hash(data))
	is.Equal(chunk.Data, nil)
	is.Equal(len(chunk.Refs), 1)
	is.True(chunk.Refs[0].Hash != "")
	is.Equal(chunk.Refs[0].Pack, packId)
}

func TestOneFileTwoChunks(t *testing.T) {
	ctx := context.Background()
	is := is.New(t)
	uploadCh := make(chan *repos.File, 1)

	upload := uploads.New(uploadCh)
	upload.MaxPackSize = 8 * kib
	upload.MinChunkSize = 512
	upload.MaxChunkSize = 1 * kib

	data := makeData(2 * kib)
	modTime := time.Now()

	packId, err := upload.Add(ctx, &uploads.File{
		Reader:  bytes.NewReader(data),
		Path:    "test.txt",
		Hash:    sha256.Hash(data),
		Mode:    0644,
		Size:    int64(len(data)),
		ModTime: modTime,
	})
	is.NoErr(err)
	is.True(packId != "")

	// There shouldn't have been a file uploaded yet because the pack is not full
	file, ok := pullPackFile(uploadCh)
	is.True(!ok)
	is.Equal(file, nil)

	// Flush the upload
	err = upload.Flush(ctx)
	is.NoErr(err)

	file, ok = pullPackFile(uploadCh)
	is.True(ok)
	is.Equal(file.Path, path.Join("packs", packId))
	is.Equal(file.Mode, fs.FileMode(0644))
	is.True(!file.ModTime.Equal(modTime))
	is.True(len(file.Data) < upload.MaxPackSize)

	pack, err := packs.Unpack(file.Data)
	is.NoErr(err)
	is.True(pack != nil)
	is.Equal(len(pack.Chunks()), 3)
	// First file chunk
	fchunk, ok := pack.Chunk("test.txt")
	is.True(ok)
	is.True(fchunk != nil)
	is.Equal(fchunk.Path, "test.txt")
	is.Equal(fchunk.Mode, fs.FileMode(0644))
	is.Equal(fchunk.Size, int64(len(data)))
	is.Equal(fchunk.ModTime, modTime.Unix())
	is.Equal(fchunk.Hash, sha256.Hash(data))
	is.Equal(fchunk.Data, nil)
	is.Equal(len(fchunk.Refs), 2)
	is.True(fchunk.Refs[0].Hash != "")
	is.Equal(fchunk.Refs[0].Pack, packId)
	is.True(fchunk.Refs[1].Hash != "")
	is.Equal(fchunk.Refs[1].Pack, packId)
	// Second blob chunk
	bchunk, ok := pack.Chunk(fchunk.Refs[0].Hash)
	is.True(ok)
	is.True(bchunk != nil)
	// Third blob chunk
	bchunk, ok = pack.Chunk(fchunk.Refs[1].Hash)
	is.True(ok)
	is.True(bchunk != nil)
}

func TestThreeFilesTwoPacks(t *testing.T) {
	ctx := context.Background()
	is := is.New(t)
	uploadCh := make(chan *repos.File, 2)

	upload := uploads.New(uploadCh)
	upload.MaxPackSize = 3 * kib
	upload.MinChunkSize = 512
	upload.MaxChunkSize = 1 * kib

	oneData := makeData(1 * kib)
	oneModTime := time.Now()
	onePackId, err := upload.Add(ctx, &uploads.File{
		Reader:  bytes.NewReader(oneData),
		Path:    "one.txt",
		Hash:    sha256.Hash(oneData),
		Mode:    0644,
		Size:    int64(len(oneData)),
		ModTime: oneModTime,
	})
	is.NoErr(err)
	is.True(onePackId != "")

	// There shouldn't have been a file uploaded yet because the pack is not full
	packFile, ok := pullPackFile(uploadCh)
	is.True(!ok)
	is.Equal(packFile, nil)

	twoData := makeData(1 * kib)
	twoModTime := time.Now()
	twoPackId, err := upload.Add(ctx, &uploads.File{
		Reader:  bytes.NewReader(twoData),
		Path:    "two.txt",
		Hash:    sha256.Hash(twoData),
		Mode:    0644,
		Size:    int64(len(twoData)),
		ModTime: twoModTime,
	})
	is.NoErr(err)
	is.True(twoPackId != "")
	is.Equal(onePackId, twoPackId)

	// Upload a third file
	threeData := makeData(2 * kib)
	threeModTime := time.Now()
	threePackId, err := upload.Add(ctx, &uploads.File{
		Reader:  bytes.NewReader(threeData),
		Path:    "three.txt",
		Hash:    sha256.Hash(threeData),
		Mode:    0644,
		Size:    int64(len(threeData)),
		ModTime: threeModTime,
	})
	is.NoErr(err)
	is.True(threePackId != "")
	is.True(twoPackId != threePackId)

	// There should have been a pack uploaded at this point
	firstPackFile, ok := pullPackFile(uploadCh)
	is.True(ok)
	is.Equal(firstPackFile.Path, path.Join("packs", onePackId))
	is.Equal(firstPackFile.Mode, fs.FileMode(0644))
	is.True(!firstPackFile.ModTime.Equal(oneModTime))
	is.True(!firstPackFile.ModTime.Equal(twoModTime))
	is.True(len(firstPackFile.Data) < upload.MaxPackSize)
	firstPack, err := packs.Unpack(firstPackFile.Data)
	is.NoErr(err)
	is.True(firstPack != nil)

	is.Equal(len(firstPack.Chunks()), 4)
	// First chunk
	chunk, ok := firstPack.Chunk("one.txt")
	is.True(ok)
	is.True(chunk != nil)
	is.Equal(chunk.Path, "one.txt")
	is.Equal(chunk.Mode, fs.FileMode(0644))
	is.Equal(chunk.Size, int64(len(oneData)))
	is.Equal(chunk.ModTime, oneModTime.Unix())
	is.Equal(chunk.Hash, sha256.Hash(oneData))
	is.Equal(chunk.Data, nil)
	is.Equal(len(chunk.Refs), 1)
	is.Equal(chunk.Refs[0].Pack, onePackId)
	is.True(chunk.Refs[0].Hash != "")
	// Second chunk
	chunk, ok = firstPack.Chunk(chunk.Refs[0].Hash)
	is.True(ok)
	is.True(chunk != nil)
	is.Equal(chunk.Data, oneData)
	// Third chunk
	chunk, ok = firstPack.Chunk("two.txt")
	is.True(ok)
	is.True(chunk != nil)
	is.Equal(chunk.Path, "two.txt")
	is.Equal(chunk.Mode, fs.FileMode(0644))
	is.Equal(chunk.Size, int64(len(twoData)))
	is.Equal(chunk.ModTime, twoModTime.Unix())
	is.Equal(chunk.Hash, sha256.Hash(twoData))
	is.Equal(chunk.Data, nil)
	is.Equal(len(chunk.Refs), 1)
	// Fourth chunk
	chunk, ok = firstPack.Chunk(chunk.Refs[0].Hash)
	is.True(ok)
	is.True(chunk != nil)
	is.Equal(chunk.Data, twoData)

	// There shouldn't have been an addition file uploaded yet because the second
	// pack is not full
	packFile, ok = pullPackFile(uploadCh)
	is.True(!ok)
	is.Equal(packFile, nil)

	// Flush the upload
	err = upload.Flush(ctx)
	is.NoErr(err)

	// Pull the second pack
	secondPackFile, ok := pullPackFile(uploadCh)
	is.True(ok)
	is.Equal(secondPackFile.Path, path.Join("packs", threePackId))
	is.Equal(secondPackFile.Mode, fs.FileMode(0644))
	is.True(!secondPackFile.ModTime.Equal(threeModTime))
	is.True(len(secondPackFile.Data) < upload.MaxPackSize)
	secondPack, err := packs.Unpack(secondPackFile.Data)
	is.NoErr(err)
	is.True(secondPack != nil)

	is.Equal(len(secondPack.Chunks()), 3)
	// First file chunk
	fchunk, ok := secondPack.Chunk("three.txt")
	is.True(ok)
	is.True(fchunk != nil)
	is.Equal(fchunk.Path, "three.txt")
	is.Equal(fchunk.Mode, fs.FileMode(0644))
	is.Equal(fchunk.Size, int64(len(threeData)))
	is.Equal(fchunk.ModTime, threeModTime.Unix())
	is.Equal(fchunk.Hash, sha256.Hash(threeData))
	is.Equal(fchunk.Data, nil)
	// Second blob chunk
	bchunk, ok := secondPack.Chunk(fchunk.Refs[0].Hash)
	is.True(ok)
	is.True(bchunk != nil)
	// Third blob chunk
	bchunk, ok = secondPack.Chunk(fchunk.Refs[1].Hash)
	is.True(ok)
	is.True(bchunk != nil)
}

// TODO: the packing algorithm is not optimal. We're only packing one chunk at
// a time, when you should be able to fit about two chunks per pack. This is due
// to the overhead of the pack file itself, where the max chunk size is not a
// hard limit, but a soft limit.
func TestBigFileThreePacks(t *testing.T) {
	ctx := context.Background()
	is := is.New(t)
	uploadCh := make(chan *repos.File, 3)

	upload := uploads.New(uploadCh)
	upload.MaxPackSize = 4 * kib
	upload.MinChunkSize = 512
	upload.MaxChunkSize = 2 * kib

	oneData := makeData(8 * kib)
	oneModTime := time.Now()

	packId, err := upload.Add(ctx, &uploads.File{
		Reader:  bytes.NewReader(oneData),
		Path:    "bigfile.txt",
		Hash:    sha256.Hash(oneData),
		Mode:    0644,
		Size:    int64(len(oneData)),
		ModTime: oneModTime,
	})
	is.NoErr(err)
	is.True(packId != "")

	// There should have been a pack uploaded at this point
	firstPackFile, ok := pullPackFile(uploadCh)
	is.True(ok)
	is.Equal(firstPackFile.Mode, fs.FileMode(0644))
	is.True(len(firstPackFile.Data) < upload.MaxPackSize)
	firstPack, err := packs.Unpack(firstPackFile.Data)
	is.NoErr(err)
	is.True(firstPack != nil)
	is.Equal(len(firstPack.Chunks()), 1)

	// Second pack
	secondPackFile, ok := pullPackFile(uploadCh)
	is.True(ok)
	is.Equal(secondPackFile.Mode, fs.FileMode(0644))
	is.True(len(secondPackFile.Data) < upload.MaxPackSize)
	secondPack, err := packs.Unpack(secondPackFile.Data)
	is.NoErr(err)
	is.True(secondPack != nil)
	is.Equal(len(secondPack.Chunks()), 1)

	// Third pack
	thirdPackFile, ok := pullPackFile(uploadCh)
	is.True(ok)
	is.Equal(thirdPackFile.Mode, fs.FileMode(0644))
	is.True(len(thirdPackFile.Data) < upload.MaxPackSize)
	thirdPack, err := packs.Unpack(thirdPackFile.Data)
	is.NoErr(err)
	is.True(thirdPack != nil)
	is.Equal(len(thirdPack.Chunks()), 1)

	// Ensure no more packs are uploaded
	packFile, ok := pullPackFile(uploadCh)
	is.True(!ok)
	is.Equal(packFile, nil)

	// Flush the upload
	err = upload.Flush(ctx)
	is.NoErr(err)

	// Fourth pack
	fourthPackFile, ok := pullPackFile(uploadCh)
	is.True(ok)
	is.Equal(fourthPackFile.Mode, fs.FileMode(0644))
	is.True(len(fourthPackFile.Data) < upload.MaxPackSize)
	fourthPack, err := packs.Unpack(fourthPackFile.Data)
	is.NoErr(err)
	is.True(fourthPack != nil)
	is.Equal(len(fourthPack.Chunks()), 2)

	is.True(firstPackFile.Path != secondPackFile.Path)
	is.True(secondPackFile.Path != thirdPackFile.Path)
	is.True(thirdPackFile.Path != fourthPackFile.Path)

	// Ensure the returned pack id points to pack with the file chunk
	is.Equal(packId, strings.TrimPrefix(fourthPackFile.Path, "packs/"))
}
