package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"

	"github.com/livebud/cli"
	"github.com/matthewmueller/chunky/internal/commits"
	"github.com/matthewmueller/chunky/internal/repo"
	"github.com/matthewmueller/virt"
)

type Download struct {
	From     string
	To       string
	Subpaths []string
	Revision string
	Sync     bool
}

func (d *Download) Command(cli cli.Command) cli.Command {
	cmd := cli.Command("download", "download a directory from a repository")
	cmd.Arg("from", "repository to download from").String(&d.From)
	cmd.Arg("to", "directory to download to").String(&d.To)
	cmd.Args("subpaths", "subpaths to download").Strings(&d.Subpaths).Default()
	cmd.Flag("revision", "revision to download").Short('r').String(&d.Revision).Default("latest")
	cmd.Flag("sync", "sync the repository before downloading").Bool(&d.Sync).Default(false)
	return cmd
}

func downloadFile(ctx context.Context, repo repo.Repo, path string) (*virt.File, error) {
	fsys := virt.Tree{}
	if err := repo.Download(ctx, fsys, path); err != nil {
		return nil, err
	}
	return fsys[path], nil
}

func loadCommitSha(ctx context.Context, repo repo.Repo, revision string) (string, error) {
	if revision == "" {
		revision = "latest"
	}
	// Try to download the commit directly
	if _, err := downloadFile(ctx, repo, "commits/"+revision); err == nil {
		return revision, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("cli: unable to download commit: %w", err)
	}
	// Try to download the tag
	tag, err := downloadFile(ctx, repo, "tags/"+revision)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("cli: revision not found: %s", revision)
		}
		return "", fmt.Errorf("cli: unable to download tag: %w", err)
	}
	return string(tag.Data), nil
}

func loadCommit(ctx context.Context, repo repo.Repo, revision string) (*commits.Commit, error) {
	commitSha, err := loadCommitSha(ctx, repo, revision)
	if err != nil {
		return nil, err
	}
	commitFile, err := downloadFile(ctx, repo, "commits/"+commitSha)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("cli: commit not found: %s", commitSha)
		}
		return nil, err
	}
	var commit *commits.Commit
	if err := json.Unmarshal(commitFile.Data, &commit); err != nil {
		return nil, fmt.Errorf("cli: unable to unmarshal commit: %w", err)
	}
	return commit, nil
}

func downloadCommitFile(ctx context.Context, repo repo.Repo, file *commits.File) (*virt.File, error) {
	vfile := &virt.File{
		Path:    file.Path,
		Mode:    file.Mode,
		ModTime: file.ModTime,
	}
	for _, object := range file.Objects {
		objectPath := fmt.Sprintf("objects/%s/%s", object[:2], object[2:])
		dataFile, err := downloadFile(ctx, repo, objectPath)
		if err != nil {
			return nil, fmt.Errorf("cli: unable to download object %q: %w", object, err)
		}
		vfile.Data = append(vfile.Data, dataFile.Data...)
	}
	return vfile, nil
}

func (c *CLI) Download(ctx context.Context, in *Download) error {
	// Load the repository to download from
	repo, err := c.loadRepo(in.From)
	if err != nil {
		return err
	}

	// Load the virtual filesystem to download into
	to, err := c.loadFS(in.To)
	if err != nil {
		return err
	}

	// Load the commit
	commit, err := loadCommit(ctx, repo, in.Revision)
	if err != nil {
		return fmt.Errorf("cli: unable to load commit %q: %w", in.Revision, err)
	}

	// Download into a virtual tree
	tree := virt.Tree{}
	for _, file := range commit.Files {
		vfile, err := downloadCommitFile(ctx, repo, file)
		if err != nil {
			return fmt.Errorf("cli: unable to download file %q: %w", file.Path, err)
		}
		tree[file.Path] = vfile
	}

	// Write the virtual tree to the filesystem
	return virt.WriteFS(tree, to)
}
