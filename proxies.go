package main

import (
	"encoding/binary"
	"flag"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strconv"
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

func (s *Server) tunnelTraffic(w http.ResponseWriter, r *http.Request) {
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

	if proxy := s.conf.findProxy(r.URL.Host, "socks"); proxy == nil {
		// connect directly
		if oconn, err := net.DialTimeout("tcp", r.URL.Host, time.Second*8); err == nil {
			go copyConn(iconn, oconn)
			go copyConn(oconn, iconn)
		} else {
			log.Printf("direct dial %v, error: %v", r.URL.Host, err)
			iconn.Close()
		}
	} else { // socks proxy
		// socks5: http://www.ietf.org/rfc/rfc1928.txt
		if oconn, err := net.DialTimeout("tcp", proxy.Addr, time.Second*8); err == nil {
			oconn.Write([]byte{ // VERSION_AUTH
				5, // PROTO_VER5
				1, //
				0, // NO_AUTH
			})

			buffer := [64]byte{}
			oconn.Read(buffer[:])

			buffer[0] = 5 // VER  5
			buffer[1] = 1 // CMD connect
			buffer[2] = 0 // RSV
			buffer[3] = 3 // DOMAINNAME: X'03'

			host, port, _ := net.SplitHostPort(r.URL.Host)
			portN, _ := strconv.Atoi(port)
			hostBytes := []byte(host)
			buffer[4] = byte(len(hostBytes))
			copy(buffer[5:], hostBytes)
			binary.BigEndian.PutUint16(buffer[5+len(hostBytes):], uint16(portN))
			oconn.Write(buffer[:5+len(hostBytes)+2])

			if n, err := oconn.Read(buffer[:]); n > 1 && err == nil && buffer[1] == 0 {
				go copyConn(iconn, oconn)
				go copyConn(oconn, iconn)
			} else {
				log.Printf("connet to socks server %s error: %v", proxy.Addr, err)
			}
		} else {
			log.Println("dial socks server %v, error: %v", proxy.Addr, err)
			iconn.Close()
		}
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == "CONNECT" {
		s.tunnelTraffic(w, r)
	} else {
		log.Println(r.URL)
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
