package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"

	"github.com/dustin/go-humanize"
	"github.com/livebud/cli"
	"github.com/matthewmueller/chunky/internal/commits"
	"github.com/matthewmueller/chunky/internal/repos"
	"github.com/matthewmueller/virt"
)

type List struct {
	Repo string
}

func (l *List) Command(cli cli.Command) cli.Command {
	cmd := cli.Command("list", "list repository")
	cmd.Arg("repo", "repo to list from").String(&l.Repo)
	return cmd
}

func (c *CLI) loadCommitFiles(ctx context.Context, repo repos.Repo) (files []*virt.File, err error) {
	// Load commit files
	if err := repo.Walk(ctx, "commits", func(path string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if de.IsDir() {
			return nil
		}
		commitFile, err := repos.Download(ctx, repo, path)
		if err != nil {
			return err
		}
		info, err := de.Info()
		if err != nil {
			return fmt.Errorf("cli: unable to get file info for %s: %w", path, err)
		}
		// Update the commit file's mod time to when the file was uploaded
		commitFile.ModTime = info.ModTime()
		files = append(files, commitFile)
		return nil
	}); err != nil {
		return nil, err
	}
	// Sort by mod time
	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime.After(files[j].ModTime)
	})
	return files, nil
}

func (c *CLI) loadTagMap(ctx context.Context, repo repos.Repo) (tags map[string][]string, err error) {
	tags = map[string][]string{}
	if err := repo.Walk(ctx, "tags", func(fpath string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		} else if de.IsDir() {
			return nil
		}
		tagFile, err := repos.Download(ctx, repo, fpath)
		if err != nil {
			return err
		}
		tags[string(tagFile.Data)] = append(tags[string(tagFile.Data)], path.Base(fpath))
		return nil
	}); err != nil {
		return nil, err
	}
	return tags, nil
}

func (c *CLI) List(ctx context.Context, in *List) error {
	repo, err := c.loadRepo(in.Repo)
	if err != nil {
		return err
	}
	tagMap, err := c.loadTagMap(ctx, repo)
	if err != nil {
		return err
	}
	commitFiles, err := c.loadCommitFiles(ctx, repo)
	if err != nil {
		return err
	}
	for _, commitFile := range commitFiles {
		hash := path.Base(commitFile.Path)
		tags := tagMap[hash]
		_ = tags
		var commit *commits.Commit
		if err := json.Unmarshal(commitFile.Data, &commit); err != nil {
			return err
		}
		fmt.Fprintf(c.Stdout, "%s %s %+v\n", hash, humanize.Bytes(commit.Size()), tags)
	}
	return nil
}
