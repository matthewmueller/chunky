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

// DefaultMaxCacheSize is the default maximum size of the LRU for caching packs
const DefaultMaxCacheSize = 512 * miB // 512 MiB

type Download struct {
	From     repos.Repo
	To       repos.FS
	Revision string

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

func (in *Download) validate() (err error) {
	// Required fields
	if in.From == nil {
		err = errors.Join(err, errors.New("missing 'from' repository"))
	}
	if in.To == nil {
		err = errors.Join(err, errors.New("missing 'to' writable filesystem"))
	}
	if in.Revision == "" {
		err = errors.Join(err, errors.New("missing 'revision'"))
	}

	if in.MaxCacheSize != "" {
		maxCacheSize, err2 := humanize.ParseBytes(in.MaxCacheSize)
		if err2 != nil {
			err = errors.Join(err, errors.New("invalid max cache size"))
		} else {
			in.maxCacheSize = int(maxCacheSize)
		}
	} else {
		in.maxCacheSize = DefaultMaxCacheSize
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
		in.concurrency = DefaultConcurrency
	}

	return err
}

// Download a directory from a repository at a specific revision
func (c *Client) Download(ctx context.Context, in *Download) error {
	if err := in.validate(); err != nil {
		return err
	}

	pr := packs.NewCachedReader(c.log, lru.New[*packs.Pack](c.log, in.maxCacheSize))
	if in.limitDownload > 0 {
		pr.Limiter = rate.New(in.limitDownload)
	}

	downloader := downloads.New(pr)
	if in.concurrency > 0 {
		downloader.Concurrency = in.concurrency
	}

	// Download the repo
	return downloader.Download(ctx, in.From, in.Revision, in.To)
}
