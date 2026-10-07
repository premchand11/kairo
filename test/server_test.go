package tests

import (
	"bufio"
	"io"
	"net"
	"testing"
	"time"

	"github.com/premchandpanku/kairo/internal/engine"
	"github.com/premchandpanku/kairo/internal/server"
)

func TestServerPing(t *testing.T) {
	eng := engine.New()
	defer eng.Close()

	srv := server.New(eng)

	client, serverConn := net.Pipe()
	defer client.Close()
	defer serverConn.Close()

	go srv.Handle(serverConn)

	_, err := client.Write([]byte("*1\r\n$4\r\nPING\r\n"))
	if err != nil {
		t.Fatal(err)
	}

	reader := bufio.NewReader(client)

	response, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}

	if response != "+PONG\r\n" {
		t.Fatalf("expected +PONG, got %q", response)
	}
}

func TestServerSetGet(t *testing.T) {
	eng := engine.New()
	defer eng.Close()

	srv := server.New(eng)

	client, serverConn := net.Pipe()
	defer client.Close()
	defer serverConn.Close()

	go srv.Handle(serverConn)

	reader := bufio.NewReader(client)

	// SET foo bar
	_, err := client.Write([]byte("*3\r\n$3\r\nSET\r\n$3\r\nfoo\r\n$3\r\nbar\r\n"))
	if err != nil {
		t.Fatal(err)
	}

	response, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}

	if response != "+OK\r\n" {
		t.Fatalf("expected +OK, got %q", response)
	}

	// GET foo
	_, err = client.Write([]byte("*2\r\n$3\r\nGET\r\n$3\r\nfoo\r\n"))
	if err != nil {
		t.Fatal(err)
	}

	response, err = reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}

	if response != "$3\r\n" {
		t.Fatalf("expected $3\\r\\n, got %q", response)
	}

	value := make([]byte, 5)
	_, err = io.ReadFull(reader, value)
	if err != nil {
		t.Fatal(err)
	}

	if string(value) != "bar\r\n" {
		t.Fatalf("expected bar, got %q", string(value))
	}
}

func TestServerDelete(t *testing.T) {
	eng := engine.New()
	defer eng.Close()

	srv := server.New(eng)

	client, serverConn := net.Pipe()
	defer client.Close()
	defer serverConn.Close()

	go srv.Handle(serverConn)

	reader := bufio.NewReader(client)

	// SET foo bar
	_, err := client.Write([]byte("*3\r\n$3\r\nSET\r\n$3\r\nfoo\r\n$3\r\nbar\r\n"))
	if err != nil {
		t.Fatal(err)
	}

	response, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}

	if response != "+OK\r\n" {
		t.Fatalf("expected +OK, got %q", response)
	}

	// DEL foo
	_, err = client.Write([]byte("*2\r\n$3\r\nDEL\r\n$3\r\nfoo\r\n"))
	if err != nil {
		t.Fatal(err)
	}

	response, err = reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}

	if response != ":1\r\n" {
		t.Fatalf("expected :1\\r\\n, got %q", response)
	}

	// GET foo
	_, err = client.Write([]byte("*2\r\n$3\r\nGET\r\n$3\r\nfoo\r\n"))
	if err != nil {
		t.Fatal(err)
	}

	response, err = reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}

	if response != "$-1\r\n" {
		t.Fatalf("expected $-1\\r\\n, got %q", response)
	}
}

func TestServerExpireTTL(t *testing.T) {
	eng := engine.New()
	defer eng.Close()

	srv := server.New(eng)

	client, serverConn := net.Pipe()
	defer client.Close()
	defer serverConn.Close()

	go srv.Handle(serverConn)

	reader := bufio.NewReader(client)

	// SET foo bar
	_, err := client.Write([]byte("*3\r\n$3\r\nSET\r\n$3\r\nfoo\r\n$3\r\nbar\r\n"))
	if err != nil {
		t.Fatal(err)
	}

	response, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}

	if response != "+OK\r\n" {
		t.Fatalf("expected +OK, got %q", response)
	}

	// EXPIRE foo 2
	_, err = client.Write([]byte("*3\r\n$6\r\nEXPIRE\r\n$3\r\nfoo\r\n$1\r\n2\r\n"))
	if err != nil {
		t.Fatal(err)
	}

	response, err = reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}

	if response != ":1\r\n" {
		t.Fatalf("expected :1\\r\\n, got %q", response)
	}

	// TTL foo
	_, err = client.Write([]byte("*2\r\n$3\r\nTTL\r\n$3\r\nfoo\r\n"))
	if err != nil {
		t.Fatal(err)
	}

	response, err = reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}

	if response != ":2\r\n" && response != ":1\r\n" {
		t.Fatalf("expected TTL 1 or 2, got %q", response)
	}

	// Wait for expiration.
	time.Sleep(4 * time.Second)

	// GET foo
	_, err = client.Write([]byte("*2\r\n$3\r\nGET\r\n$3\r\nfoo\r\n"))
	if err != nil {
		t.Fatal(err)
	}

	response, err = reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}

	if response != "$-1\r\n" {
		t.Fatalf("expected $-1\\r\\n after expiration, got %q", response)
	}
}

func TestServerUnknownCommand(t *testing.T) {
	eng := engine.New()
	defer eng.Close()

	srv := server.New(eng)

	client, serverConn := net.Pipe()
	defer client.Close()
	defer serverConn.Close()

	go srv.Handle(serverConn)

	reader := bufio.NewReader(client)

	_, err := client.Write([]byte("*1\r\n$4\r\nNOPE\r\n"))
	if err != nil {
		t.Fatal(err)
	}

	response, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}

	if response != "-ERR unknown command 'NOPE'\r\n" {
		t.Fatalf("unexpected response: %q", response)
	}
}

func TestServerWrongArguments(t *testing.T) {
	eng := engine.New()
	defer eng.Close()

	srv := server.New(eng)

	client, serverConn := net.Pipe()
	defer client.Close()
	defer serverConn.Close()

	go srv.Handle(serverConn)

	reader := bufio.NewReader(client)

	// GET with no key
	_, err := client.Write([]byte("*1\r\n$3\r\nGET\r\n"))
	if err != nil {
		t.Fatal(err)
	}

	response, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}

	if response != "-ERR wrong number of arguments\r\n" {
		t.Fatalf("unexpected response: %q", response)
	}

	// Connection should still work.
	_, err = client.Write([]byte("*1\r\n$4\r\nPING\r\n"))
	if err != nil {
		t.Fatal(err)
	}

	response, err = reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}

	if response != "+PONG\r\n" {
		t.Fatalf("expected connection to remain usable, got %q", response)
	}
}

func TestServerSetWithExpiration(t *testing.T) {
	eng := engine.New()
	defer eng.Close()

	srv := server.New(eng)

	client, serverConn := net.Pipe()
	defer client.Close()
	defer serverConn.Close()

	go srv.Handle(serverConn)

	reader := bufio.NewReader(client)

	// SET foo bar EX 2
	_, err := client.Write([]byte(
		"*5\r\n$3\r\nSET\r\n$3\r\nfoo\r\n$3\r\nbar\r\n$2\r\nEX\r\n$1\r\n2\r\n",
	))
	if err != nil {
		t.Fatal(err)
	}

	response, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}

	if response != "+OK\r\n" {
		t.Fatalf("expected +OK, got %q", response)
	}

	// TTL foo
	_, err = client.Write([]byte("*2\r\n$3\r\nTTL\r\n$3\r\nfoo\r\n"))
	if err != nil {
		t.Fatal(err)
	}

	response, err = reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}

	if response != ":2\r\n" && response != ":1\r\n" {
		t.Fatalf("expected TTL 1 or 2, got %q", response)
	}

	// GET foo
	_, err = client.Write([]byte("*2\r\n$3\r\nGET\r\n$3\r\nfoo\r\n"))
	if err != nil {
		t.Fatal(err)
	}

	response, err = reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}

	if response != "$3\r\n" {
		t.Fatalf("expected $3\\r\\n, got %q", response)
	}

	value := make([]byte, 5)
	_, err = io.ReadFull(reader, value)
	if err != nil {
		t.Fatal(err)
	}

	if string(value) != "bar\r\n" {
		t.Fatalf("expected bar, got %q", string(value))
	}
}

func TestServerSetWithPX(t *testing.T) {
	eng := engine.New()
	defer eng.Close()

	srv := server.New(eng)

	client, serverConn := net.Pipe()
	defer client.Close()
	defer serverConn.Close()

	go srv.Handle(serverConn)

	reader := bufio.NewReader(client)

	// SET foo bar PX 2000
	_, err := client.Write([]byte(
		"*5\r\n$3\r\nSET\r\n$3\r\nfoo\r\n$3\r\nbar\r\n$2\r\nPX\r\n$4\r\n2000\r\n",
	))
	if err != nil {
		t.Fatal(err)
	}

	response, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}

	if response != "+OK\r\n" {
		t.Fatalf("expected +OK, got %q", response)
	}

	// TTL should report approximately 2 seconds.
	_, err = client.Write([]byte("*2\r\n$3\r\nTTL\r\n$3\r\nfoo\r\n"))
	if err != nil {
		t.Fatal(err)
	}

	response, err = reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}

	if response != ":2\r\n" && response != ":1\r\n" {
		t.Fatalf("expected TTL 1 or 2, got %q", response)
	}

	// GET should still return the value.
	_, err = client.Write([]byte("*2\r\n$3\r\nGET\r\n$3\r\nfoo\r\n"))
	if err != nil {
		t.Fatal(err)
	}

	response, err = reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}

	if response != "$3\r\n" {
		t.Fatalf("expected $3\\r\\n, got %q", response)
	}

	value := make([]byte, 5)
	_, err = io.ReadFull(reader, value)
	if err != nil {
		t.Fatal(err)
	}

	if string(value) != "bar\r\n" {
		t.Fatalf("expected bar, got %q", string(value))
	}
}

func TestServerMGet(t *testing.T) {
	eng := engine.New()
	defer eng.Close()

	srv := server.New(eng)

	client, serverConn := net.Pipe()
	defer client.Close()
	defer serverConn.Close()

	go srv.Handle(serverConn)

	reader := bufio.NewReader(client)

	// SET foo one
	_, err := client.Write([]byte(
		"*3\r\n$3\r\nSET\r\n$3\r\nfoo\r\n$3\r\none\r\n",
	))
	if err != nil {
		t.Fatal(err)
	}

	if response, err := reader.ReadString('\n'); err != nil {
		t.Fatal(err)
	} else if response != "+OK\r\n" {
		t.Fatalf("expected +OK, got %q", response)
	}

	// SET bar two
	_, err = client.Write([]byte(
		"*3\r\n$3\r\nSET\r\n$3\r\nbar\r\n$3\r\ntwo\r\n",
	))
	if err != nil {
		t.Fatal(err)
	}

	if response, err := reader.ReadString('\n'); err != nil {
		t.Fatal(err)
	} else if response != "+OK\r\n" {
		t.Fatalf("expected +OK, got %q", response)
	}

	// MGET foo bar missing
	_, err = client.Write([]byte(
		"*4\r\n$4\r\nMGET\r\n$3\r\nfoo\r\n$3\r\nbar\r\n$7\r\nmissing\r\n",
	))
	if err != nil {
		t.Fatal(err)
	}

	response, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}

	if response != "*3\r\n" {
		t.Fatalf("expected array of 3, got %q", response)
	}

	response, err = reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}

	if response != "$3\r\n" {
		t.Fatalf("expected $3\\r\\n, got %q", response)
	}

	value := make([]byte, 5)
	if _, err := io.ReadFull(reader, value); err != nil {
		t.Fatal(err)
	}

	if string(value) != "one\r\n" {
		t.Fatalf("expected one, got %q", string(value))
	}

	response, err = reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}

	if response != "$3\r\n" {
		t.Fatalf("expected $3\\r\\n, got %q", response)
	}

	value = make([]byte, 5)
	if _, err := io.ReadFull(reader, value); err != nil {
		t.Fatal(err)
	}

	if string(value) != "two\r\n" {
		t.Fatalf("expected two, got %q", string(value))
	}

	response, err = reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}

	if response != "$-1\r\n" {
		t.Fatalf("expected nil, got %q", response)
	}
}

func TestServerPipeline(t *testing.T) {
	eng := engine.New()
	defer eng.Close()

	srv := server.New(eng)

	client, serverConn := net.Pipe()
	defer client.Close()
	defer serverConn.Close()

	go srv.Handle(serverConn)

	_, err := client.Write([]byte("*1\r\n$4\r\nPING\r\n*1\r\n$4\r\nPING\r\n"))
	if err != nil {
		t.Fatal(err)
	}

	reader := bufio.NewReader(client)
	for i := 0; i < 2; i++ {
		response, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if response != "+PONG\r\n" {
			t.Fatalf("reply %d = %q", i, response)
		}
	}
}

func TestServerMaxMemory(t *testing.T) {
	eng := engine.New()
	eng.SetMaxMemory(3)
	defer eng.Close()

	srv := server.New(eng)

	client, serverConn := net.Pipe()
	defer client.Close()
	defer serverConn.Close()

	go srv.Handle(serverConn)

	reader := bufio.NewReader(client)
	_, err := client.Write([]byte("*3\r\n$3\r\nSET\r\n$1\r\na\r\n$3\r\nbar\r\n"))
	if err != nil {
		t.Fatal(err)
	}

	response, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if response != "-ERR maxmemory reached\r\n" {
		t.Fatalf("expected maxmemory error, got %q", response)
	}
}
