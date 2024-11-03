package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"time"

	"github.com/livebud/cli"
	"github.com/matthewmueller/chunky/internal/chunker"
	"github.com/matthewmueller/chunky/internal/commits"
	"github.com/matthewmueller/chunky/internal/gitignore"
	"github.com/matthewmueller/chunky/internal/gzip"
	"github.com/matthewmueller/virt"
)

type Upload struct {
	From     string
	To       string
	Subpaths []string
	Tag      *string
	Message  string
}

// upload [--tag=<tag> --message=<msg>] <from> <to> [subpaths...]
func (u *Upload) Command(cli cli.Command) cli.Command {
	cmd := cli.Command("upload", "upload a directory to a repository")
	cmd.Arg("from", "directory to upload").String(&u.From)
	cmd.Arg("repo", "repository to upload to").String(&u.To)
	cmd.Args("subpaths", "subpaths to upload").Strings(&u.Subpaths).Default()
	cmd.Flag("tag", "tag the revision").Short('t').Optional().String(&u.Tag)
	return cmd
}

func (u *Upload) Validate() (err error) {
	if u.Tag != nil && *u.Tag == "latest" {
		err = errors.Join(err, errors.New("tag cannot be 'latest'"))
	}
	return
}

func (c *CLI) Upload(ctx context.Context, in *Upload) error {
	if err := in.Validate(); err != nil {
		return fmt.Errorf("cli: validating upload: %w", err)
	}
	repo, err := c.loadRepo(in.To)
	if err != nil {
		return err
	}

	fsys, err := c.loadFS(in.From)
	if err != nil {
		return err
	}

	ignore := gitignore.FromFS(fsys)

	tree := virt.Tree{}
	commit := commits.New()
	commit.CreatedAt = time.Now()

	checksum := sha256.New()
	size := uint64(0)

	// Walk over the files, chunk them and add them to the file system we're going
	// to upload. We'll also add each file to the commit object.
	// TODO: handle subpaths
	if err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		} else if d.IsDir() || ignore(path) {
			return nil
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		file := commit.File(path, info)
		checksum.Write(data)
		size += uint64(info.Size())

		// create a chunker
		chunker := chunker.New(data)
		for {
			chunk, err := chunker.Chunk()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return err
			}
			hash := chunk.Hash()
			fpath := fmt.Sprintf("objects/%s/%s", hash[:2], hash[2:])
			compressed, err := gzip.Compress(chunk.Data)
			if err != nil {
				return err
			}
			tree[fpath] = &virt.File{
				Data: compressed,
				Mode: info.Mode(),
			}
			file.Add(hash)
		}
		return nil
	}); err != nil {
		return err
	}

	// add the checksum to the commit
	commit.Checksum = hex.EncodeToString(checksum.Sum(nil))
	commit.Size = size

	// add the commit file
	if err := commits.Write(ctx, tree, commit); err != nil {
		return fmt.Errorf("cli: unable to write commit: %w", err)
	}

	// Add the latest ref
	commitHash := commit.Hash()
	tree["tags/latest"] = &virt.File{
		Data: []byte(commitHash),
		Mode: 0644,
	}

	// Tag the revision
	if in.Tag != nil {
		tree[fmt.Sprintf("tags/%s", *in.Tag)] = &virt.File{
			Data: []byte(commitHash),
			Mode: 0644,
		}
	}

	return repo.Upload(ctx, tree)
}
