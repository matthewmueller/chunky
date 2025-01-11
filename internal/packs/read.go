package packs

import (
	"context"
	"path"

	"github.com/matthewmueller/chunky/internal/lru"
	"github.com/matthewmueller/chunky/repos"
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

// NewReader creates a new cached reader
func NewReader(cache lru.Cache[*Pack]) Reader {
	return &cachedReader{cache}
}

type cachedReader struct {
	cache lru.Cache[*Pack]
}

func (r *cachedReader) Read(ctx context.Context, repo repos.Repo, packId string) (*Pack, error) {
	if pack, ok := r.cache.Get(packId); ok {
		return pack, nil
	}
	pack, err := Read(ctx, repo, packId)
	if err != nil {
		return nil, err
	}
	r.cache.Set(packId, pack)
	return pack, nil
}
