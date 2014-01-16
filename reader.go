package main

import (
	"io"
	"strings"
)

type PushBackReader struct {
	reader io.Reader
	buffer []byte
	idx    int
	limit  int
}

func NewPushBackReader(reader io.Reader, buffer int) *PushBackReader {
	return &PushBackReader{
		reader: reader,
		buffer: make([]byte,0, buffer),
		idx:    0,
		limit:  0,
	}
}

func (r *PushBackReader) Read(p []byte) (n int, err error) {
	if r.limit > r.idx {
		n = copy(p, r.buffer[r.idx:r.limit])
		r.idx += n
		if r.limit == r.idx {
			r.limit = 0
			r.idx = 0
		}
		return
	}

	return r.reader.Read(p)
}

func (r *PushBackReader) UnRead(p []byte) {
	if r.idx > len(p) {
		copy(r.buffer[r.idx - len(p):r.idx], p)
		r.idx -= len(p)
	} else {
		r.buffer = append(r.buffer[:r.limit], p...)
		r.limit += len(p)
	}
}


func (r *PushBackReader) IsHttpGet(addr string) bool {
	if strings.HasSuffix(addr, "80") { // only for HTTP port
		buf := [3]byte{}
		if n, err := r.Read(buf[:]); err == nil {
			r.UnRead(buf[:n])
			if "GET" == string(buf[:n]) {
				return true
			}
		}
	}
	return false
}
