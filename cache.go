package main

import (
	"net/http"
	"encoding/binary"
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
	e.Body = buffer[n:n + int(bodylen)]
	n += int(bodylen)

	headercnt, c := binary.Varint(buffer[n:])
	n += c
	e.Header = make(http.Header)
	for i := 0; i < int(headercnt); i++ {
		keylen, c := binary.Varint(buffer[n:])
		n += c
		key := string(buffer[n:n + int(keylen)])
		n += int(keylen)

		valcnt, c := binary.Varint(buffer[n:])
		n += c
		for j := 0; j < int(valcnt); j++ {
			vallen, c := binary.Varint(buffer[n:])
			n += c
			e.Header[key] = append(e.Header[key], string(buffer[n:n + int(vallen)]))
			n += int(vallen)
		}
	}
}

type Cache interface {
	Get(url string) (CacheEntry, bool)
	Set(url string, entry CacheEntry)
}

type MemCache map[string]CacheEntry

func NewMemCache() MemCache {
	return make(map[string]CacheEntry)
}

func (c MemCache) Get(url string) (CacheEntry, bool) {
	t := time.Now().Unix()
	e, ok := c[url];
	if ok {
		if e.AddTime + e.Expire < t {
			return e, ok
		}
		delete(c, url)
	}
	return e, false
}

func (c MemCache) Set(url string, entry CacheEntry) {
	c[url] = entry
}

