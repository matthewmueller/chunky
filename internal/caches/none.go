package caches

import "github.com/matthewmueller/chunky/internal/commits"

var None = none{}

type none struct{}

var _ Cache = (*none)(nil)

func (none) Get(fileId string) (*commits.File, bool) {
	return nil, false
}

func (none) Set(commitId string, commit *commits.Commit) error {
	return nil
}
