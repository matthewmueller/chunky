package uploads_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"path"
	"testing"
	"time"

	"github.com/matryer/is"
	"github.com/matthewmueller/chunky/internal/packs"
	"github.com/matthewmueller/chunky/internal/sha256"
	"github.com/matthewmueller/chunky/internal/uploads"
	"github.com/matthewmueller/chunky/repos"
	"github.com/matthewmueller/chunky/repos/local"
	"github.com/matthewmueller/logs"
	"github.com/matthewmueller/virt"
)

const kib = 1024

func makeData(amount int) []byte {
	data := make([]byte, amount)
	for i := range amount {
		data[i] = byte(i % 256)
	}
	return data
}

func makeFile(path string, size int) *uploads.File {
	data := makeData(size)
	return &uploads.File{
		Reader:  bytes.NewReader(data),
		Path:    "test.txt",
		Hash:    sha256.Hash(data),
		Mode:    0644,
		Size:    int64(len(data)),
		ModTime: time.Now(),
	}
}

func TestEmpty(t *testing.T) {
	ctx := context.Background()
	is := is.New(t)
	dir := t.TempDir()
	fsys := virt.OS(dir)
	repo := local.New(fsys)
	upload := uploads.New(logs.Discard(), repo, 1)
	upload.MaxPackSize = 8 * kib
	upload.MinChunkSize = 1 * kib
	upload.MaxChunkSize = 2 * kib
	is.NoErr(upload.Flush(ctx))
	des, err := fs.ReadDir(fsys, "packs")
	is.True(errors.Is(err, fs.ErrNotExist))
	is.Equal(len(des), 0)
}

func TestOneFileNoChunks(t *testing.T) {
	ctx := context.Background()
	is := is.New(t)
	dir := t.TempDir()
	fsys := virt.OS(dir)
	repo := local.New(fsys)
	upload := uploads.New(logs.Discard(), repo, 1)
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
	des, err := fs.ReadDir(fsys, "packs")
	is.True(errors.Is(err, fs.ErrNotExist))
	is.Equal(len(des), 0)

	// Close the upload
	err = upload.Flush(ctx)
	is.NoErr(err)

	des, err = fs.ReadDir(fsys, "packs")
	is.NoErr(err)
	is.Equal(len(des), 1)

	stat, err := fsys.Stat(path.Join("packs", packId))
	is.NoErr(err)
	is.Equal(stat.Mode(), fs.FileMode(0644))
	is.True(!stat.ModTime().Equal(modTime))

	packData, err := fs.ReadFile(fsys, path.Join("packs", packId))
	is.NoErr(err)
	is.True(len(packData) < upload.MaxPackSize)
	pack, err := packs.Unpack(packData)
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

	dir := t.TempDir()
	fsys := virt.OS(dir)
	repo := local.New(fsys)
	upload := uploads.New(logs.Discard(), repo, 1)
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
	des, err := fs.ReadDir(fsys, "packs")
	is.True(errors.Is(err, fs.ErrNotExist))
	is.Equal(len(des), 0)

	// Close the upload
	err = upload.Flush(ctx)
	is.NoErr(err)

	stat, err := fsys.Stat(path.Join("packs", packId))
	is.NoErr(err)
	is.Equal(stat.Mode(), fs.FileMode(0644))
	is.True(!stat.ModTime().Equal(modTime))

	packData, err := fs.ReadFile(fsys, path.Join("packs", packId))
	is.NoErr(err)
	is.True(len(packData) < upload.MaxPackSize)
	pack, err := packs.Unpack(packData)
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

	dir := t.TempDir()
	fsys := virt.OS(dir)
	repo := local.New(fsys)
	upload := uploads.New(logs.Discard(), repo, 1)
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
	des, err := fs.ReadDir(fsys, "packs")
	is.True(errors.Is(err, fs.ErrNotExist))
	is.Equal(len(des), 0)

	// Flush the upload
	err = upload.Flush(ctx)
	is.NoErr(err)

	stat, err := fsys.Stat(path.Join("packs", packId))
	is.NoErr(err)
	is.Equal(stat.Mode(), fs.FileMode(0644))
	is.True(!stat.ModTime().Equal(modTime))

	packData, err := fs.ReadFile(fsys, path.Join("packs", packId))
	is.NoErr(err)
	is.True(len(packData) < upload.MaxPackSize)
	pack, err := packs.Unpack(packData)
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

	dir := t.TempDir()
	fsys := virt.OS(dir)
	repo := local.New(fsys)
	upload := uploads.New(logs.Discard(), repo, 1)
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
	des, err := fs.ReadDir(fsys, "packs")
	is.True(errors.Is(err, fs.ErrNotExist))
	is.Equal(len(des), 0)

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
	is.NoErr(upload.Wait())
	des, err = fs.ReadDir(fsys, "packs")
	is.NoErr(err)
	is.Equal(len(des), 1)

	firstPackStat, err := fsys.Stat(path.Join("packs", onePackId))
	is.NoErr(err)
	is.Equal(firstPackStat.Mode(), fs.FileMode(0644))
	is.True(!firstPackStat.ModTime().Equal(oneModTime))
	is.True(!firstPackStat.ModTime().Equal(twoModTime))

	firstPackData, err := fs.ReadFile(fsys, path.Join("packs", onePackId))
	is.NoErr(err)
	is.True(len(firstPackData) < upload.MaxPackSize)

	firstPack, err := packs.Unpack(firstPackData)
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
	is.NoErr(upload.Wait())
	des, err = fs.ReadDir(fsys, "packs")
	is.NoErr(err)
	is.Equal(len(des), 1)

	// Flush the upload
	err = upload.Flush(ctx)
	is.NoErr(err)

	// Pull the second pack
	secondPackStat, err := fsys.Stat(path.Join("packs", threePackId))
	is.NoErr(err)
	is.Equal(secondPackStat.Mode(), fs.FileMode(0644))
	is.True(!secondPackStat.ModTime().Equal(threeModTime))

	secondPackData, err := fs.ReadFile(fsys, path.Join("packs", threePackId))
	is.NoErr(err)
	is.True(len(secondPackData) < upload.MaxPackSize)

	secondPack, err := packs.Unpack(secondPackData)
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

	fsys := virt.OS(t.TempDir())
	repo := local.New(fsys)
	upload := uploads.New(logs.Discard(), repo, 1)
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

	// Wait for current uploads to finish
	is.NoErr(upload.Wait())

	des, err := fs.ReadDir(fsys, "packs")
	is.NoErr(err)
	is.Equal(len(des), 3)

	seen := map[string]bool{}
	for _, de := range des {
		seen[de.Name()] = true
		stat, err := fsys.Stat(path.Join("packs", de.Name()))
		is.NoErr(err)
		is.Equal(stat.Mode(), fs.FileMode(0644))

		packData, err := fs.ReadFile(fsys, path.Join("packs", de.Name()))
		is.NoErr(err)
		is.True(len(packData) < upload.MaxPackSize)

		pack, err := packs.Unpack(packData)
		is.NoErr(err)
		is.True(pack != nil)
		is.Equal(len(pack.Chunks()), 1)
	}

	// Flush the upload
	err = upload.Flush(ctx)
	is.NoErr(err)

	des, err = fs.ReadDir(fsys, "packs")
	is.NoErr(err)
	is.Equal(len(des), 4)

	// Get the fourth pack
	for _, de := range des {
		if seen[de.Name()] {
			continue
		}
		stat, err := fsys.Stat(path.Join("packs", de.Name()))
		is.NoErr(err)
		is.Equal(stat.Mode(), fs.FileMode(0644))

		packData, err := fs.ReadFile(fsys, path.Join("packs", de.Name()))
		is.NoErr(err)
		is.True(len(packData) < upload.MaxPackSize)

		pack, err := packs.Unpack(packData)
		is.NoErr(err)
		is.True(pack != nil)
		is.Equal(len(pack.Chunks()), 2)
	}
}

type slowRepo struct {
	*local.Repo
}

func (s *slowRepo) Upload(ctx context.Context, file *repos.File) error {
	time.Sleep(100 * time.Millisecond)
	return s.Repo.Upload(ctx, file)
}

func TestConcurrency(t *testing.T) {
	ctx := context.Background()
	is := is.New(t)

	fsys := virt.OS(t.TempDir())
	repo := &slowRepo{local.New(fsys)}
	upload := uploads.New(logs.Discard(), repo, 3)
	upload.MaxPackSize = 2 * kib
	upload.MinChunkSize = 512
	upload.MaxChunkSize = 1 * kib

	now := time.Now()
	pid, err := upload.Add(ctx, makeFile("one.txt", 1*kib))
	is.True(pid != "")
	is.NoErr(err)
	pid, err = upload.Add(ctx, makeFile("two.txt", 1*kib))
	is.True(pid != "")
	is.NoErr(err)
	pid, err = upload.Add(ctx, makeFile("three.txt", 1*kib))
	is.True(pid != "")
	is.NoErr(err)

	// Flush the upload
	is.NoErr(upload.Flush(ctx))

	// Ensure that the uploads took less than 300ms, which means they were done
	// concurrently
	is.True(time.Since(now) < 300*time.Millisecond)
}
