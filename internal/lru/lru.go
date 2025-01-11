package lru

import (
	"container/list"
	"sync"

	"github.com/matthewmueller/chunky/internal/packs"
)

type Cache struct {
	maxBytes  int64
	usedBytes int64
	ll        *list.List
	cache     map[string]*list.Element
	mu        sync.Mutex
}

type entry struct {
	key   string
	value *packs.Pack
}

func New(maxBytes int64) *Cache {
	return &Cache{
		maxBytes: maxBytes,
		ll:       list.New(),
		cache:    make(map[string]*list.Element),
	}
}

func (c *Cache) Get(key string) (*packs.Pack, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ele, ok := c.cache[key]; ok {
		c.ll.MoveToFront(ele)
		kv := ele.Value.(*entry)
		return kv.value, true
	}
	return nil, false
}

func (c *Cache) Add(key string, value *packs.Pack) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ele, ok := c.cache[key]; ok {
		c.ll.MoveToFront(ele)
		kv := ele.Value.(*entry)
		c.usedBytes += value.Length() - kv.value.Length()
		kv.value = value
	} else {
		ele := c.ll.PushFront(&entry{key, value})
		c.cache[key] = ele
		c.usedBytes += int64(len(key)) + value.Length()
	}

	for c.maxBytes != 0 && c.maxBytes < c.usedBytes {
		c.removeOldest()
	}
}

func (c *Cache) removeOldest() {
	ele := c.ll.Back()
	if ele != nil {
		c.ll.Remove(ele)
		kv := ele.Value.(*entry)
		delete(c.cache, kv.key)
		c.usedBytes -= int64(len(kv.key)) + kv.value.Length()
	}
}

func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len()
}
