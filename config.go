package main

import (
	"encoding/json"
	"math/rand"
	"encoding/binary"
	"net"
	"regexp"
	"strconv"
	"strings"
	"net/http"
	"sync"
	"fmt"
	"sync/atomic"
	"time"
)

type Backend struct { // real proxy server
	Addr   string    `json:"addr"`
	Type   string    `json:"type"` // http, socks
	Groups []string  `json:"groups"`
	Hits   AtomicInt `json:"hits"`

	Fails     AtomicInt                  `json:"fails"`     // TCP failed
	Timeouts  AtomicInt                  `json:"timeouts"`  // timeout
	Ongoing   AtomicInt                  `json:"ongoing"`   // on going request
	Dead      AtomicInt                  `json:"dead"`      // is dead
	Latencies [LatencySize]time.Duration `json:"latencies"` //	round buffer

	mu          sync.Mutex //	only used by connections
	connections []net.Conn
}

type TServerConf struct {
	Proxies   []*Backend `json:"proxies"`
	BlackList []string   `json:"blacks"`

	blacklist []*regexp.Regexp
	groups    map[string][]*Backend
}

func readConfigFromBytes(data []byte) (*TServerConf, error) {
	cfg := &TServerConf{groups: make(map[string][]*Backend)}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	} else {
		for _, b := range cfg.Proxies {
			cfg.groups[b.Type] = append(cfg.groups[b.Type], b)
			for _, g := range b.Groups {
				cfg.groups[g] = append(cfg.groups[g], b)
			}
		}
		return cfg, nil
	}
}

func (c *TServerConf) findProxy(host string, group string) *Backend {
	use_proxy := false
	for _, black := range c.BlackList {
		if strings.Contains(host, black) {
			use_proxy = true
			break
		}
	}

	if use_proxy && len(c.groups[group]) > 0 {
		ss := c.groups[group]
		return ss[rand.Intn(len(ss))]
	}

	return nil
}


func (s *Server) getConn(addr string, d time.Duration) (con *persistConn, err error) {
	s.idleMu.Lock()
	conns := s.idleConn[addr]
	if len(conns) > 0 {
		con = conns[len(conns) - 1]
		s.idleConn[addr] = conns[0 : len(conns) - 1]
	}
	s.idleMu.Unlock()

	if con != nil {
		return
	}
	if c, err := net.DialTimeout("tcp", addr, d); err == nil {
		return &persistConn{
			cacheKey: addr,
			conn:     c,
		}, nil
	} else {
		return nil, err
	}
}

func (s *Server) returnConn(p *persistConn) {
	s.idleMu.Lock()
	if len(s.idleConn[p.cacheKey]) >= MaxConPerHost {
		p.conn.Close()
	} else {
		s.idleConn[p.cacheKey] = append(s.idleConn[p.cacheKey], p)
	}
	s.idleMu.Unlock()
}

func splitHostAndPort(host string) (string, uint16) {
	if idx := strings.Index(host, ":"); idx < 0 {
		return host, 80
	} else {
		port, _ := strconv.Atoi(host[idx+1:])
		return host[:idx], uint16(port)
	}

}

// if return err, should retry sometime later
func (s *Backend) openConn(d time.Duration, r *http.Request) (con net.Conn, err error) {
	if oconn, err := net.DialTimeout("tcp", s.Addr, d); err == nil {
		if (s.Type == "socks") {
			// socks5: http://www.ietf.org/rfc/rfc1928.txt
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

			host, port := splitHostAndPort(r.URL.Host)
			hostBytes := []byte(host)
			buffer[4] = byte(len(hostBytes))
			copy(buffer[5:], hostBytes)
			binary.BigEndian.PutUint16(buffer[5 + len(hostBytes):], uint16(port))
			oconn.Write(buffer[:5 + len(hostBytes) + 2])
			if n, err := oconn.Read(buffer[:]); n > 1 && err == nil && buffer[1] == 0 {
				return oconn, nil
			} else {
				oconn.Close()
				return nil, fmt.Errorf("connet to socks server %s error: %v", s.Addr, err)
			}
		} else {
			return oconn, err
		}
	} else {
		return nil, err
	}
}

func (s *Backend) returnConn(c net.Conn) {
	if c != nil {
		s.mu.Lock()
		if len(s.connections) >= MaxConPerHost {
			c.Close()
		} else {
			s.connections = append(s.connections, c)
		}
		s.mu.Unlock()
	}
}

type persistConn struct {
	cacheKey string // addr
	conn     net.Conn
}

// An AtomicInt is an int64 to be accessed atomically.
type AtomicInt int64

// Add atomically adds n to i.
func (i *AtomicInt) Add(n int64) {
	atomic.AddInt64((*int64)(i), n)
}

// Get atomically gets the value of i.
func (i *AtomicInt) Get() int64 {
	return atomic.LoadInt64((*int64)(i))
}

// Get atomically gets the value of i.
func (i *AtomicInt) Set(v int64) {
	atomic.StoreInt64((*int64)(i), v)
}

func (i *AtomicInt) String() string {
	return strconv.FormatInt(i.Get(), 10)
}
