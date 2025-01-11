package lru_test

import (
	"testing"

	"github.com/matryer/is"
	"github.com/matthewmueller/chunky/internal/lru"
	"github.com/matthewmueller/chunky/internal/packs"
)

func TestCacheAddGet(t *testing.T) {
	is := is.New(t)
	cache := lru.New(1024)

	pack := packs.New()
	pack.Add(&packs.Chunk{Path: "test.txt", Data: []byte("test data")})

	cache.Add("test", pack)
	got, ok := cache.Get("test")
	is.True(ok)
	is.Equal(got, pack)
}

func TestCacheEviction(t *testing.T) {
	is := is.New(t)
	cache := lru.New(64)

	pack1 := packs.New()
	pack1.Add(&packs.Chunk{Path: "test1.txt", Data: []byte("test data 1")})
	cache.Add("test1", pack1)

	pack2 := packs.New()
	pack2.Add(&packs.Chunk{Path: "test2.txt", Data: []byte("test data 2")})
	cache.Add("test2", pack2)

	_, ok := cache.Get("test1")
	is.True(!ok) // test1 should be evicted
	got, ok := cache.Get("test2")
	is.True(ok)
	is.Equal(got, pack2)
}

func TestCacheUpdate(t *testing.T) {
	is := is.New(t)
	cache := lru.New(1024)

	pack1 := packs.New()
	pack1.Add(&packs.Chunk{Path: "test.txt", Data: []byte("test data 1")})
	cache.Add("test", pack1)

	pack2 := packs.New()
	pack2.Add(&packs.Chunk{Path: "test.txt", Data: []byte("test data 2")})
	cache.Add("test", pack2)

	got, ok := cache.Get("test")
	is.True(ok)
	is.Equal(got, pack2)
}

func TestCacheLen(t *testing.T) {
	is := is.New(t)
	cache := lru.New(1024)

	is.Equal(cache.Len(), 0)

	pack := packs.New()
	pack.Add(&packs.Chunk{Path: "test.txt", Data: []byte("test data")})
	cache.Add("test", pack)

	is.Equal(cache.Len(), 1)
}
