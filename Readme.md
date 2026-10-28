# Chunky

[![Go Reference](https://pkg.go.dev/badge/github.com/matthewmueller/chunky.svg)](https://pkg.go.dev/github.com/matthewmueller/chunky)

Chunky uploads directories to remote servers and keeps every version, like rsync with history. Uploads only send the files that changed, and content-defined chunking keeps a repository of every version smaller than the original directory.

![Chunky](https://github.com/user-attachments/assets/eb0c85c0-4c15-4032-b656-bcc48eaa01d2)

## Install

```sh
go install github.com/matthewmueller/chunky/cmd/chunky@latest
```

## Example

Upload the current directory to a repository over SFTP:

```sh
chunky upload . vagrant@127.0.0.1:2222/my-repo
```

Without SFTP, stream the upload over SSH to `chunky` on the remote machine:

```sh
chunky upload . - | ssh vagrant@127.0.0.1 -p 2222 chunky upload - my-repo
```

List the versions, tag one and download it:

```sh
$ chunky list vagrant@127.0.0.1:2222/my-repo
3LJWjx7QYf2ZE6we2JDLvtqilJy [latest] 69kB matt 3 minutes ago
3LJWixXt1Zjt16Y3yVkgvXMqBLI          68kB matt 1 hour ago

$ chunky tag --revision 3LJWixXt1Zjt16Y3yVkgvXMqBLI vagrant@127.0.0.1:2222/my-repo v0.0.1
$ chunky download --revision v0.0.1 vagrant@127.0.0.1:2222/my-repo my-repo-v1
```

## CLI Reference

```
Usage:
  $ chunky [flags] [command]

Commands:
  cat       show a file
  create    create a new repository
  download  download a directory from a repository
  list      list uploads to a repository
  show      show a revision
  tag       tag a commit
  tags      list tags
  upload    upload a directory to a repository

Advanced Commands:
  cache-prune  prune a repository and local cache
  cache-size   print the size of the cache
  cat-commit   show a commit
  cat-pack     show a pack
  cat-tag      show a tag
```

Uploads remember what each repository has in a local cache. If a streamed upload is refused as stale, usually because the repository is new or was reset, run `chunky cache-prune -` and try again.

## API Reference

Upload and download with any repository:

```go
chky := chunky.New(logs.Default())
repo := local.New(virt.OS("/var/lib/my-repo"))

revision, err := chky.Upload(ctx, &chunky.Upload{
	From:  virt.OS("."),
	To:    repo,
	Cache: virt.OS(cacheDir),
})

err = chky.Download(ctx, &chunky.Download{
	From:     repo,
	To:       virt.OS("my-repo-v1"),
	Revision: revision,
})
```

### Repositories

- `repos/local`: a directory on this machine
- `repos/sftp`: a directory on a remote machine over SFTP
- `repos/http`: a repository served by `http.Handler`
- `repos/stream`: a write-only stream, like SSH stdin or a request body

Serve a repository over HTTP. `Handler` doesn't authenticate requests, so put your own auth and TLS in front of it:

```go
repo := local.New(virt.OS("/var/lib/my-repo"))
http.Handle("/repo/", http.StripPrefix("/repo", http_repo.Handler(repo)))

// On the client
repo := http_repo.New("https://example.com/repo")
```

New backends implement `repos.Repo`, or only `repos.Writer` if they can't be read:

```go
type Writer interface {
	Upload(ctx context.Context, file *File) error
	Close() error
}

type Repo interface {
	Writer
	Download(ctx context.Context, path string) (*File, error)
	ReadDir(dir string) ([]fs.DirEntry, error)
}
```

### Uploading over a one-way stream

Ask the host which files it already has, then stream only the rest.

On the client:

```go
upload := &chunky.Upload{From: virt.OS(".")}

// 1. Send the manifest to the host, and read back the files it already has
manifest, err := chky.Manifest(ctx, upload) // a map of path to hash
err = json.NewEncoder(request).Encode(manifest)
err = json.NewDecoder(response).Decode(&upload.Matches)

// 2. Stream the rest of the files to the host
upload.To = stream.New(w) // w is the body of a second request, like ssh stdin
revision, err := chky.Upload(ctx, upload)
err = upload.To.Close()
```

On the host:

```go
// 1. Read the client's manifest, and answer with the files the repository already has
var manifest chunky.Manifest
err := json.NewDecoder(request).Decode(&manifest)
matches, err := chky.Matches(ctx, repo, manifest)
err = json.NewEncoder(response).Encode(matches)

// 2. Receive the client's stream. The commit is only written once the whole
// stream arrives, and ErrStale means it references packs the repo doesn't have.
revision, err := chky.Receive(ctx, r, repo) // r is the second request's body
```

## Development

```sh
git clone https://github.com/matthewmueller/chunky
cd chunky
make test
```
