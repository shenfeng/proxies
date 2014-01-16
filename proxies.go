package main

import (
	"bufio"
	"bytes"
	"flag"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
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
	DailTimeOut   = 10 * time.Second
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	if t, ok := http.DefaultClient.Transport.(*http.Transport); ok {
		t.MaxIdleConnsPerHost = 4
	}
}

type ProxyServer struct {
	conf *TServerConf

	idleConn map[string][]*Connection
	idleMu   sync.Mutex
	cachedir string
	cache    Cache
}

func NewServer(filename string) (server *ProxyServer, e error) {
	if data, err := ioutil.ReadFile(filename); err == nil {
		if conf, err := readConfigFromBytes(data); err == nil {
			return &ProxyServer{conf: conf, idleConn: make(map[string][]*Connection)}, err
		} else {
			return nil, err
		}
	} else {
		return nil, err
	}
}

func (s *ProxyServer) fetchFromHttpProxy(w http.ResponseWriter, r *http.Request) {

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

func (s *ProxyServer) getCacheName(ireq *http.Request) (cachename string, fullpath string) {
	cachename = path.Join(ireq.URL.Host, url.QueryEscape(ireq.URL.RequestURI()))
	fullpath = path.Join(s.cachedir, cachename)
	return
}

func (s *ProxyServer) copyAndSave(w http.ResponseWriter, resp *http.Response, ireq *http.Request) {
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

func (s *ProxyServer) fetchDirectly(w http.ResponseWriter, ireq *http.Request) {
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

func copyConn(in io.ReadWriteCloser, out io.ReadWriteCloser) {
	buffer := [4096]byte{}
	for {
		if n, err := in.Read(buffer[:]); err == nil {
			out.Write(buffer[:n])
		} else {
			in.Close()
			out.Close()
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

func (s *ProxyServer) tunnelTraffic(iconn net.Conn, brw *bufio.ReadWriter, r *http.Request) {
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

func (s *ProxyServer) fetchHTTP(c *Connection, req []byte) (*http.Response, error) {
	c.Conn.SetDeadline(time.Now().Add(time.Minute * 1))
	if _, err := c.Conn.Write(req); err != nil {
		return nil, err
	} else {
		return http.ReadResponse(c.Reader, c.Request)
	}
}

func (s *ProxyServer) proxyHttp(r *http.Request, c *Connection) {

}

func (s *ProxyServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	in, brw, err := w.(http.Hijacker).Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	hash := getProxyHash(r.Header)
	ureader := NewPushBackReader(in, 10)
	reader := bufio.NewReader(ureader)

	addr := getHost(r)

	if r.Method == "CONNECT" {
		w.WriteHeader(200)

		if conn, err := s.getConn(addr, hash); err == nil {
			if ureader.IsHttpGet(addr) {
				for {
					r, err := http.ReadRequest(reader)
					if err != nil {
						break
					}
					r.Write(conn.Brw)
					resp, err := http.ReadResponse(conn.Brw, r)
					if err != nil {
						break
					}
					resp.Write(brw)
				}
			} else {
				go copyConn(conn, in)
				go copyConn(in, conn)
			}
		} else {
			s.returnConn(conn, true)
		}
	} else {

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
