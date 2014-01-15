package main

import (
	"flag"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"bytes"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	MaxRetry      = 5
	LatencySize   = 12
	MaxConPerHost = 3
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	if t, ok := http.DefaultClient.Transport.(*http.Transport); ok {
		t.MaxIdleConnsPerHost = 4
	}
}

type Server struct {
	conf     *TServerConf
	idleConn map[string][]*persistConn
	idleMu   sync.Mutex
	cachedir string
}

func NewServer(filename string) (server *Server, e error) {
	if data, err := ioutil.ReadFile(filename); err == nil {
		if conf, err := readConfigFromBytes(data); err == nil {
			return &Server{conf: conf, idleConn: make(map[string][]*persistConn)}, err
		} else {
			return nil, err
		}
	} else {
		return nil, err
	}
}

func (s *Server) fetchFromHttpProxy(w http.ResponseWriter, r *http.Request) {

}

func cacheTime(resp *http.Response) int {
	if cc := resp.Header.Get("Cache-Control"); strings.Contains(cc, "max-age=") {
		//		return 1
		//		if c, err := strconv.Atoi(cc[len("max-age="):]); err == nil {
		//			return c
		//		}
	}
	return 0
}

func (s *Server) getCacheName(ireq *http.Request) (cachename string, fullpath string) {
	cachename = path.Join(ireq.URL.Host, url.QueryEscape(ireq.URL.RequestURI()))
	fullpath = path.Join(s.cachedir, cachename)
	return
}

func (s *Server) copyAndSave(w http.ResponseWriter, resp *http.Response, ireq *http.Request) {
	c := cacheTime(resp)
	if resp.StatusCode == 200 && c > 0 {
		_, fullpath := s.getCacheName(ireq)
		os.MkdirAll(filepath.Dir(fullpath), 0755)
		if file, err := os.Create(fullpath); err == nil {
			log.Println("write to", fullpath)
			buf := make([]byte, 32*1024)
			for {
				nr, er := resp.Body.Read(buf)
				if er == nil {
					_, _ = w.Write(buf[0:nr])
					file.Write(buf[0:nr])
				} else {
					break
				}
			}
			file.Close()
		} else {
			log.Println("open write cache error:", err)
			io.Copy(w, resp.Body)
		}
	} else {
		io.Copy(w, resp.Body)
	}
}

func (s *Server) fetchDirectly(w http.ResponseWriter, ireq *http.Request) {
	if req, err := http.NewRequest(ireq.Method, ireq.URL.String(), ireq.Body); err == nil {
		for k, values := range ireq.Header {
			for _, v := range values {
				req.Header.Add(k, v)
			}
		}
		req.ContentLength = ireq.ContentLength
		// do not follow any redirect， browser will do that
		if resp, err := http.DefaultTransport.RoundTrip(req); err == nil {
			for k, values := range resp.Header {
				for _, v := range values {
					w.Header().Add(k, v)
				}
			}
			defer resp.Body.Close()
			w.WriteHeader(resp.StatusCode)
			s.copyAndSave(w, resp, ireq)
		}
	}
}

func copyConn(iconn net.Conn, oconn net.Conn) {
	buffer := [4096]byte{}
	for {
		if n, err := iconn.Read(buffer[:]); err == nil {
			oconn.Write(buffer[:n])
		} else {
			iconn.Close()
			oconn.Close()
			return
		}
	}
}

func readData(con net.Conn, bytes int) (r []byte, e error) {
	r = make([]byte, bytes)
	nread := 0
	for nread < bytes {
		if n, err := con.Read(r[nread:]); err == nil {
			nread += n
		} else {
			return nil, err
		}
	}
	return r, nil
}

func (s *Server) tunnelTraffic(iconn net.Conn, w http.ResponseWriter, r *http.Request) {

	if proxy := s.conf.findProxy(r.URL.Host, "socks"); proxy == nil {
		log.Printf("direct tunnel %v", r.URL.Host)
		// connect directly
		if oconn, err := net.DialTimeout("tcp", r.URL.Host, time.Second*8); err == nil {
			go copyConn(iconn, oconn)
			go copyConn(oconn, iconn)
		} else {
			log.Printf("direct dial %v, error: %v", r.URL.Host, err)
			iconn.Close()
		}
	} else { // socks proxy
		log.Printf("socks tunnel %v, by %v", r.URL.Host, proxy.Addr)
		if oconn, err := proxy.openConn(time.Second*8, r); err == nil {
			go copyConn(iconn, oconn)
			go copyConn(oconn, iconn)
		} else {
			log.Println("dial socks server %v, error: %v", proxy.Addr, err)
			iconn.Close()
		}
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	if in, _, err := w.(http.Hijacker).Hijack(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	} else if r.Method == "CONNECT" {
		w.WriteHeader(200)
		s.tunnelTraffic(in, w, r)
	} else {

		req := bytes.NewBuffer(make([]byte, 0, 4096))
		if r.Header.Get("Accept-Encoding") == "" && r.Method != "HEAD" {
			r.Header.Set("Accept-Encoding", "gzip") // gzip is good
		}

		proxy := s.conf.findProxy(r.URL.Host, "socks")

		if proxy != nil && proxy.Type == "http" {
			r.WriteProxy(req)
		} else {
			r.Write(req)
		}

		buffer := req.Bytes() // request






//		r.Write(req)

		if proxy := s.conf.findProxy(r.URL.Host, "socks"); proxy != nil {
			r.WriteProxy(req)



			log.Println("directly: ", r.URL)
			s.fetchDirectly(w, r)
		} else {

			r.Write(req)

			if oconn, err := proxy.openConn(time.Second*8, r); err == nil {
				log.Printf("proxy by %v: %v", proxy.Addr, r.URL.Host)
				r.Write(oconn)
				go copyConn(in, oconn)
				go copyConn(oconn, in)
			} else {
				log.Println("open proxy failed", proxy.Addr, err)
				in.Close()
			}
		}

	}

}

func main() {
	var addr, httpAdmin, conf, cachedir string
	flag.StringVar(&addr, "addr", "0.0.0.0:6666", "Which Addr the proxy listens")
	flag.StringVar(&httpAdmin, "http", "0.0.0.0:6060", "HTTP admin addr")
	flag.StringVar(&conf, "conf", "config.json", "Config file")
	flag.StringVar(&cachedir, "cache", "/tmp/pcache", "proxy cache file directory")
	flag.Parse()

	if server, err := NewServer(conf); err == nil {
		server.cachedir = cachedir
		log.Println("Proxy multiplexer listens on", addr)
		log.Fatal(http.ListenAndServe(addr, server))
	} else {
		log.Fatal(err)
	}
}
