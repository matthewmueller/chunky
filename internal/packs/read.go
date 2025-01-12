package packs

import (
	"context"
	"fmt"
	"log/slog"
	"path"
	"time"

	"github.com/matthewmueller/chunky/internal/lru"
	"github.com/matthewmueller/chunky/internal/rate"
	"github.com/matthewmueller/chunky/internal/singleflight"
	"github.com/matthewmueller/chunky/repos"
	"github.com/matthewmueller/logs"
)

type Reader interface {
	Read(ctx context.Context, repo repos.Repo, packId string) (*Pack, error)
}

func Read(ctx context.Context, repo repos.Repo, packId string) (*Pack, error) {
	packFile, err := repos.Download(ctx, repo, path.Join("packs", packId))
	if err != nil {
		return nil, err
	}
	return Unpack(packFile.Data)
}

// NewCachedReader creates a new cached reader
func NewCachedReader(log *slog.Logger, cache lru.Cache[*Pack]) *CachedReader {
	return &CachedReader{
		Limiter: rate.New(0),
		log:     log,
		cache:   cache,
	}
}

type CachedReader struct {
	Limiter rate.Limiter

	log   *slog.Logger
	cache lru.Cache[*Pack]
	group singleflight.Group[string, *Pack]
}

var _ Reader = (*CachedReader)(nil)

func (r *CachedReader) read(ctx context.Context, repo repos.Repo, packId string) (*Pack, error) {
	log := logs.Scope(r.log)
	now := time.Now()

	packFile, err := repos.Download(ctx, repo, path.Join("packs", packId))
	if err != nil {
		return nil, fmt.Errorf("packs: unable to download pack %s: %w", packId, err)
	}
	if err := r.Limiter.Use(ctx, len(packFile.Data)); err != nil {
		return nil, err
	}

	log.Debug("downloaded pack",
		slog.String("path", packFile.Path),
		slog.Int("size", len(packFile.Data)),
		slog.Duration("time", time.Since(now)),
	)

	return Unpack(packFile.Data)
}

func (r *CachedReader) Read(ctx context.Context, repo repos.Repo, packId string) (*Pack, error) {
	if pack, ok := r.cache.Get(packId); ok {
		return pack, nil
	}
	pack, err, _ := r.group.Do(packId, func() (*Pack, error) {
		return r.read(ctx, repo, packId)
	})
	if err != nil {
		return nil, fmt.Errorf("packs: unable to read pack %s: %w", packId, err)
	}
	r.cache.Set(packId, pack)
	return pack, nil
}
