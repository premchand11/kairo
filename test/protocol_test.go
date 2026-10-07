package tests

import (
	"bufio"
	"strings"
	"testing"

	"github.com/premchandpanku/kairo/internal/protocol"
)

func TestReadCommand(t *testing.T) {
	input := "*2\r\n$3\r\nGET\r\n$3\r\nfoo\r\n"

	reader := bufio.NewReader(strings.NewReader(input))

	command, err := protocol.ReadCommand(reader)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(command) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(command))
	}

	if command[0] != "GET" {
		t.Fatalf("expected GET, got %s", command[0])
	}

	if command[1] != "foo" {
		t.Fatalf("expected foo, got %s", command[1])
	}
}

func TestReadMultipleCommands(t *testing.T) {
	input :=
		"*2\r\n$3\r\nGET\r\n$3\r\nfoo\r\n" +
			"*2\r\n$3\r\nGET\r\n$3\r\nbar\r\n"

	reader := bufio.NewReader(strings.NewReader(input))

	first, err := protocol.ReadCommand(reader)

	if err != nil {
		t.Fatalf("unexpected error reading first command: %v", err)
	}

	second, err := protocol.ReadCommand(reader)

	if err != nil {
		t.Fatalf("unexpected error reading second command: %v", err)
	}

	if first[1] != "foo" {
		t.Fatalf("expected foo, got %s", first[1])
	}

	if second[1] != "bar" {
		t.Fatalf("expected bar, got %s", second[1])
	}
}

func TestReadCommandInvalidArray(t *testing.T) {
	input := "hello\r\n"

	reader := bufio.NewReader(strings.NewReader(input))

	_, err := protocol.ReadCommand(reader)

	if err == nil {
		t.Fatal("expected error")
	}
}

func TestReadCommandInvalidBulkString(t *testing.T) {
	input := "*1\r\n+3\r\nGET\r\n"

	reader := bufio.NewReader(strings.NewReader(input))

	_, err := protocol.ReadCommand(reader)

	if err == nil {
		t.Fatal("expected error")
	}
}

func TestReadEmptyCommand(t *testing.T) {
	input := "*0\r\n"

	reader := bufio.NewReader(strings.NewReader(input))

	command, err := protocol.ReadCommand(reader)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(command) != 0 {
		t.Fatalf("expected empty command, got %d parts", len(command))
	}
}

func TestWriteResponses(t *testing.T) {
	var buffer strings.Builder

	if err := protocol.WriteSimpleString(&buffer, "OK"); err != nil {
		t.Fatal(err)
	}

	if buffer.String() != "+OK\r\n" {
		t.Fatalf("unexpected response: %q", buffer.String())
	}

	buffer.Reset()

	if err := protocol.WriteInteger(&buffer, 1); err != nil {
		t.Fatal(err)
	}

	if buffer.String() != ":1\r\n" {
		t.Fatalf("unexpected response: %q", buffer.String())
	}

	buffer.Reset()

	if err := protocol.WriteBulkString(&buffer, "hello"); err != nil {
		t.Fatal(err)
	}

	if buffer.String() != "$5\r\nhello\r\n" {
		t.Fatalf("unexpected response: %q", buffer.String())
	}

	buffer.Reset()

	if err := protocol.WriteNil(&buffer); err != nil {
		t.Fatal(err)
	}

	if buffer.String() != "$-1\r\n" {
		t.Fatalf("unexpected response: %q", buffer.String())
	}
}

func TestReadCommandRejectsNegativeBulkLength(t *testing.T) {
	input := "*1\r\n$-2\r\nfoo\r\n"

	reader := bufio.NewReader(strings.NewReader(input))

	_, err := protocol.ReadCommand(reader)
	if err == nil {
		t.Fatal("expected error for negative bulk length")
	}
}

func TestReadCommandRejectsInvalidCRLF(t *testing.T) {
	input := "*1\n$4\r\nPING\r\n"

	reader := bufio.NewReader(strings.NewReader(input))

	_, err := protocol.ReadCommand(reader)
	if err == nil {
		t.Fatal("expected error for invalid CRLF")
	}
}

func TestReadCommandRejectsOversizedBulkString(t *testing.T) {
	input := "*1\r\n$1048577\r\n"

	reader := bufio.NewReader(strings.NewReader(input))

	_, err := protocol.ReadCommand(reader)
	if err == nil {
		t.Fatal("expected error for oversized bulk string")
	}
}
