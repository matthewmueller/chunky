package caches

import (
	"github.com/matthewmueller/chunky/internal/commits"
)

type Cache interface {
	Get(path, hash string) (file *commits.File, ok bool)
	Set(commitId string, commit *commits.Commit) error
}
