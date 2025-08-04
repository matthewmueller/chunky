package chunky

import (
	"context"
	"errors"
	"io"

	"github.com/dustin/go-humanize"
	"github.com/matthewmueller/chunky/internal/downloads"
	"github.com/matthewmueller/chunky/internal/lru"
	"github.com/matthewmueller/chunky/internal/packs"
	"github.com/matthewmueller/chunky/internal/rate"
	"github.com/matthewmueller/chunky/repos"
)

type Cat struct {
	From     repos.Repo
	To       io.Writer
	Revision string
	Path     string

	// MaxCacheSize is the maximum size of the LRU for caching packs (default: 512MiB)
	MaxCacheSize string
	maxCacheSize int

	// LimitDownload is the maximum download speed per second (default: unlimited)
	LimitDownload string
	limitDownload int

	// Concurrency is the number of concurrent downloads (default: num cpus * 2)
	Concurrency *int
	concurrency int
}

func (in *Cat) validate() (err error) {
	// Required fields
	if in.From == nil {
		err = errors.Join(err, errors.New("missing 'from' repository"))
	}
	if in.To == nil {
		err = errors.Join(err, errors.New("missing 'to'"))
	}
	if in.Revision == "" {
		err = errors.Join(err, errors.New("missing 'revision'"))
	}
	if in.Path == "" {
		err = errors.Join(err, errors.New("missing 'path'"))
	}

	if in.MaxCacheSize != "" {
		maxCacheSize, err2 := humanize.ParseBytes(in.MaxCacheSize)
		if err2 != nil {
			err = errors.Join(err, errors.New("invalid max cache size"))
		} else {
			in.maxCacheSize = int(maxCacheSize)
		}
	} else {
		in.maxCacheSize = 512 * miB
	}

	if in.LimitDownload != "" {
		limitDownload, err2 := humanize.ParseBytes(in.LimitDownload)
		if err2 != nil {
			err = errors.Join(err, errors.New("invalid limit download"))
		} else {
			in.limitDownload = int(limitDownload)
		}
	} else {
		in.limitDownload = 0
	}

	// Set the concurrency if provided
	if in.Concurrency != nil {
		in.concurrency = *in.Concurrency
		// Disallow "unlimited" concurrency for now
		if in.concurrency <= 0 {
			err = errors.Join(err, errors.New("invalid concurrency"))
		}
	} else {
		in.concurrency = defaultConcurrency
	}

	return err
}

func (c *Client) Cat(ctx context.Context, in *Cat) error {
	if err := in.validate(); err != nil {
		return err
	}

	// Create a cached pack reader with the specified max cache size
	pr := packs.NewCachedReader(c.log, lru.New[*packs.Pack](c.log, in.maxCacheSize))

	// Set the download limit if provided
	if in.LimitDownload != "" {
		pr.Limiter = rate.New(in.limitDownload)
	}

	download := downloads.New(pr)

	// Set the concurrency if provided
	if in.Concurrency != nil {
		download.Concurrency = *in.Concurrency
	}

	return download.Cat(ctx, in.To, in.From, in.Revision, in.Path)
}
