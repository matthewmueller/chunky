# Chunky

Efficiently store versioned data.

![CleanShot 2024-11-04 at 22 49 26@2x](https://github.com/user-attachments/assets/eb0c85c0-4c15-4032-b656-bcc48eaa01d2)

Chunky was built to ship code and binaries to remote servers. Then once on those remote servers, Chunky helps you quickly swap versions.

Chunky is like if rsync and git had a baby.

Chunky uses content-defined-chunking (CDC) powered by Restic's [chunker library](https://github.com/restic/chunker) to efficiently store data on disk. When you upload new versions, only the files that have changed will be uploaded. Preliminary estimates suggest that repos are about half the size of the original codebase, while storing every version!

**Status**: Early release. It's working and I'm using it in production, but it lacks tests and at this stage I mostly optimized the binary files for understandability. I think there's a more efficient way to link and store commits and packs. I'd encourage you to look for ways to make Chunky more efficient!

## Examples

### Upload a directory

```bash
$ chunky upload . vagrant@127.0.0.1:2222/my-repo
```

This example uploads the entire current directory to the `vagrant@127.0.0.1:2222/my-repo` remote repo.

This will create a directory `my-repo` in the `$HOME/vagrant` directory on your remote machine.

### List all versions

```bash
$ chunky list vagrant@127.0.0.1:2222/my-repo
20241105033915 (latest) 69kB Matt Mueller 39 minutes ago
20241105033613          69kB Matt Mueller 42 minutes ago
20241105033612          69kB Matt Mueller 42 minutes ago
20241105033611          69kB Matt Mueller 42 minutes ago
20241105033610 (v0.0.0) 69kB Matt Mueller 42 minutes ago
20241105033609          69kB Matt Mueller 42 minutes ago
20241105033607          69kB Matt Mueller 42 minutes ago
20241105032915          68kB Matt Mueller 49 minutes ago
```

This command lists all the versions on the `vagrant@127.0.0.1:2222/my-repo` remote repo.

### Tag a revision

```bash
$ chunky tag vagrant@127.0.0.1:2222/my-repo 20241105033612 v0.0.1
```

This command tags the `20241105033612` commit with the `v0.0.1` tag

```bash
$ chunky list vagrant@127.0.0.1:2222/my-repo
20241105033915 (latest) 69kB Matt Mueller 39 minutes ago
20241105033613          69kB Matt Mueller 42 minutes ago
20241105033612 (v0.0.1) 69kB Matt Mueller 42 minutes ago
20241105033611          69kB Matt Mueller 42 minutes ago
20241105033610 (v0.0.0) 69kB Matt Mueller 42 minutes ago
20241105033609          69kB Matt Mueller 42 minutes ago
20241105033607          69kB Matt Mueller 42 minutes ago
20241105032915          68kB Matt Mueller 49 minutes ago
```

### Download a version

```bash
$ chunky download vagrant@127.0.0.1:2222/my-repo v0.0.1 my-repo-v1
```

This command downloads the `v0.0.1` revision from the `vagrant@127.0.0.1:2222/my-repo` remote repo into the `my-repo-v1` directory.

By default this overwrites any existing file, but will not delete local files that no longer exist in the remote repository. To fully sync `my-repo-v1` with the remote repository, include the `--sync` flag.

```bash
$ chunky download --sync vagrant@127.0.0.1:2222/my-repo v0.0.1 my-repo-v1
```

## Usage

Chunky ships with a CLI and programmatic API.

### CLI

You can install the CLI using Go with:

```bash
go install github.com/matthewmueller/chunky/cmd/chunky@latest
```

If that succeeds, you should be able to type `chunky` and see the help menu below.

```bash
$ chunky

  Usage:
    $ chunky [command]

  Description:
    efficiently store versioned data

  Commands:
    cat       show a file
    create    create a new repository
    download  download a directory from a repository
    list      list repository
    show      show a revision
    tag       tag a commit
    upload    upload a directory to a repository

  Advanced Commands:
    cat-commit  show a commit
    cat-pack    show a pack
    cat-tag     show a tag
    clean       clean a repository and local cache

```

### API

[![Go Reference](https://pkg.go.dev/badge/github.com/matthewmueller/chunky.svg)](https://pkg.go.dev/github.com/matthewmueller/chunky)

Chunky also includes a programmatic API. For now, that's undocumented, but it should be fairly straightforward to understand by looking in the `./cli` directory.

You can also review the documentation on [go.dev](https://pkg.go.dev/github.com/matthewmueller/chunky).

### Serving a repository over HTTP

The `repos/http` package lets you upload and download over HTTP(S) instead of SFTP. It has two halves: `Handler` serves an existing repository from the remote machine and `New` connects to it.

On the server, wrap the directory holding the repository and mount it in any HTTP server:

```go
import (
	"net/http"

	http_repo "github.com/matthewmueller/chunky/repos/http"
	"github.com/matthewmueller/chunky/repos/local"
	"github.com/matthewmueller/virt"
)

repo := local.New(virt.OS("/var/lib/my-repo"))
http.Handle("/repo/", http.StripPrefix("/repo", http_repo.Handler(repo)))
http.ListenAndServe(":8080", nil)
```

On the client, point the HTTP repository at that URL and use it like any other repository:

```go
repo := http_repo.New("https://example.com/repo")
revision, err := chunky.New(logs.Default()).Upload(ctx, &chunky.Upload{
	From:  virt.OS("."),
	To:    repo,
	Cache: virt.OS(cacheDir),
})
```

`Handler` rejects paths that escape the repository and strips special mode bits (e.g. setuid) from uploads, but it does not authenticate requests. Add your own authentication and TLS in front of it.

The HTTP repository is only available through the Go API for now. The CLI supports local and SFTP repositories.

### Uploading over a one-way stream

The `repos/stream` package carries an upload over any one-way pipe, like SSH stdin or an HTTP request body, and `Receive` writes it into a repository on the other end.

The client can't read the repository through a pipe, so it only skips files it already has in its cache. A fresh client can read the repository's latest commit separately and pass it as `Base`:

```go
import "github.com/matthewmueller/chunky/repos/stream"

// Optional: data is latest.Pack() from the host, read separately
base, err := chunky.UnpackCommit(data)

to := stream.New(stdin) // an io.WriteCloser
revision, err := chunky.New(logs.Default()).Upload(ctx, &chunky.Upload{
	From:  virt.OS("."),
	To:    to,
	Cache: virt.OS(cacheDir),
	Base:  base,
})
err = to.Close()
```

On the host, receive the stream into a repository:

```go
repo := local.New(virt.OS("/var/lib/my-repo"))
revision, err := chunky.New(logs.Default()).Receive(ctx, &chunky.Receive{
	From: os.Stdin,
	To:   repo,
})
if errors.Is(err, chunky.ErrStale) {
	// The upload references a pack the repository doesn't have. The client
	// should clear its cache, read the base again and retry once.
}
```

`Receive` only writes the commit and its tags once the stream ends with the latest tag, so a cut or refused stream leaves at most unreferenced packs.

## Similar Tools

- **Git:** You need to use a Git extension to store large binaries and overall requires too much ceremony when you just want to sync a directory on a remote machine. You may also want to sync files in your `.gitignore` to production servers (e.g. compiled assets).
- **Restic:** An excellent file backup tool. Chunky took a lot of design inspiration from Restic. Restic doesn't have a programmatic API and the backups are encrypted, which is not helpful in a server-side setting.
- **Rsync:** Great for syncing files to a remote machine, but does not store older versions of those files.

## Adding New Repositories

Chunky currently supports three repository backends:

1. `Local`: Store your repository in your local filesystem
2. `SFTP`: Store your repository on a remote server
3. `HTTP`: Store your repository on a remote server that serves it with `http.Handler`

I'd encourage you to contribute new repository backends to Chunky. A backend you can only write to, like `repos/stream`, implements `Writer`. One you can also read from implements `Repo`:

```go
type Writer interface {
	// Upload from a filesystem to the repository
	Upload(ctx context.Context, file *File) error
	// Close the repository
	Close() error
}

type Repo interface {
	Writer
	// Download paths from the repository to a filesystem
	Download(ctx context.Context, path string) (*File, error)
	// ReadDir reads a directory from the repository
	ReadDir(dir string) ([]fs.DirEntry, error)
}
```

## Development

First, clone the repo:

```bash
git clone https://github.com/matthewmueller/chunky
cd chunky
```

Next, install dependencies:

```bash
go mod tidy
```

Finally, try running the tests:

```bash
go test ./...
```

## License

MIT
