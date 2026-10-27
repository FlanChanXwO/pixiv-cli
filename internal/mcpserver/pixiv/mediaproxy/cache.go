package mediaproxy

import (
	"container/list"
	"sync"
	"time"
)

type cacheEntry struct {
	refs    int
	key     string
	chunks  [][]byte
	size    int64
	expires time.Time
	timer   *time.Timer
}
type contentCache struct {
	mu             sync.Mutex
	capacity, used int64
	entries        map[string]*list.Element
	lru            *list.List
}

func newContentCache(capacity int64) *contentCache {
	return &contentCache{capacity: capacity, entries: map[string]*list.Element{}, lru: list.New()}
}
func (c *contentCache) remove(e *list.Element) {
	v := e.Value.(*cacheEntry)
	if v.timer != nil {
		v.timer.Stop()
	}
	c.used -= v.size
	delete(c.entries, v.key)
	c.lru.Remove(e)
}

// invalidate 让已被 pin 的字节留到读者释放；判断函数与发布操作在同一把锁下运行。
func (c *contentCache) invalidate(reject func(string) bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, entry := range c.entries {
		value := entry.Value.(*cacheEntry)
		if reject(value.key) {
			value.expires = time.Time{}
			if value.refs == 0 {
				c.remove(entry)
			}
		}
	}
}
func (c *contentCache) reserve(size int64) bool {
	if size > c.capacity || c.capacity == 0 {
		return false
	}
	for e := c.lru.Back(); c.used > c.capacity-size && e != nil; {
		prev := e.Prev()
		if e.Value.(*cacheEntry).refs == 0 {
			c.remove(e)
		}
		e = prev
	}
	if c.used > c.capacity-size {
		return false
	}
	c.used += size
	return true
}
func (c *contentCache) putValidated(key string, data []byte, expires time.Time, validate func() bool) {
	fill := c.begin()
	defer fill.abort()
	if fill.append(data) {
		fill.commitValidated(key, expires, validate)
	}
}

// cacheFill 表示正在填充的缓冲：它和已完成条目共用字节预算，也不会被淘汰。
type cacheFill struct {
	cache  *contentCache
	chunks [][]byte
	size   int64
	done   bool
}

func (c *contentCache) begin() *cacheFill { return &cacheFill{cache: c} }
func (f *cacheFill) append(data []byte) bool {
	if f.done {
		return false
	}
	c := f.cache
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.reserve(int64(len(data))) {
		c.used -= f.size
		f.chunks = nil
		f.size = 0
		f.done = true
		return false
	}
	f.chunks = append(f.chunks, append([]byte(nil), data...))
	f.size += int64(len(data))
	return true
}
func (f *cacheFill) abort() {
	if f.done {
		return
	}
	f.cache.mu.Lock()
	defer f.cache.mu.Unlock()
	f.cache.used -= f.size
	f.done = true
	f.chunks = nil
}

// commitValidated 在缓存锁内重新运行 validate；失效与发布共用这把锁，迟到的填充无法
// 在失效通知处理完之后复活条目。
func (f *cacheFill) commitValidated(key string, expires time.Time, validate func() bool) {
	if f.done {
		return
	}
	c := f.cache
	c.mu.Lock()
	defer c.mu.Unlock()
	f.done = true
	if !time.Now().Before(expires) || (validate != nil && !validate()) {
		c.used -= f.size
		f.chunks = nil
		return
	}
	if old := c.entries[key]; old != nil {
		if old.Value.(*cacheEntry).refs > 0 {
			c.used -= f.size
			f.chunks = nil
			return
		}
		c.remove(old)
	}
	value := &cacheEntry{key: key, chunks: f.chunks, size: f.size, expires: expires}
	entry := c.lru.PushFront(value)
	c.entries[key] = entry
	value.timer = time.AfterFunc(time.Until(expires), func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.entries[key] == entry && value.refs == 0 {
			c.remove(entry)
		}
	})
}

// load pin 住命中的载荷直到调用方解码或传输完；返回的释放函数只能调用一次。
func (c *contentCache) load(key string) ([][]byte, func(), bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.entries[key]
	if e == nil {
		return nil, func() {}, false
	}
	v := e.Value.(*cacheEntry)
	if !time.Now().Before(v.expires) {
		if v.refs == 0 {
			c.remove(e)
		}
		return nil, func() {}, false
	}
	v.refs++
	c.lru.MoveToFront(e)
	var once sync.Once
	return v.chunks, func() {
		once.Do(func() {
			c.mu.Lock()
			defer c.mu.Unlock()
			v.refs--
			if v.refs == 0 && !time.Now().Before(v.expires) {
				c.remove(e)
			}
		})
	}, true
}
