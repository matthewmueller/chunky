package http_test

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/matryer/is"
	"github.com/matthewmueller/chunky"
	http_repo "github.com/matthewmueller/chunky/repos/http"
	"github.com/matthewmueller/chunky/repos/local"
	"github.com/matthewmueller/logs"
	"github.com/matthewmueller/virt"
)

func httpServer(t testing.TB, dir string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.StripPrefix("/repo", http_repo.Handler(local.New(virt.OS(dir)))))
	t.Cleanup(server.Close)
	return server
}

func TestUploadDownload(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	log := logs.Default()
	server := httpServer(t, t.TempDir())

	from := virt.Tree{
		"a.txt":     &virt.File{Data: []byte("a"), Mode: 0644},
		"b.txt":     &virt.File{Data: []byte("b"), Mode: 0644},
		"dir/c.txt": &virt.File{Data: []byte("c"), Mode: 0755},
	}

	repo := http_repo.New(server.Client(), server.URL+"/repo")
	cache := virt.OS(t.TempDir())
	chky := chunky.New(log)
	revision, err := chky.Upload(ctx, &chunky.Upload{
		From:  from,
		To:    repo,
		Cache: cache,
	})
	is.NoErr(err)
	is.True(revision != "")

	// Upload again so the cache downloads the commits listed by the server
	from["b.txt"].Data = []byte("b2")
	revision, err = chky.Upload(ctx, &chunky.Upload{
		From:  from,
		To:    repo,
		Cache: virt.OS(t.TempDir()),
	})
	is.NoErr(err)
	is.True(revision != "")

	toFs := virt.OS(t.TempDir())
	err = chky.Download(ctx, &chunky.Download{
		From:     repo,
		To:       toFs,
		Revision: "latest",
	})
	is.NoErr(err)

	data, err := fs.ReadFile(toFs, "a.txt")
	is.NoErr(err)
	is.Equal(string(data), "a")
	data, err = fs.ReadFile(toFs, "b.txt")
	is.NoErr(err)
	is.Equal(string(data), "b2")
	data, err = fs.ReadFile(toFs, "dir/c.txt")
	is.NoErr(err)
	is.Equal(string(data), "c")
	stat, err := toFs.Lstat("dir/c.txt")
	is.NoErr(err)
	is.Equal(stat.Mode(), fs.FileMode(0755))
}

func TestNotExist(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	server := httpServer(t, t.TempDir())
	repo := http_repo.New(server.Client(), server.URL+"/repo")

	_, err := repo.Download(ctx, "commits/missing")
	is.True(errors.Is(err, fs.ErrNotExist))
	_, err = repo.ReadDir("commits")
	is.True(errors.Is(err, fs.ErrNotExist))
}

func TestInvalidPath(t *testing.T) {
	is := is.New(t)
	server := httpServer(t, t.TempDir())

	res, err := server.Client().Get(server.URL + "/repo/tags/../../secret")
	is.NoErr(err)
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	is.NoErr(err)
	is.Equal(res.StatusCode, http.StatusBadRequest)
	is.Equal(string(body), "invalid path \"tags/../../secret\"\n")
}
