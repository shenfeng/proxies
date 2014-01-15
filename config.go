package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Connection struct {
	Conn       net.Conn
	Addr       string
	LastActive time.Time

	Reader  *bufio.Reader
	Request *http.Request
	Backend   *Backend

	ureader *PushBackReader
}

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
	connections []*Connection
}

type TServerConf struct {
	Proxies []*Backend    `json:"proxies"`
	Configs map[string]string `json:"configs"`

	blacklist []*regexp.Regexp
	groups    map[string][]*Backend
}

func (c *Connection) IsGet() (bool, error) {
	if strings.HasSuffix(c.Addr, "80") { // only for HTTP port
		buf := [3]byte{}
		if n, err := c.ureader.Read(buf[:]); err == nil {
			c.ureader.UnRead(buf[:n])
			if "GET" == string(buf[:n]) {
				return true, nil
			} else {
				return false, nil
			}
		} else {
			return false, err
		}
	} else {
		return false, nil
	}
}

func (s *ProxyServer) getConn(addr string, hash int) (*Connection, error) {
	if proxy := s.conf.findProxy(addr, hash); proxy != nil {
		return proxy.getConn(addr)
	}

	s.idleMu.Lock()
	if conns, ok := s.idleConn[addr]; ok && len(conns) > 0 {
		c := conns[len(conns) - 1]
		s.idleConn[addr] = conns[:len(conns) - 1]
		s.idleMu.Unlock()
		return c, nil
	} else {
		s.idleMu.Unlock()
	}

	if c, err := net.DialTimeout("tcp", addr, DailTimeOut); err == nil {
		ureader := NewPushBackReader(c, 10)
		return &Connection{
			Conn:    c,
			Addr:    addr,
			ureader: ureader,
			Reader:  bufio.NewReader(ureader),
		}, nil
	} else {
		return nil, err
	}
}

func (s *ProxyServer) returnConn(c *Connection, close bool) {
	if close {
		c.Conn.Close()
	} else if c.Backend != nil {

	} else {
		s.idleMu.Lock()
		if conns, ok := s.idleConn[c.Addr]; ok && len(conns) > MaxConPerHost {
			c.Conn.Close()
		} else {
			c.LastActive = time.Now()
			s.idleConn[c.Addr] = append(conns, c)
		}
		s.idleMu.Unlock()
	}
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

func (c *TServerConf) findProxy(host string, hash int) *Backend {
	group := ""
	for server, g := range c.Configs {
		if strings.Contains(host, server) {
			group = g
			break
		}
	}

	if proxies, ok := c.groups[group]; ok && len(proxies) > 0 {
		idx, end := hash, hash + len(proxies)

		for ; idx < end; idx++ {
			if proxies[idx%len(proxies)].Dead.Get() == 0 {
				return proxies[idx%len(proxies)]
			}
		}
	}

	return nil
}

func splitHostAndPort(host string) (string, uint16) {
	if idx := strings.Index(host, ":"); idx < 0 {
		return host, 80
	} else {
		port, _ := strconv.Atoi(host[idx + 1:])
		return host[:idx], uint16(port)
	}

}

func (p *Backend) getConn(addr string) (con *Connection, err error) {
	p.mu.Lock()

	if len(p.connections) > 0 {
		con = p.connections[len(p.connections) - 1]
		p.connections = p.connections[0 : len(p.connections) - 1]
	}
	p.mu.Unlock()
	if con != nil {
		return
	}

	oconn, err := net.DialTimeout("tcp", addr, DailTimeOut);
	if err != nil {
		return nil, err
	}

	if p.Type == "socks" {
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

		host, port := splitHostAndPort(addr)
		hostBytes := []byte(host)
		buffer[4] = byte(len(hostBytes))
		copy(buffer[5:], hostBytes)
		binary.BigEndian.PutUint16(buffer[5 + len(hostBytes):], uint16(port))
		oconn.Write(buffer[:5 + len(hostBytes) + 2])
		if n, err := oconn.Read(buffer[:]); n > 1 && err == nil && buffer[1] == 0 {

		} else {
			oconn.Close()
			return nil, fmt.Errorf("connet to socks server %s error: %v", addr, err)
		}
	}

	ureader := NewPushBackReader(oconn, 10)
	return &Connection{
		Conn: oconn,
		Addr: addr,
		ureader: ureader,
		Reader: bufio.NewReader(ureader),
		Backend: p,
	}, nil
}

// if return err, should retry sometime later
func (s *Backend) openConn(d time.Duration, r *http.Request) (con net.Conn, err error) {
	if oconn, err := net.DialTimeout("tcp", s.Addr, d); err == nil {
		if s.Type == "socks" {
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
