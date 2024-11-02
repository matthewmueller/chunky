package cli

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"time"

	"github.com/livebud/cli"
	"github.com/matthewmueller/chunky/internal/commits"
	"github.com/matthewmueller/chunky/internal/gitignore"
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
	cmd.Flag("message", "message for the revision").Short('m').String(&u.Message).Default("")
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
	commit := commits.New(in.Message)
	commit.CreatedAt = time.Now()

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

		// create a chunker
		chunker := c.chunker(data)
		buf := make([]byte, chunker.MaxSize)
		for {
			chunk, err := chunker.Next(buf)
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return err
			}
			hash := sha256.Sum256(chunk.Data)
			fpath := fmt.Sprintf("objects/%02x/%02x", hash[:1], hash[1:])
			tree[fpath] = &virt.File{
				Data: chunk.Data,
				Mode: info.Mode(),
			}
			file.Add(fmt.Sprintf("%02x", hash))
		}
		return nil
	}); err != nil {
		return err
	}

	// add the commit file
	commitData, err := json.MarshalIndent(commit, "", "  ")
	if err != nil {
		return err
	}
	commitHash := fmt.Sprintf("%02x", sha256.Sum256(commitData))
	commitPath := fmt.Sprintf("commits/%s", commitHash)
	tree[commitPath] = &virt.File{
		Data: commitData,
		Mode: 0644,
	}
	fmt.Println(commitHash)
	// Add the latest ref
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
