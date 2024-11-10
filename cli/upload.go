package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"time"

	"github.com/livebud/cli"
	"github.com/matthewmueller/chunky/internal/caches"
	"github.com/matthewmueller/chunky/internal/chunkyignore"
	"github.com/matthewmueller/chunky/internal/commits"
	"github.com/matthewmueller/chunky/internal/packs"
	"github.com/matthewmueller/chunky/internal/repos"
	"github.com/matthewmueller/chunky/internal/sha256"
	"github.com/matthewmueller/virt"
)

type Upload struct {
	From  string
	To    string
	Tag   *string
	Cache bool
}

func (u *Upload) command(cli cli.Command) cli.Command {
	cmd := cli.Command("upload", "upload a directory to a repository")
	cmd.Arg("from", "directory to upload").String(&u.From)
	cmd.Arg("repo", "repository to upload to").String(&u.To)
	cmd.Flag("tag", "tag the revision").Short('t').Optional().String(&u.Tag)
	cmd.Flag("cache", "use a cache").Bool(&u.Cache).Default(true)
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

	repoUrl, err := repos.Parse(in.To)
	if err != nil {
		return err
	}

	repo, err := c.loadRepoFromUrl(ctx, repoUrl)
	if err != nil {
		return err
	}

	var cache caches.Cache = caches.None
	if in.Cache {
		cache, err = caches.Download(ctx, repo, repoUrl)
		if err != nil {
			return err
		}
	}

	fsys, err := c.loadFS(in.From)
	if err != nil {
		return err
	}

	user, err := c.getUser()
	if err != nil {
		return err
	}

	ignore := chunkyignore.FromFS(fsys)
	createdAt := time.Now().UTC()
	commit := commits.New(user, createdAt)
	commitId := commit.ID()
	pack := packs.New()

	// Walk over the files, chunk them and add them to the file system we're going
	// to upload. We'll also add each file to the commit object.
	// TODO: handle subpaths
	if err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		} else if d.IsDir() || ignore(path) {
			return nil
		}

		file, err := fsys.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		fileHash := sha256.Hash(data)

		info, err := file.Stat()
		if err != nil {
			return err
		}

		// Check if the file is already in the pack
		// TODO: right now this will duplicate content when the file path in the
		// pack is different from the file path in the commit. We should add a
		// way to alias files in the pack to other packs.
		if commitFile, ok := cache.Get(fileHash); ok && commitFile.Path == path {
			commit.Add(&commits.File{
				Path:   path,
				Size:   uint64(info.Size()),
				Id:     fileHash,
				PackId: commitFile.PackId,
			})
			return nil
		}

		entry := &packs.File{
			Path:    path,
			Mode:    info.Mode(),
			ModTime: info.ModTime(),
			Data:    data,
		}

		// Add the entry to the pack
		if err := pack.Add(entry); err != nil {
			return err
		}

		// Add the file to the commit
		commit.Add(&commits.File{
			Path:   path,
			Id:     fileHash,
			PackId: commitId,
			Size:   uint64(info.Size()),
		})

		return nil
	}); err != nil {
		return err
	}

	tree := virt.Tree{}

	// Add the pack to the tree
	packData, err := pack.Pack()
	if err != nil {
		return err
	}
	if len(packData) > 0 {
		tree[path.Join("packs", commitId)] = &virt.File{
			Data:    packData,
			Mode:    0644,
			ModTime: createdAt,
		}
	}

	// Add the commit to the tree
	commitData, err := commit.Pack()
	if err != nil {
		return err
	}
	tree[path.Join("commits", commitId)] = &virt.File{
		Data:    commitData,
		Mode:    0644,
		ModTime: createdAt,
	}

	// Add the commit to the cache
	if err := cache.Set(commitId, commit); err != nil {
		return err
	}

	// Add the latest ref
	tree["tags/latest"] = &virt.File{
		Data: []byte(commitId),
		Mode: 0644,
	}

	// Tag the revision
	if in.Tag != nil {
		tree[fmt.Sprintf("tags/%s", *in.Tag)] = &virt.File{
			Data: []byte(commitId),
			Mode: 0644,
		}
	}

	return repo.Upload(ctx, tree)
}
