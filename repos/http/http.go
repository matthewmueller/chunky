package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/matthewmueller/chunky/repos"
	"github.com/matthewmueller/virt"
)

// modeHeader carries a file's mode, since the body only carries its data
const modeHeader = "Chunky-Mode"

// New creates a repository that reads and writes files served by Handler at
// url
func New(client *http.Client, url string) *Repo {
	return &Repo{client, strings.TrimSuffix(url, "/")}
}

type Repo struct {
	client *http.Client
	url    string
}

var _ repos.Repo = (*Repo)(nil)

func (r *Repo) Upload(ctx context.Context, file *repos.File) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, r.fileUrl(file.Path), bytes.NewReader(file.Data))
	if err != nil {
		return fmt.Errorf("http: unable to create upload request for %q: %w", file.Path, err)
	}
	req.Header.Set(modeHeader, strconv.FormatUint(uint64(file.Mode), 10))
	if _, err := r.do(req); err != nil {
		return fmt.Errorf("http: unable to upload %q: %w", file.Path, err)
	}
	return nil
}

func (r *Repo) Download(ctx context.Context, fpath string) (*repos.File, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.fileUrl(fpath), nil)
	if err != nil {
		return nil, fmt.Errorf("http: unable to create download request for %q: %w", fpath, err)
	}
	res, err := r.do(req)
	if err != nil {
		return nil, fmt.Errorf("http: unable to download %q: %w", fpath, err)
	}
	mode, err := strconv.ParseUint(res.header.Get(modeHeader), 10, 32)
	if err != nil {
		return nil, fmt.Errorf("http: unable to parse the mode of %q: %w", fpath, err)
	}
	return &repos.File{
		Path: fpath,
		Data: res.body,
		Mode: fs.FileMode(mode),
	}, nil
}

func (r *Repo) ReadDir(dir string) ([]fs.DirEntry, error) {
	// A trailing slash lists the directory instead of downloading it
	req, err := http.NewRequest(http.MethodGet, r.fileUrl(dir)+"/", nil)
	if err != nil {
		return nil, fmt.Errorf("http: unable to create read directory request for %q: %w", dir, err)
	}
	res, err := r.do(req)
	if err != nil {
		return nil, fmt.Errorf("http: unable to read directory %q: %w", dir, err)
	}
	var entries []*virt.DirEntry
	if err := json.Unmarshal(res.body, &entries); err != nil {
		return nil, fmt.Errorf("http: unable to decode directory %q: %w", dir, err)
	}
	des := make([]fs.DirEntry, len(entries))
	for i, entry := range entries {
		des[i] = entry
	}
	return des, nil
}

func (r *Repo) Close() error {
	return nil
}

func (r *Repo) fileUrl(fpath string) string {
	if fpath == "." {
		return r.url
	}
	return r.url + "/" + (&url.URL{Path: fpath}).EscapedPath()
}

type response struct {
	header http.Header
	body   []byte
}

// do sends the request, mapping a missing file to fs.ErrNotExist
func (r *Repo) do(req *http.Request) (*response, error) {
	res, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	switch {
	case res.StatusCode == http.StatusNotFound:
		return nil, fs.ErrNotExist
	case res.StatusCode >= 300:
		return nil, fmt.Errorf("%s: %s", res.Status, strings.TrimSpace(string(body)))
	}
	return &response{res.Header, body}, nil
}

// Handler serves the repository to Repo. A path with a trailing slash lists
// that directory.
func Handler(repo repos.Repo) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fpath := strings.TrimPrefix(r.URL.Path, "/")
		dir := fpath == "" || strings.HasSuffix(fpath, "/")
		if fpath = strings.TrimSuffix(fpath, "/"); fpath == "" {
			fpath = "."
		}
		if !fs.ValidPath(fpath) {
			http.Error(w, fmt.Sprintf("invalid path %q", fpath), http.StatusBadRequest)
			return
		}
		var err error
		switch {
		case r.Method == http.MethodGet && dir:
			err = serveDir(w, repo, fpath)
		case r.Method == http.MethodGet:
			err = serveFile(r.Context(), w, repo, fpath)
		case r.Method == http.MethodPut && !dir:
			err = uploadFile(r, repo, fpath)
		default:
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		if errors.Is(err, fs.ErrNotExist) {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
}

func serveDir(w http.ResponseWriter, repo repos.Repo, dir string) error {
	des, err := repo.ReadDir(dir)
	if err != nil {
		return err
	}
	entries := make([]*virt.DirEntry, len(des))
	for i, de := range des {
		entries[i] = &virt.DirEntry{Path: de.Name(), Mode: de.Type()}
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(data)
	return err
}

func serveFile(ctx context.Context, w http.ResponseWriter, repo repos.Repo, fpath string) error {
	file, err := repo.Download(ctx, fpath)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set(modeHeader, strconv.FormatUint(uint64(file.Mode), 10))
	_, err = w.Write(file.Data)
	return err
}

func uploadFile(r *http.Request, repo repos.Repo, fpath string) error {
	mode, err := strconv.ParseUint(r.Header.Get(modeHeader), 10, 32)
	if err != nil {
		return fmt.Errorf("http: unable to parse the mode of %q: %w", fpath, err)
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("http: unable to read %q: %w", fpath, err)
	}
	return repo.Upload(r.Context(), &repos.File{
		Path: fpath,
		Data: data,
		// Only permissions and the directory bit, so clients can't create
		// setuid files or other special files in the repository
		Mode: fs.FileMode(mode) & (fs.ModeDir | fs.ModePerm),
	})
}
