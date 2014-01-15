package main

import (
	"net/http"
	"bufio"
	"strings"
	"log"
)

type Reader strings.Reader

func (r *Reader) Close() error {
	return nil
}

func (r *Reader) Read(p []byte) (int, error) {
	return (*strings.Reader)(r).Read(p)
}

func NewReader(s string) *Reader {
	return (*Reader)(strings.NewReader(s))
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func HandleFunc(w http.ResponseWriter, r *http.Request) {
	if in, _, err := w.(http.Hijacker).Hijack(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	} else {
		for {
			log.Println(in.RemoteAddr(), r)
			bufin := bufio.NewReader(in)
			header := http.Header{}
			body := "hello world"
			resp := http.Response {
				Status: "200 ok",
				StatusCode: 200,
				Proto: "http/1.1",
				ProtoMajor: 1,
				ProtoMinor: 1,
				ContentLength: int64(len(body)),
				Header: header,
				Body: NewReader(body),
			}
			if err := resp.Write(in); err != nil {
				log.Println(err)
				in.Close()
				break
			}
			if r, err = http.ReadRequest(bufin); err != nil {
				log.Println("read", err)
				break
			}
		}
	}
}

func main() {
	//	go func() {
	http.ListenAndServe("0.0.0.0:8787", http.HandlerFunc(HandleFunc));
	//	}()


	//	if ln, err := net.Listen("tcp", "0.0.0.0:7878"); err != nil {
	//		log.Fatal(err)
	//	} else {
	//		for {
	//			if conn, err := ln.Accept(); err != nil {
	//				log.Println(err)
	//			} else {
	//				go func() {
	//
	//				}()
	//			}
	//		}
	//	}

}
