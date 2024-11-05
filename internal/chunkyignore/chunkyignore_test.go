package chunkyignore_test

import (
	"testing"

	"github.com/matryer/is"
	"github.com/matthewmueller/chunky/internal/chunkyignore"
	"github.com/matthewmueller/virt"
)

func TestDirRoot(t *testing.T) {
	is := is.New(t)
	ignore := chunkyignore.FromFS(virt.Map{
		".chunkyignore": "/dir",
	})
	is.True(ignore("dir/internal/web/web.go"))
	is.True(!ignore("main.go"))
}

// .git ignored by default
func TestGitDir(t *testing.T) {
	is := is.New(t)
	ignore := chunkyignore.FromFS(virt.Map{})
	is.True(ignore(".git"))
	is.True(ignore(".git/objects"))
}

// ignore node_modules
func TestNodeModules(t *testing.T) {
	is := is.New(t)
	ignore := chunkyignore.FromFS(virt.Map{
		".chunkyignore": "node_modules",
	})
	is.True(ignore("node_modules"))
	is.True(ignore("node_modules/svelte/internal/compiler.js"))
}
