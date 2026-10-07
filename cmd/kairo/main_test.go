package main

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/premchandpanku/kairo/internal/engine"
	"github.com/premchandpanku/kairo/internal/server"
)

func TestServeStopsOnCancel(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	eng := engine.New()
	defer eng.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		serve(ctx, listener, server.New(eng))
		close(done)
	}()

	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("serve did not return after cancel")
	}
}
