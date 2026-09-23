package formats

import (
	"container/list"
	"io"
	"sync"
)

// Cache keeps the most recently used open archives (comics or EPUBs) so page
// turns don't re-read the archive index. Entries in use are never closed
// underneath a reader.
type Cache[T io.Closer] struct {
	mu    sync.Mutex
	max   int
	order *list.List // front = most recent; values are *cacheEntry[T]
	byKey map[int64]*list.Element
}

type cacheEntry[T io.Closer] struct {
	key     int64
	c       T
	refs    int
	evicted bool
}

func NewCache[T io.Closer](max int) *Cache[T] {
	return &Cache[T]{max: max, order: list.New(), byKey: map[int64]*list.Element{}}
}

// Get returns the container for key, opening it on a miss. Call release when done.
func (c *Cache[T]) Get(key int64, open func() (T, error)) (T, func(), error) {
	c.mu.Lock()
	if el, ok := c.byKey[key]; ok {
		e := el.Value.(*cacheEntry[T])
		e.refs++
		c.order.MoveToFront(el)
		c.mu.Unlock()
		return e.c, c.releaser(e), nil
	}
	c.mu.Unlock()

	// Open outside the lock; a racing open of the same key just loses and closes.
	ct, err := open()
	if err != nil {
		var zero T
		return zero, nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.byKey[key]; ok {
		ct.Close()
		e := el.Value.(*cacheEntry[T])
		e.refs++
		c.order.MoveToFront(el)
		return e.c, c.releaser(e), nil
	}
	e := &cacheEntry[T]{key: key, c: ct, refs: 1}
	c.byKey[key] = c.order.PushFront(e)
	for c.order.Len() > c.max {
		c.evict(c.order.Back())
	}
	return ct, c.releaser(e), nil
}

// Forget drops key, e.g. after a rescan changed the file.
func (c *Cache[T]) Forget(key int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.byKey[key]; ok {
		c.evict(el)
	}
}

func (c *Cache[T]) evict(el *list.Element) {
	e := el.Value.(*cacheEntry[T])
	c.order.Remove(el)
	delete(c.byKey, e.key)
	e.evicted = true
	if e.refs == 0 {
		e.c.Close()
	}
}

func (c *Cache[T]) releaser(e *cacheEntry[T]) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			c.mu.Lock()
			defer c.mu.Unlock()
			if e.refs--; e.refs == 0 && e.evicted {
				e.c.Close()
			}
		})
	}
}
