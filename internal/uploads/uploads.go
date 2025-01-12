package uploads

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"path"
	"time"

	"github.com/matthewmueller/chunky/internal/chunker"
	"github.com/matthewmueller/chunky/internal/packs"
	"github.com/matthewmueller/chunky/internal/rate"
	"github.com/matthewmueller/chunky/internal/sha256"
	"github.com/matthewmueller/chunky/repos"
	"github.com/matthewmueller/logs"
	"github.com/matthewmueller/virt"
	"github.com/segmentio/ksuid"
)

const kiB = 1024
const miB = 1024 * kiB

func New(log *slog.Logger, uploadCh chan<- *repos.File) *Upload {
	return &Upload{
		log:      log,
		uploadCh: uploadCh,

		MaxPackSize:  32 * miB,
		MinChunkSize: 512 * kiB,
		MaxChunkSize: 8 * miB,

		// By default, don't limit the upload rate
		Limiter: rate.New(0),

		current: newPackFile(),
	}
}

type Upload struct {
	log          *slog.Logger
	uploadCh     chan<- *repos.File
	MaxPackSize  int
	MinChunkSize int
	MaxChunkSize int
	Limiter      rate.Limiter
	Concurrency  int

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

func (u *Upload) Add(ctx context.Context, file *File) (packId string, err error) {
	fileChunk := &packs.Chunk{
		Path:    file.Path,
		Mode:    file.Mode,
		Size:    file.Size,
		Hash:    file.Hash,
		ModTime: file.ModTime.Unix(),
	}

	// If the file data is less than one chunk, just add it directly to the pack
	if fileChunk.Length()+int(file.Size) < u.MaxChunkSize {
		data, err := io.ReadAll(file)
		if err != nil {
			return "", fmt.Errorf("reading file data: %w", err)
		}
		fileChunk.Data = data
		if err := u.maybeFlush(ctx, fileChunk.Length()); err != nil {
			return "", fmt.Errorf("flushing pack: %w", err)
		}
		u.current.Add(fileChunk)
		return u.current.ID, nil
	}

	// Chunk the file data
	chunker := chunker.New(file, uint(u.MinChunkSize), uint(u.MaxChunkSize))
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
		if err := u.maybeFlush(ctx, blobChunk.Length()); err != nil {
			return "", fmt.Errorf("flushing pack: %w", err)
		}

		// Link the blob chunk to the file chunk
		fileChunk.Link(u.current.ID, blobChunk)

		// Add the blob chunk to the current pack
		u.current.Add(blobChunk)
	}

	// If adding the file chunk exceeds the max pack size, upload the current pack
	// and start a new one
	if err := u.maybeFlush(ctx, fileChunk.Length()); err != nil {
		return "", fmt.Errorf("flushing pack: %w", err)
	}
	u.current.Add(fileChunk)

	return u.current.ID, nil
}

// Flush the current pack if adding the chunk would exceed the max pack size
func (u *Upload) maybeFlush(ctx context.Context, chunkLength int) error {
	if u.current.Length()+chunkLength < u.MaxPackSize {
		return nil
	}
	return u.Flush(ctx)
}

func (u *Upload) Flush(ctx context.Context) error {
	if u.current.Length() == 0 {
		return nil
	}

	log := logs.Scope(u.log)

	packFile, err := u.current.File()
	if err != nil {
		return err
	}

	now := time.Now()
	if err := u.Limiter.Use(ctx, len(packFile.Data)); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case u.uploadCh <- packFile:
	}

	log.Debug("uploaded pack",
		slog.String("path", packFile.Path),
		slog.Int("size", len(packFile.Data)),
		slog.Duration("time", time.Since(now)),
	)

	u.current = newPackFile()
	return nil
}
