package tests

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/premchandpanku/kairo/internal/protocol"
)

func FuzzReadCommand(f *testing.F) {
	f.Add([]byte("*1\r\n$4\r\nPING\r\n"))
	f.Add([]byte("*2\r\n$3\r\nGET\r\n$3\r\nfoo\r\n"))
	f.Add([]byte(""))
	f.Add([]byte("*"))
	f.Add([]byte("*-1\r\n"))
	f.Add([]byte("*1\r\n$4\r\nPI"))

	f.Fuzz(func(t *testing.T, data []byte) {
		reader := bufio.NewReader(bytes.NewReader(data))
		_, _ = protocol.ReadCommand(reader)
	})
}
