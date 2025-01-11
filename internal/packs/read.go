package packs

import (
	"context"
	"path"

	"github.com/matthewmueller/chunky/repos"
)

func Read(ctx context.Context, repo repos.Repo, packId string) (*Pack, error) {
	packFile, err := repos.Download(ctx, repo, path.Join("packs", packId))
	if err != nil {
		return nil, err
	}
	return Unpack(packFile.Data)
}

func NewReader(repo repos.Repo, maxBytes int64) *Reader {
	return &Reader{repo, newCache(maxBytes)}
}

type Reader struct {
	repo  repos.Repo
	cache *lruCache
}

func (r *Reader) Read(ctx context.Context, packId string) (*Pack, error) {
	if pack, ok := r.cache.Get(packId); ok {
		return pack, nil
	}
	pack, err := Read(ctx, r.repo, packId)
	if err != nil {
		return nil, err
	}
	r.cache.Set(packId, pack)
	return pack, nil
}
