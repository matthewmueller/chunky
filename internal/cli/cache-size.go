package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/livebud/cli"
	"github.com/matthewmueller/chunky/internal/humanize"
	"github.com/matthewmueller/chunky/repos"
)

type CacheSize struct {
	Repo *string
}

func (c *CacheSize) command(cli cli.Command) cli.Command {
	cmd := cli.Command("cache-size", "print the size of the cache").Advanced()
	cmd.Arg("repo", "repo path").Optional().String(&c.Repo)
	return cmd
}

func (c *CLI) CacheSize(ctx context.Context, in *CacheSize) error {
	cacheDir, err := cacheDir()
	if err != nil {
		return err
	}

	tw := tabwriter.NewWriter(c.Stdout, 0, 0, 1, ' ', 0)

	// If we have a repo, just compute the size of that repo's cache
	if in.Repo != nil {
		repoUrl, err := repos.Parse(*in.Repo)
		if err != nil {
			return err
		}
		cacheDir = filepath.Join(cacheDir, cacheName(repoUrl))
		dirSize, err := getDirSize(cacheDir)
		if err != nil {
			return err
		}
		fmt.Fprintf(tw, "%s\t%s\n", humanize.Bytes(uint64(dirSize)), cacheDir)
		return tw.Flush()
	}

	// Print the size of each repo's cache
	des, err := os.ReadDir(cacheDir)
	if err != nil {
		return err
	}
	for _, de := range des {
		if !de.IsDir() {
			continue
		}
		dirSize, err := getDirSize(filepath.Join(cacheDir, de.Name()))
		if err != nil {
			return err
		}
		fmt.Fprintf(tw, "%s\t%s\n", humanize.Bytes(uint64(dirSize)), de.Name())
	}
	return tw.Flush()
}

// Get the size of a directory
func getDirSize(dir string) (int, error) {
	var size int
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		size += int(info.Size())
		return nil
	})
	return size, err
}
