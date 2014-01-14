package main

import (
	"encoding/binary"
	"github.com/jmhodges/levigo"
	"net/http"
	"time"
)

type CacheEntry struct {
	Status  int64
	AddTime int64
	Expire  int64
	Body    []byte
	Header  http.Header
}

func (e *CacheEntry) Encode() []byte {
	hl := 0
	for key, vals := range e.Header {
		hl = hl + binary.MaxVarintLen32*2 + len(key)
		for _, v := range vals {
			hl += binary.MaxVarintLen32 + len(v)
		}
	}

	size := 5*binary.MaxVarintLen32 + len(e.Body) + hl
	buffer := make([]byte, size)

	n := 0

	n += binary.PutVarint(buffer[n:], int64(e.Status))
	n += binary.PutVarint(buffer[n:], int64(e.AddTime))
	n += binary.PutVarint(buffer[n:], int64(e.Expire))
	n += binary.PutVarint(buffer[n:], int64(len(e.Body)))
	n += copy(buffer[n:], e.Body)
	n += binary.PutVarint(buffer[n:], int64(len(e.Header)))

	for key, vals := range e.Header {
		n += binary.PutVarint(buffer[n:], int64(len(key)))
		n += copy(buffer[n:], key)
		n += binary.PutVarint(buffer[n:], int64(len(vals)))
		for _, v := range vals {
			n += binary.PutVarint(buffer[n:], int64(len(v)))
			n += copy(buffer[n:], v)
		}
	}

	return buffer[:n]
}

func (e *CacheEntry) Decode(buffer []byte) {
	n := 0
	status, c := binary.Varint(buffer[n:])
	n += c
	e.Status = status

	addtime, c := binary.Varint(buffer[n:])
	n += c
	e.AddTime = addtime

	expire, c := binary.Varint(buffer[n:])
	n += c
	e.Expire = expire

	bodylen, c := binary.Varint(buffer[n:])
	n += c
	e.Body = buffer[n : n+int(bodylen)]
	n += int(bodylen)

	headercnt, c := binary.Varint(buffer[n:])
	n += c
	e.Header = make(http.Header)
	for i := 0; i < int(headercnt); i++ {
		keylen, c := binary.Varint(buffer[n:])
		n += c
		key := string(buffer[n : n+int(keylen)])
		n += int(keylen)

		valcnt, c := binary.Varint(buffer[n:])
		n += c
		for j := 0; j < int(valcnt); j++ {
			vallen, c := binary.Varint(buffer[n:])
			n += c
			e.Header[key] = append(e.Header[key], string(buffer[n:n+int(vallen)]))
			n += int(vallen)
		}
	}
}

type Cache interface {
	Get(key string) (CacheEntry, bool)
	Set(key string, val CacheEntry) error
	Delete(key string) error
	Close() error
}

// cache backend by RAM
type MemCache map[string]CacheEntry

func NewMemCache() MemCache { return make(map[string]CacheEntry) }

func (c MemCache) Close() error                     { return nil }
func (c MemCache) Delete(key string)                { delete(c, key) }
func (c MemCache) Set(key string, value CacheEntry) { c[key] = value }
func (c MemCache) Get(key string) (CacheEntry, bool) {
	t := time.Now().Unix()
	e, ok := c[key]
	if ok {
		if e.AddTime+e.Expire < t {
			return e, ok
		}
		delete(c, key)
	}
	return e, false
}

// cache backend by leveldb(github.com/jmhodges/levigo)
type LeveldbCache struct {
	db    *levigo.DB
	fp    *levigo.FilterPolicy
	cache *levigo.Cache

	Wo *levigo.WriteOptions
	Ro *levigo.ReadOptions
	So *levigo.ReadOptions
}

func NewLeveldbCache(dbname string, cacheM int) (*LeveldbCache, error) {
	opts := levigo.NewOptions()
	filter := levigo.NewBloomFilter(10)
	cache := levigo.NewLRUCache(1024 * 1024 * cacheM)
	opts.SetFilterPolicy(filter)
	opts.SetCache(cache)
	opts.SetCreateIfMissing(true)
	opts.SetWriteBufferSize(8 * 1024 * 104) // 8M
	opts.SetCompression(levigo.SnappyCompression)

	if ldb, err := levigo.Open(dbname, opts); err == nil {
		so := levigo.NewReadOptions()
		so.SetFillCache(false)
		return &LeveldbCache{
			db:    ldb,
			fp:    filter,
			cache: cache,
			Ro:    levigo.NewReadOptions(),
			Wo:    levigo.NewWriteOptions(),
			So:    so,
		}, nil
	} else {
		return nil, err
	}
}

func (c *LeveldbCache) Close() error {
	c.Wo.Close()
	c.Ro.Close()
	c.So.Close()
	c.db.Close()
	c.fp.Close()
	c.cache.Close()
	return nil
}

func (c *LeveldbCache) Set(key string, val CacheEntry) error {
	return c.db.Put(c.Wo, []byte(key), val.Encode())
}

func (c *LeveldbCache) Get(key string) (CacheEntry, bool) {
	e := CacheEntry{}
	if d, err := c.db.Get(c.Ro, []byte(key)); err == nil && len(d) > 0 {
		e.Decode(d)
		return e, true
	}
	return e, false
}

func (c *LeveldbCache) Delete(key string) error {
	return c.db.Delete(c.Wo, []byte(key))
}
