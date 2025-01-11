package chunky

import (
	"context"
	"errors"

	"github.com/matthewmueller/chunky/internal/downloads"
	"github.com/matthewmueller/chunky/internal/lru"
	"github.com/matthewmueller/chunky/internal/packs"
	"github.com/matthewmueller/chunky/repos"
)

type Download struct {
	From         repos.Repo
	To           repos.FS
	Revision     string
	MaxCacheSize int64
}

func (d *Download) validate() (err error) {
	// Required fields
	if d.From == nil {
		err = errors.Join(err, errors.New("missing 'from' repository"))
	}
	if d.To == nil {
		err = errors.Join(err, errors.New("missing 'to' writable filesystem"))
	}
	if d.Revision == "" {
		err = errors.Join(err, errors.New("missing 'revision'"))
	}
	if d.MaxCacheSize < 0 {
		err = errors.Join(err, errors.New("invalid max cache size"))
	} else if d.MaxCacheSize == 0 {
		d.MaxCacheSize = 512 * miB
	}

	return err
}

// Download a directory from a repository at a specific revision
func (c *Client) Download(ctx context.Context, in *Download) error {
	if err := in.validate(); err != nil {
		return err
	}

	pr := packs.NewReader(lru.New[*packs.Pack](in.MaxCacheSize))
	downloader := downloads.New(pr)

	// Download the repo
	return downloader.Download(ctx, in.From, in.Revision, in.To)
}
