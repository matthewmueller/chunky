package lru

import (
	"container/list"
	"log/slog"
	"sync"

	"github.com/matthewmueller/logs"
)

type Cache[I Item] interface {
	Get(key string) (I, bool)
	Set(key string, value I)
}

type Item interface {
	Length() int
}

type cache[I Item] struct {
	log       *slog.Logger
	maxBytes  int
	usedBytes int
	ll        *list.List
	cache     map[string]*list.Element
	mu        sync.Mutex
}

type entry[I Item] struct {
	key   string
	value I
}

func New[I Item](log *slog.Logger, maxBytes int) *cache[I] {
	return &cache[I]{
		log:      log,
		maxBytes: maxBytes,
		ll:       list.New(),
		cache:    make(map[string]*list.Element),
	}
}

func (c *cache[I]) Get(key string) (I, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ele, ok := c.cache[key]; ok {
		c.ll.MoveToFront(ele)
		kv := ele.Value.(*entry[I])
		return kv.value, true
	}
	var zero I
	return zero, false
}

func (c *cache[I]) Set(key string, value I) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ele, ok := c.cache[key]; ok {
		c.ll.MoveToFront(ele)
		kv := ele.Value.(*entry[I])
		c.usedBytes += value.Length() - kv.value.Length()
		kv.value = value
	} else {
		ele := c.ll.PushFront(&entry[I]{key, value})
		c.cache[key] = ele
		c.usedBytes += len(key) + value.Length()
	}

	for c.maxBytes != 0 && c.maxBytes < c.usedBytes {
		c.removeOldest()
	}
}

func (c *cache[I]) removeOldest() {
	log := logs.Scope(c.log)
	ele := c.ll.Back()
	if ele != nil {
		c.ll.Remove(ele)
		kv := ele.Value.(*entry[I])
		delete(c.cache, kv.key)
		log.Debug("lru dropped item", slog.String("key", kv.key))
		c.usedBytes -= len(kv.key) + kv.value.Length()
	}
}

func (c *cache[I]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len()
}
