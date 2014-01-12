package main

import (
	"testing"
)

func TestSplitHostAndPort(t *testing.T) {
	host, port := splitHostAndPort("localhost:9090")

	if host != "localhost" || port != 9090 {
		t.Error("localhost:9090")
	}

	host, port = splitHostAndPort("localhost")
	if host != "localhost" || port != 80 {
		t.Error("localhost:9090")
	}
}
