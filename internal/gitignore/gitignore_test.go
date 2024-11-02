package gitignore_test

import (
	"testing"

	"github.com/matryer/is"
	"github.com/matthewmueller/chunky/internal/gitignore"
	"github.com/matthewmueller/virt"
)

func TestDirRoot(t *testing.T) {
	is := is.New(t)
	ignore := gitignore.FromFS(virt.Map{
		".gitignore": "/dir",
	})
	is.True(ignore("dir/internal/web/web.go"))
	is.True(!ignore("main.go"))
}

// .git ignored by default
func TestGitDir(t *testing.T) {
	is := is.New(t)
	ignore := gitignore.FromFS(virt.Map{
		".gitignore": "",
	})
	is.True(ignore(".git"))
	is.True(ignore(".git/objects"))
}

// node_modules ignored by default
func TestNodeModules(t *testing.T) {
	is := is.New(t)
	ignore := gitignore.FromFS(virt.Map{
		".gitignore": "",
	})
	is.True(ignore("node_modules"))
	is.True(ignore("node_modules/svelte/internal/compiler.js"))
}
