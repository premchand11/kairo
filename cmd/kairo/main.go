package main

import (
	"flag"
	"log"
	"net"

	"github.com/premchandpanku/kairo/internal/engine"
	"github.com/premchandpanku/kairo/internal/server"
)

func main() {
	addr := flag.String("addr", ":8989", "TCP listen address")
	maxMemory := flag.Int64("max-memory", 0, "maximum sum of key and value bytes; 0 means unlimited")
	flag.Parse()

	e := engine.New()
	e.SetMaxMemory(*maxMemory)
	defer e.Close()

	s := server.New(e)

	listener, err := net.Listen("tcp", *addr)

	if err != nil {
		log.Fatal(err)
	}

	defer listener.Close()

	log.Printf("Kairo listening on %s", *addr)

	for {
		conn, err := listener.Accept()

		if err != nil {
			log.Println("accept error:", err)
			continue
		}

		go s.Handle(conn)
	}
}
