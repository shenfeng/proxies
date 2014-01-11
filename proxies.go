package main

import (
	"flag"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	MaxRetry      = 5
	LatencySize   = 128
	MaxConPerHost = 3
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

type Server struct {
	conf     *TServerConf
	idleConn map[string][]*persistConn
	idleMu   sync.Mutex
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
			io.Copy(w, resp.Body)
		}
	}
}

func (s *Server) httpTunnel(w http.ResponseWriter, r *http.Request) {

}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == "CONNECT" {
		w.WriteHeader(200)

		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "webserver doesn't support hijacking", http.StatusInternalServerError)
			return
		}
		iconn, _, err := hj.Hijack()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if oconn, err := net.DialTimeout("tcp", r.URL.Host, time.Millisecond*1000); err == nil {
			go func() { // read loop
				buffer := [4096]byte{}
				for {
					if n, err := iconn.Read(buffer[:]); err == nil {
						log.Println("read iconn", n, iconn.RemoteAddr())
						oconn.Write(buffer[:n])
					} else {
						log.Println("------", err)
						iconn.Close()
						oconn.Close()
						break
					}
				}
			}()

			go func() { // write loop
				buffer := [4096]byte{}
				for {
					if n, err := oconn.Read(buffer[:]); err == nil {
						log.Println("read oconn", n, oconn.LocalAddr())
						iconn.Write(buffer[:n])
					} else {
						log.Println("------wwww", err)
						iconn.Close()
						oconn.Close()
						break
					}
				}
			}()
		} else {
			log.Println("dial timeout")
			iconn.Close()
		}
	} else {
		log.Println(r.URL, r)
		//		s.fetchFromHttpProxy(w, r)
		s.fetchDirectly(w, r)
	}
}

func main() {
	var addr, httpAdmin, conf string
	flag.StringVar(&addr, "addr", "0.0.0.0:6666", "Which Addr the proxy listens")
	flag.StringVar(&httpAdmin, "http", "0.0.0.0:6060", "HTTP admin addr")
	flag.StringVar(&conf, "conf", "config.json", "Config file")
	flag.Parse()

	if server, err := NewServer(conf); err == nil {
		log.Println("Proxy multiplexer listens on", addr)
		log.Fatal(http.ListenAndServe(addr, server))
	} else {
		log.Fatal(err)
	}
}
