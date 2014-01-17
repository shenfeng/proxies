package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

type Proxy struct {
	Type  string
	Addr  string
	D     time.Duration
	Error error
}

func (p *Proxy) String() string {
	if p.Error != nil {
		return fmt.Sprintf("%s %s, ERROR: %s", p.Type, p.Addr, p.Error)
	} else {
		return fmt.Sprintf("%s %s, latency: %v", p.Type, p.Addr, p.D)
	}
}

func NewProxy(line string) *Proxy {
	parts := strings.Split(line, " ")
	return &Proxy{
		Type: parts[0],
		Addr: parts[1],
	}
}

func main() {

	var file string
	flag.StringVar(&file, "file", "result.txt", "Proxy list")

	if file, err := os.Open(file); err == nil {
		const C = 600
		results, count, limiting := make(chan *Proxy, C), 0, make(chan int, C)

		seen := make(map[string]string)
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "#") {
				p := NewProxy(line)
				if _, ok := seen[p.Addr]; ok {
					continue
				}
				count += 1
				seen[p.Addr] = p.Type

				go func() {
					limiting <- count
					start := time.Now()
					c, err := net.DialTimeout("tcp", p.Addr, time.Second*10)
					if c != nil {
						c.Close()
					}
					p.D = time.Since(start)
					p.Error = err

					results <- p
					<-limiting
				}()
			}
		}

		success := 0

		ok := make(map[string]string)
		for i := 0; i < count; i++ {
			p := <-results
			log.Println(p)

			if p.Error == nil {
				success += 1
				ok[p.Addr] = p.Type
			}
		}
		log.Printf("total: %d, success: %d, unique: %d", count, success, len(ok))
	} else {
		log.Fatal(err)
	}
}


// 2014/01/16 17:35:25 latency.go:90: total: 4192, success: 1736, unique: 1736
// 2014/01/16 18:03:58 latency.go:90: total: 4350, success: 2079, unique: 2079
// 2014/01/16 18:05:31 latency.go:90: total: 4899, success: 2315, unique: 2315
