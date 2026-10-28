package stream

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sync"

	"github.com/matthewmueller/chunky/repos"
)

// New writes uploads to w as a tar stream that Read reads
func New(w io.WriteCloser) *Writer {
	return &Writer{w: w, tw: tar.NewWriter(w)}
}

// Writer is a write-only repository. Uploads may be concurrent.
type Writer struct {
	mu sync.Mutex
	w  io.WriteCloser
	tw *tar.Writer
}

var _ repos.Writer = (*Writer)(nil)

func (w *Writer) Upload(ctx context.Context, file *repos.File) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeReg,
		Name:     file.Path,
		Mode:     int64(file.Mode.Perm()),
		Size:     int64(len(file.Data)),
		ModTime:  file.ModTime,
	}); err != nil {
		return fmt.Errorf("stream: unable to write header for %q: %w", file.Path, err)
	}
	if _, err := w.tw.Write(file.Data); err != nil {
		return fmt.Errorf("stream: unable to write %q: %w", file.Path, err)
	}
	return nil
}

// Close ends the tar stream and closes the underlying writer
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return errors.Join(w.tw.Close(), w.w.Close())
}

// Read calls fn with each file in a stream from Writer. Only regular files
// with valid paths are allowed, and modes are masked to their permissions. A
// stream cut between files reads as a clean end, so callers decide whether
// the stream is complete.
func Read(r io.Reader, fn func(file *repos.File) error) error {
	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		} else if err != nil {
			return fmt.Errorf("stream: unable to read entry: %w", err)
		}
		if header.Typeflag != tar.TypeReg || !fs.ValidPath(header.Name) {
			return fmt.Errorf("stream: invalid entry %q", header.Name)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return fmt.Errorf("stream: unable to read %q: %w", header.Name, err)
		}
		if err := fn(&repos.File{
			Path:    header.Name,
			Data:    data,
			Mode:    fs.FileMode(header.Mode).Perm(),
			ModTime: header.ModTime,
		}); err != nil {
			return err
		}
	}
}
