package main

import (
	"context"
	"flag"
	"log"
	"net"
	"os/signal"
	"sync"
	"syscall"

	"github.com/premchandpanku/kairo/internal/engine"
	"github.com/premchandpanku/kairo/internal/server"
)

func main() {
	addr := flag.String("addr", ":8989", "TCP listen address")
	maxMemory := flag.Int64("max-memory", 0, "maximum sum of key and value bytes; 0 means unlimited")
	maxShards := flag.Int("max-shards", 16, "number of keyspace shards")
	flag.Parse()

	if *maxShards < 1 {
		log.Fatal("max-shards must be at least 1")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	e := engine.NewWithShards(*maxShards)
	defer e.Close()
	e.SetMaxMemory(*maxMemory)

	s := server.New(e)

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Kairo listening on %s", *addr)
	serve(ctx, listener, s)
	log.Printf("Kairo stopped")
}

// serve accepts until ctx is cancelled, then closes the listener and any
// connection still blocked in a read, and waits for those handlers to return.
func serve(ctx context.Context, listener net.Listener, s *server.Server) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	active := make(map[net.Conn]struct{})

	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			log.Println("accept error:", err)
			continue
		}

		mu.Lock()
		active[conn] = struct{}{}
		mu.Unlock()

		wg.Add(1)
		go func(conn net.Conn) {
			defer wg.Done()
			defer func() {
				mu.Lock()
				delete(active, conn)
				mu.Unlock()
				conn.Close()
			}()
			s.Handle(conn)
		}(conn)
	}

	mu.Lock()
	for conn := range active {
		conn.Close()
	}
	mu.Unlock()

	wg.Wait()
}
