package packs

import (
	"testing"

	"github.com/matryer/is"
)

func TestCacheAddGet(t *testing.T) {
	is := is.New(t)
	cache := newCache(1024)

	pack := New()
	pack.Add(&Chunk{Path: "test.txt", Data: []byte("test data")})

	cache.Set("test", pack)
	got, ok := cache.Get("test")
	is.True(ok)
	is.Equal(got, pack)
}

func TestCacheEviction(t *testing.T) {
	is := is.New(t)
	cache := newCache(64)

	pack1 := New()
	pack1.Add(&Chunk{Path: "test1.txt", Data: []byte("test data 1")})
	cache.Set("test1", pack1)

	pack2 := New()
	pack2.Add(&Chunk{Path: "test2.txt", Data: []byte("test data 2")})
	cache.Set("test2", pack2)

	_, ok := cache.Get("test1")
	is.True(!ok) // test1 should be evicted
	got, ok := cache.Get("test2")
	is.True(ok)
	is.Equal(got, pack2)
}

func TestCacheUpdate(t *testing.T) {
	is := is.New(t)
	cache := newCache(1024)

	pack1 := New()
	pack1.Add(&Chunk{Path: "test.txt", Data: []byte("test data 1")})
	cache.Set("test", pack1)

	pack2 := New()
	pack2.Add(&Chunk{Path: "test.txt", Data: []byte("test data 2")})
	cache.Set("test", pack2)

	got, ok := cache.Get("test")
	is.True(ok)
	is.Equal(got, pack2)
}

func TestCacheLen(t *testing.T) {
	is := is.New(t)
	cache := newCache(1024)

	is.Equal(cache.Len(), 0)

	pack := New()
	pack.Add(&Chunk{Path: "test.txt", Data: []byte("test data")})
	cache.Set("test", pack)

	is.Equal(cache.Len(), 1)
}
