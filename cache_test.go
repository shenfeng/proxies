package main

import (
	"bytes"
	"code.google.com/p/leveldb-go/leveldb"
	"code.google.com/p/leveldb-go/leveldb/db"
	"encoding/gob"
	"github.com/jmhodges/levigo"
	"log"
	"os"
	"reflect"
	"strconv"
	"testing"
)

// CGO_CFLAGS="-I/path/to/leveldb/include" CGO_LDFLAGS="-L/path/to/leveldb/lib" go get github.com/jmhodges/levigo

var entry = CacheEntry{
	Status:  200,
	AddTime: 100000010,
	Expire:  100101,
	Header: map[string][]string{
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

func TestMemCache(t *testing.T) {
	cache := NewMemCache()
	url := "http://test.com"
	cache.Set(url, entry)

	if r, ok := cache.Get(url); ok && reflect.DeepEqual(r, entry) {

	} else {
		t.Fail()
	}
}

func TestLeveldbCache(t *testing.T) {
	path := "/tmp/leveldb_cache__"
	os.RemoveAll(path)
	cache, err := NewLeveldbCache(path, 10)
	if err != nil {
		t.Fail()
	}
	url := "http://test.com"
	cache.Set(url, entry)

	if r, ok := cache.Get(url); ok && reflect.DeepEqual(r, entry) {

	} else {
		t.Fail()
	}

	cache.Delete(url)
	if _, ok := cache.Get(url); ok {
		t.Fail()
	}
	cache.Close()
}

func BenchmarkLeveldbCache(b *testing.B) {
	path := "/tmp/leveldb_cache__"
	os.RemoveAll(path)
	cache, err := NewLeveldbCache(path, 10)
	if err != nil {
		b.Fail()
	}

	for i := 0; i < b.N; i++ {
		url := "http://test.com" + strconv.Itoa(i)
		cache.Set(url, entry)
	}
	cache.Close()
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

	for i := 0; i < b.N; i++ {
		buffer := bytes.NewReader(data)
		dec := gob.NewDecoder(buffer)
		e := CacheEntry{}
		dec.Decode(&e)
		b.SetBytes(int64(len(data)))
	}
}

func TestLevigo(t *testing.T) {
	path := "/tmp/levigo_test_10101"
	os.RemoveAll(path)

	opts := levigo.NewOptions()
	filter := levigo.NewBloomFilter(10)
	opts.SetFilterPolicy(filter)
	opts.SetCache(levigo.NewLRUCache(1024 << 20)) // 1G
	opts.SetCreateIfMissing(true)
	if ldb, err := levigo.Open(path, opts); err == nil {
		key := []byte("test-test hwl0dsfds")
		val := []byte("value")

		if err = ldb.Put(levigo.NewWriteOptions(), key, val); err != nil {
			t.Fail()
		} else {
			ro := levigo.NewReadOptions()
			if data, err := ldb.Get(ro, key); err == nil && reflect.DeepEqual(data, val) {
				ro.SetFillCache(false)
				it := ldb.NewIterator(ro)
				it.Seek([]byte{0})
				for ; it.Valid(); it.Next() {
					log.Printf("%s => %s", it.Key(), it.Value())
				}
			} else {
				t.Fail()
			}
		}
	} else {
		t.Fail()
	}
}

func TestLevelDB(t *testing.T) {
	path := "/tmp/leveldb_test_10101"
	os.RemoveAll(path)

	options := db.Options{}
	if ldb, err := leveldb.Open(path, &options); err == nil {
		key := []byte("test-test hwl0dsfds")
		val := []byte("value")
		ldb.Set(key, val, &db.WriteOptions{})

		if r, err := ldb.Get(key, &db.ReadOptions{}); err == nil && reflect.DeepEqual(r, val) {
			// it := ldb.Find(nil, nil)
			// for it.Next() {
			// 	log.Printf("%s => %s", it.Key(), it.Value())
			// }

		} else {
			t.Fail()
		}
	} else {
		t.Fail()
	}

}

func BenchmarkGobEncoding(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var buffer bytes.Buffer
		enc := gob.NewEncoder(&buffer)
		enc.Encode(entry)
		b.SetBytes(int64(buffer.Len()))
	}
}
