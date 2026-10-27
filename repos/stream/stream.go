package stream

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sync"

	"github.com/matthewmueller/chunky/internal/commits"
	"github.com/matthewmueller/chunky/repos"
)

// ErrStale means the receiving repo is missing the base or a pack the commit
// references. Re-read the base and retry once.
var ErrStale = errors.New("stream: stale base")

// New writes uploads to w as a tar stream that Receive reads
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

// Receive writes a stream from Writer into a repository and returns the
// uploaded commit. The commit and tags are only written once the stream ends
// with tags/latest, so a cut stream leaves at most orphan packs.
func Receive(ctx context.Context, r io.Reader, to repos.Repo) (commitId string, err error) {
	tr := tar.NewReader(r)
	var commit *repos.File
	var tags []*repos.File
	latest := false
	for first := true; ; first = false {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return "", fmt.Errorf("stream: unable to read entry: %w", err)
		}
		if header.Typeflag != tar.TypeReg || !fs.ValidPath(header.Name) {
			return "", fmt.Errorf("stream: invalid entry %q", header.Name)
		}
		dir, name := path.Split(header.Name)
		data, err := io.ReadAll(tr)
		if err != nil {
			return "", fmt.Errorf("stream: unable to read %q: %w", header.Name, err)
		}
		file := &repos.File{
			Path:    header.Name,
			Data:    data,
			Mode:    fs.FileMode(header.Mode).Perm(),
			ModTime: header.ModTime,
		}
		switch {
		case header.Name == "base" && first:
			if err := checkBase(ctx, to, string(data)); err != nil {
				return "", err
			}
		case dir == "packs/" && commit == nil:
			if err := to.Upload(ctx, file); err != nil {
				return "", fmt.Errorf("stream: unable to upload %q: %w", header.Name, err)
			}
		case dir == "commits/" && commit == nil:
			if err := checkCommit(to, name, data); err != nil {
				return "", err
			}
			commit, commitId = file, name
		case dir == "tags/" && commit != nil:
			if string(data) != commitId {
				return "", fmt.Errorf("stream: tag %q doesn't name commit %s", name, commitId)
			}
			tags = append(tags, file)
			latest = latest || name == "latest"
		default:
			return "", fmt.Errorf("stream: unexpected entry %q", header.Name)
		}
	}
	if !latest {
		return "", errors.New("stream: ended before tags/latest named the commit")
	}
	for _, file := range append([]*repos.File{commit}, tags...) {
		if err := to.Upload(ctx, file); err != nil {
			return "", fmt.Errorf("stream: unable to upload %q: %w", file.Path, err)
		}
	}
	return commitId, nil
}

// checkBase refuses a base the repo doesn't have. An empty base means the
// client is sending every file.
func checkBase(ctx context.Context, to repos.Repo, baseId string) error {
	if baseId == "" {
		return nil
	}
	if !fs.ValidPath(baseId) || path.Base(baseId) != baseId {
		return fmt.Errorf("stream: invalid base %q", baseId)
	}
	if _, err := to.Download(ctx, path.Join("commits", baseId)); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w: missing commit %s", ErrStale, baseId)
		}
		return fmt.Errorf("stream: unable to check base %s: %w", baseId, err)
	}
	return nil
}

// checkCommit refuses a commit that references a pack the repo doesn't have
func checkCommit(to repos.Repo, commitId string, data []byte) error {
	commit, err := commits.Unpack(data)
	if err != nil {
		return fmt.Errorf("stream: unable to unpack commit %s: %w", commitId, err)
	}
	des, err := to.ReadDir("packs")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("stream: unable to read packs: %w", err)
	}
	present := make(map[string]bool, len(des))
	for _, de := range des {
		present[de.Name()] = true
	}
	for _, pack := range commit.Packs() {
		if !present[pack.ID] {
			return fmt.Errorf("%w: missing pack %s", ErrStale, pack.ID)
		}
	}
	return nil
}
