package main

import (
	"testing"
	"strings"
)

func TestPushBackReader(t *testing.T) {
	s := "hello world"
	r := strings.NewReader(s)
	pbr := NewPushBackReader(r, 10)
	buf := [20]byte{}

	if n, err := pbr.Read(buf[:]); err != nil || n <= 0 || s != string(buf[:n]) {
		t.Fail()
	}

	pbr.UnRead([]byte("hello"))

	if n, err := pbr.Read(buf[:]); err != nil || n <= 0 || "hello" != string(buf[:n]) {
		t.Error(string(buf[:n]), n)
	}

	pbr.UnRead([]byte("www"))
	if n, err := pbr.Read(buf[:]); err != nil || n <= 0 || "www" != string(buf[:n]) {
		t.Error(string(buf[:n]), n)
	}
}
