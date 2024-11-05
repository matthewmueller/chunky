# Chunky

[![Go Reference](https://pkg.go.dev/badge/github.com/matthewmueller/chunky.svg)](https://pkg.go.dev/github.com/matthewmueller/chunky)

Efficiently store versioned data.

Chunky was built to ship code and binaries to remote servers. Then once on those remote servers, Chunky helps you quickly swap versions.

Chunky uses content-defined-chunking (CDC) using Restic's [chunker library](https://github.com/restic/chunker) to efficiently store data on disk. When you upload new versions, only the files that have changed will be uploaded. Preliminary estimates suggest that repos are about half the size of the original codebase, while storing every version!

**Status**: Early release. It's working and I'm using it in production, but it lacks tests and at this stage I mostly optimized the binary files for understandability. I think there's a more efficient way to link and store commits and packs. I'd encourage you to look for ways to make Chunky more efficient!

## Install

```sh
go install github.com/matthewmueller/chunky@latest
```

## Usage

Chunky ships with a CLI and programmatic API.

### CLI

```sh

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

TBD. See godoc above for now.

## Similar Tools

- **Git:** You need to use a Git extension to store large binaries and overall requires too much ceremony when you just want to sync a directory on a remote machine. You may also want to sync files in your `.gitignore` to production servers (e.g. compiled assets).
- **Restic:** An excellent file backup tool. Chunky took a lot of design inspiration from Restic. Restic doesn't have a programmatic API and the backups are encrypted, which is not helpful in a server-side setting.
- **Rsync:** Great for syncing files to a remote machine, but does not store older versions of those files.

## Adding New Repositories

Chunky currently supports two repository backends:

1. `Local`: Store your repository in your local filesystem
2. `SFTP`: Store your repository on a remote server

I'd encourage you to contribute new repository backends to Chunky. The interface is quite straightforward to implement:

```go
type Repo interface {
	Upload(ctx context.Context, from fs.FS) error
	Download(ctx context.Context, to virt.FS, paths ...string) error
	Walk(ctx context.Context, dir string, fn fs.WalkDirFunc) error
	Close() error
}
```

## Development

First, clone the repo:

```sh
git clone https://github.com/matthewmueller/chunky
cd chunky
```

Next, install dependencies:

```sh
go mod tidy
```

Finally, try running the tests:

```sh
go test ./...
```

## License

MIT
