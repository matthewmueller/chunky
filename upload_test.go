package chunky_test

import (
	"context"
	"testing"

	"github.com/matryer/is"
	"github.com/matthewmueller/chunky"
	"github.com/matthewmueller/chunky/repos/local"
	"github.com/matthewmueller/logs"
	"github.com/matthewmueller/virt"
)

func TestUpload(t *testing.T) {
	is := is.New(t)
	log := logs.Default()
	ctx := context.Background()
	c := chunky.New(log)
	cache := virt.Tree{}
	from := virt.Tree{
		"a.js": &virt.File{Path: "a.js", Data: []byte("aaa")},
		"b.js": &virt.File{Path: "a.js", Data: []byte("bbb")},
	}
	to := virt.Tree{}
	repo := local.New(to)
	err := c.Upload(ctx, &chunky.Upload{
		Cache: cache,
		From:  from,
		To:    repo,
	})
	is.NoErr(err)
}
