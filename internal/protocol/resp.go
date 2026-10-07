package protocol

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
)

const maxBulkStringSize = 1 << 20 // 1 MiB

var (
	crlf      = []byte("\r\n")
	plus      = []byte("+")
	errPrefix = []byte("-ERR ")
	nilBulk   = []byte("$-1\r\n")
)

func ReadCommand(r *bufio.Reader) ([]string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}

	if len(line) < 3 || line[0] != '*' {
		return nil, fmt.Errorf("expected RESP array")
	}

	count, err := strconv.Atoi(line[1 : len(line)-2])
	if err != nil {
		return nil, fmt.Errorf("invalid array length")
	}

	parts := make([]string, count)

	for i := 0; i < count; i++ {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}

		if len(line) < 3 || line[0] != '$' {
			return nil, fmt.Errorf("expected bulk string")
		}

		length, err := strconv.Atoi(line[1 : len(line)-2])
		if err != nil {
			return nil, fmt.Errorf("invalid bulk string length")
		}
		if length < 0 {
			return nil, fmt.Errorf("invalid bulk string length")
		}

		if length > maxBulkStringSize {
			return nil, fmt.Errorf("bulk string too large")
		}

		data := make([]byte, length+2)

		if _, err := io.ReadFull(r, data); err != nil {
			return nil, err
		}

		if data[length] != '\r' || data[length+1] != '\n' {
			return nil, fmt.Errorf("invalid bulk string termination")
		}

		parts[i] = string(data[:length])
	}

	return parts, nil
}

func WriteSimpleString(w io.Writer, value string) error {
	if _, err := w.Write(plus); err != nil {
		return err
	}
	if _, err := io.WriteString(w, value); err != nil {
		return err
	}

	_, err := w.Write(crlf)
	return err
}

func WriteError(w io.Writer, message string) error {
	if _, err := w.Write(errPrefix); err != nil {
		return err
	}
	if _, err := io.WriteString(w, message); err != nil {
		return err
	}

	_, err := w.Write(crlf)
	return err
}

func WriteInteger(w io.Writer, value int64) error {
	return writePrefixedInt(w, ':', value)
}

func WriteBulkString(w io.Writer, value string) error {
	if err := writePrefixedInt(w, '$', int64(len(value))); err != nil {
		return err
	}
	if _, err := io.WriteString(w, value); err != nil {
		return err
	}

	_, err := w.Write(crlf)
	return err
}

func WriteNil(w io.Writer) error {
	_, err := w.Write(nilBulk)
	return err
}

func WriteArrayHeader(w io.Writer, count int) error {
	return writePrefixedInt(w, '*', int64(count))
}

func writePrefixedInt(w io.Writer, prefix byte, value int64) error {
	var buf [24]byte
	buf[0] = prefix
	encoded := strconv.AppendInt(buf[:1], value, 10)
	encoded = append(encoded, '\r', '\n')
	_, err := w.Write(encoded)
	return err
}
