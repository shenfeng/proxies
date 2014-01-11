package main

import (
	"encoding/json"
	"net"
	"regexp"
	"strconv"
	"sync"
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
	Proxies   []Backend `json:"proxies"`
	BlackList []string  `json:"black"`

	blacklist []*regexp.Regexp
	groups    map[string]*Backend
}

func readConfigFromBytes(data []byte) (*TServerConf, error) {
	cfg := &TServerConf{groups: make(map[string]*Backend)}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	} else {
		for _, b := range cfg.Proxies {
			cfg.groups[b.Type] = &b
			for _, g := range b.Groups {
				cfg.groups[g] = &b
			}
		}
		return cfg, nil
	}
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
			conn: c,
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

// if return err, should retry sometime later
func (s *Backend) getConn(d time.Duration) (con net.Conn, err error) {
	s.mu.Lock()
	if len(s.connections) > 0 {
		con = s.connections[len(s.connections) - 1]
		s.connections = s.connections[0 : len(s.connections) - 1]
	}
	s.mu.Unlock()

	if con != nil {
		return
	}
	return net.DialTimeout("tcp", s.Addr, d)
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
	conn      net.Conn
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
