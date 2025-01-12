package chunky

import (
	"context"
	"errors"

	"github.com/dustin/go-humanize"
	"github.com/matthewmueller/chunky/internal/downloads"
	"github.com/matthewmueller/chunky/internal/lru"
	"github.com/matthewmueller/chunky/internal/packs"
	"github.com/matthewmueller/chunky/internal/rate"
	"github.com/matthewmueller/chunky/repos"
)

type Download struct {
	From     repos.Repo
	To       repos.FS
	Revision string

	MaxCacheSize string
	maxCacheSize int

	LimitDownload string
	limitDownload int
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

	if d.MaxCacheSize != "" {
		maxCacheSize, err2 := humanize.ParseBytes(d.MaxCacheSize)
		if err2 != nil {
			err = errors.Join(err, errors.New("invalid max cache size"))
		} else {
			d.maxCacheSize = int(maxCacheSize)
		}
	} else {
		d.maxCacheSize = 0
	}

	if d.LimitDownload != "" {
		limitDownload, err2 := humanize.ParseBytes(d.LimitDownload)
		if err2 != nil {
			err = errors.Join(err, errors.New("invalid limit download"))
		} else {
			d.limitDownload = int(limitDownload)
		}
	} else {
		d.limitDownload = 0
	}

	return err
}

// Download a directory from a repository at a specific revision
func (c *Client) Download(ctx context.Context, in *Download) error {
	if err := in.validate(); err != nil {
		return err
	}

	pr := packs.NewCachedReader(lru.New[*packs.Pack](in.maxCacheSize))
	if in.limitDownload > 0 {
		pr.Limiter = rate.New(in.limitDownload)
	}
	downloader := downloads.New(pr)

	// Download the repo
	return downloader.Download(ctx, in.From, in.Revision, in.To)
}
