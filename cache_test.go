package main

import (
	"testing"
	"encoding/gob"
	"bytes"
	"reflect"
)

var entry = CacheEntry{
	Status: 200,
	AddTime: 100000010,
	Expire: 100101,
	Header:  map[string][]string{
		"X-header": []string{"abc", "def"},
	},
	Body: []byte("hello world"),
}

func TestEncoding(t *testing.T) {
	bytes := entry.Encode()
	e := CacheEntry{}
	e.Decode(bytes)
	if !reflect.DeepEqual(e, entry) {
		t.Fail()
	}
}

func TestMemTable(t *testing.T) {
	cache := NewMemCache()
	url := "http://test.com"
	cache.Set(url, entry)

	if r, ok := cache.Get(url); ok && reflect.DeepEqual(r, entry) {

	} else {
		t.Fail()
	}
}

func BenchmarkEncoding(b *testing.B) {
	for i := 0; i < b.N; i++ {
		bytes := entry.Encode()
		b.SetBytes(int64(len(bytes)))
	}
}

func BenchmarkDecoding(b *testing.B) {
	bytes := entry.Encode()
	for i := 0; i < b.N; i++ {
		e := CacheEntry{}
		e.Decode(bytes)
		b.SetBytes(int64(len(bytes)))
	}
}


func BenchmarkGobDecoding(b *testing.B) {
	var buffer bytes.Buffer
	enc := gob.NewEncoder(&buffer)
	enc.Encode(entry)
	data := buffer.Bytes()

	for i := 0 ; i < b.N; i++ {
		buffer := bytes.NewReader(data)
		dec := gob.NewDecoder(buffer)
		e := CacheEntry{}
		dec.Decode(&e)
		b.SetBytes(int64(len(data)))
	}
}


func BenchmarkGobEncoding(b *testing.B) {
	for i := 0 ; i < b.N; i++ {
		var buffer bytes.Buffer
		enc := gob.NewEncoder(&buffer)
		enc.Encode(entry)
		b.SetBytes(int64(buffer.Len()))
	}
}
