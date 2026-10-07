package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

type replyKind int

const (
	replyValue replyKind = iota
	replyNil
)

type client struct {
	conn net.Conn
	r    *bufio.Reader
	w    *bufio.Writer
}

func dial(addr string) (*client, error) {
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return nil, err
	}

	return &client{
		conn: conn,
		r:    bufio.NewReader(conn),
		w:    bufio.NewWriter(conn),
	}, nil
}

func (c *client) Close() error {
	return c.conn.Close()
}

func (c *client) Do(payload []byte) (replyKind, error) {
	if err := c.conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return 0, err
	}

	if _, err := c.w.Write(payload); err != nil {
		return 0, err
	}

	if err := c.w.Flush(); err != nil {
		return 0, err
	}

	return readReply(c.r)
}

func (c *client) DoMany(payloads [][]byte) ([]replyKind, error) {
	if err := c.conn.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
		return nil, err
	}

	for _, payload := range payloads {
		if _, err := c.w.Write(payload); err != nil {
			return nil, err
		}
	}

	if err := c.w.Flush(); err != nil {
		return nil, err
	}

	kinds := make([]replyKind, len(payloads))
	for i := range payloads {
		kind, err := readReply(c.r)
		if err != nil {
			return nil, err
		}
		kinds[i] = kind
	}

	return kinds, nil
}

func encodeArray(args ...string) []byte {
	var b strings.Builder

	fmt.Fprintf(&b, "*%d\r\n", len(args))

	for _, arg := range args {
		fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(arg), arg)
	}

	return []byte(b.String())
}

func readReply(r *bufio.Reader) (replyKind, error) {
	prefix, err := r.ReadByte()
	if err != nil {
		return 0, err
	}

	switch prefix {
	case '+', ':':
		_, err = r.ReadSlice('\n')
		return replyValue, err

	case '-':
		line, err := r.ReadString('\n')
		if err != nil {
			return 0, err
		}

		return 0, fmt.Errorf("server error: %s", strings.TrimSpace(line))

	case '$':
		line, err := r.ReadSlice('\n')
		if err != nil {
			return 0, err
		}

		n, err := strconv.Atoi(strings.TrimSpace(string(line)))
		if err != nil {
			return 0, fmt.Errorf("invalid bulk length: %w", err)
		}

		if n < 0 {
			return replyNil, nil
		}

		if _, err := r.Discard(n + 2); err != nil {
			return 0, err
		}

		return replyValue, nil

	default:
		return 0, fmt.Errorf("unexpected reply prefix %q", prefix)
	}
}

func ping(addr string) error {
	c, err := dial(addr)
	if err != nil {
		return err
	}
	defer c.Close()

	_, err = c.Do(encodeArray("PING"))
	return err
}

func waitReady(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last error

	for time.Now().Before(deadline) {
		last = ping(addr)
		if last == nil {
			return nil
		}

		if last == io.EOF {
			last = fmt.Errorf("connection closed")
		}

		time.Sleep(20 * time.Millisecond)
	}

	return fmt.Errorf("server %s not ready: %w", addr, last)
}
