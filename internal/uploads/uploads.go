package uploads

import (
	"fmt"
	"io"
	"io/fs"
	"path"
	"time"

	"github.com/matthewmueller/chunky/internal/chunker"
	"github.com/matthewmueller/chunky/internal/packs"
	"github.com/matthewmueller/chunky/internal/sha256"
	"github.com/matthewmueller/chunky/repos"
	"github.com/matthewmueller/virt"
	"github.com/segmentio/ksuid"
)

func New(uploadCh chan<- *repos.File, maxPackSize, minChunkSize, maxChunkSize int64) *Upload {
	return &Upload{
		uploadCh:     uploadCh,
		maxPackSize:  maxPackSize,
		minChunkSize: minChunkSize,
		maxChunkSize: maxChunkSize,
		current:      newPackFile(),
	}
}

type Upload struct {
	uploadCh     chan<- *repos.File
	maxPackSize  int64
	minChunkSize int64
	maxChunkSize int64

	// Current pack
	current *packFile
}

type File struct {
	io.Reader
	Path    string
	Hash    string
	Mode    fs.FileMode
	Size    int64
	ModTime time.Time
}

func newPackFile() *packFile {
	return &packFile{
		packs.New(),
		ksuid.New().String(),
	}
}

type packFile struct {
	*packs.Pack
	ID string
}

func (p *packFile) File() (*virt.File, error) {
	packData, err := p.Pack.Pack()
	if err != nil {
		return nil, err
	}
	return &virt.File{
		Path:    path.Join("packs", p.ID),
		Data:    packData,
		Mode:    0644,
		ModTime: time.Now(),
	}, nil
}

func (u *Upload) Add(file *File) (packId string, err error) {
	fileChunk := &packs.Chunk{
		Path:    file.Path,
		Mode:    file.Mode,
		Size:    file.Size,
		Hash:    file.Hash,
		ModTime: file.ModTime.Unix(),
	}

	// If the file data is less than one chunk, just add it directly to the pack
	if fileChunk.Length()+file.Size < u.maxChunkSize {
		data, err := io.ReadAll(file)
		if err != nil {
			return "", fmt.Errorf("reading file data: %w", err)
		}
		fileChunk.Data = data
		if err := u.maybeFlush(fileChunk.Length()); err != nil {
			return "", fmt.Errorf("flushing pack: %w", err)
		}
		u.current.Add(fileChunk)
		return u.current.ID, nil
	}

	// Chunk the file data
	chunker := chunker.New(file, uint(u.minChunkSize), uint(u.maxChunkSize))
	for {
		chunk, err := chunker.Chunk()
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", fmt.Errorf("chunking file: %w", err)
		}

		// Create a blob chunk
		blobChunk := &packs.Chunk{
			Hash: sha256.Hash(chunk.Data),
			Data: chunk.Data,
		}

		// If adding the blob chunk exceeds the max pack size, upload the current
		// pack and start a new pack
		if err := u.maybeFlush(blobChunk.Length()); err != nil {
			return "", fmt.Errorf("flushing pack: %w", err)
		}

		// Link the blob chunk to the file chunk
		fileChunk.Link(u.current.ID, blobChunk)

		// Add the blob chunk to the current pack
		u.current.Add(blobChunk)
	}

	// If adding the file chunk exceeds the max pack size, upload the current pack
	// and start a new one
	if err := u.maybeFlush(fileChunk.Length()); err != nil {
		return "", fmt.Errorf("flushing pack: %w", err)
	}
	u.current.Add(fileChunk)

	return u.current.ID, nil
}

// Flush the current pack if adding the chunk would exceed the max pack size
func (u *Upload) maybeFlush(chunkLength int64) error {
	if u.current.Length()+chunkLength < u.maxPackSize {
		return nil
	}
	return u.Flush()
}

func (u *Upload) Flush() error {
	if u.current.Length() == 0 {
		return nil
	}
	packFile, err := u.current.File()
	if err != nil {
		return err
	}
	u.uploadCh <- packFile
	u.current = newPackFile()
	return nil
}
