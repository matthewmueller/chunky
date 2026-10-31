package chunkyignore

import (
	"io/fs"
	"strings"

	gitignore "github.com/sabhiram/go-gitignore"
)

var defaultIgnores = []string{
	".git",
	".DS_Store",
}

var defaultIgnore = gitignore.CompileIgnoreLines(defaultIgnores...).MatchesPath

func FromFS(fsys fs.FS) (ignore func(path string) bool) {
	code, err := fs.ReadFile(fsys, ".chunkyignore")
	if err != nil {
		return defaultIgnore
	}
	lines := strings.Split(string(code), "\n")
	ignorer := gitignore.CompileIgnoreLines(lines...)
	return ignorer.MatchesPath
}
