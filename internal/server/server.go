package server

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/premchandpanku/kairo/internal/engine"
	"github.com/premchandpanku/kairo/internal/protocol"
)

type Server struct {
	engine *engine.Engine
}

func New(e *engine.Engine) *Server {
	return &Server{
		engine: e,
	}
}

func (s *Server) Handle(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	for {
		args, err := protocol.ReadCommand(reader)

		if err != nil {
			if err == io.EOF {
				return
			}

			_ = protocol.WriteError(writer, err.Error())
			_ = writer.Flush()
			return
		}

		if len(args) == 0 {
			_ = protocol.WriteError(writer, "empty command")
			_ = writer.Flush()
			continue
		}

		switch strings.ToUpper(args[0]) {
		case "PING":
			_ = protocol.WriteSimpleString(writer, "PONG")

		case "SET":
			ttl, err := parseSetTTL(args)
			if err != nil {
				_ = protocol.WriteError(writer, err.Error())
				break
			}

			if !s.engine.Set(args[1], args[2], ttl) {
				_ = protocol.WriteError(writer, "maxmemory reached")
				break
			}
			_ = protocol.WriteSimpleString(writer, "OK")

		case "GET":
			if len(args) != 2 {
				_ = protocol.WriteError(writer, "wrong number of arguments")
				break
			}

			entry, ok := s.engine.Get(args[1])
			if !ok {
				_ = protocol.WriteNil(writer)
			} else {
				_ = protocol.WriteBulkString(writer, entry.Value)
			}

		case "MGET":
			if len(args) < 2 {
				_ = protocol.WriteError(writer, "wrong number of arguments")
				break
			}

			_ = protocol.WriteArrayHeader(writer, len(args)-1)

			for _, key := range args[1:] {
				entry, ok := s.engine.Get(key)

				if !ok {
					_ = protocol.WriteNil(writer)
					continue
				}

				_ = protocol.WriteBulkString(writer, entry.Value)
			}

		case "DEL":
			if len(args) != 2 {
				_ = protocol.WriteError(writer, "wrong number of arguments")
				break
			}

			deleted := s.engine.Delete(args[1])

			if deleted {
				_ = protocol.WriteInteger(writer, 1)
			} else {
				_ = protocol.WriteInteger(writer, 0)
			}

		case "EXPIRE":
			if len(args) != 3 {
				_ = protocol.WriteError(writer, "wrong number of arguments")
				break
			}

			seconds, err := time.ParseDuration(args[2] + "s")
			if err != nil {
				_ = protocol.WriteError(writer, "invalid expiration")
				break
			}

			if s.engine.Expire(args[1], seconds) {
				_ = protocol.WriteInteger(writer, 1)
			} else {
				_ = protocol.WriteInteger(writer, 0)
			}

		case "TTL":
			if len(args) != 2 {
				_ = protocol.WriteError(writer, "wrong number of arguments")
				break
			}

			ttl := s.engine.TTL(args[1])
			_ = protocol.WriteInteger(writer, ttl)

		default:
			_ = protocol.WriteError(
				writer,
				fmt.Sprintf("unknown command '%s'", args[0]),
			)
		}

		// A pipelined client leaves the next command in the buffer.
		// Flush only when this connection has nothing else to read,
		// so one command still gets its reply immediately.
		if reader.Buffered() == 0 {
			if err := writer.Flush(); err != nil {
				return
			}
		}
	}
}

func parseSetTTL(args []string) (time.Duration, error) {
	if len(args) != 3 && len(args) != 5 {
		return 0, fmt.Errorf("wrong number of arguments")
	}

	if len(args) == 3 {
		return 0, nil
	}

	switch strings.ToUpper(args[3]) {
	case "EX":
		seconds, err := strconv.ParseInt(args[4], 10, 64)
		if err != nil || seconds <= 0 {
			return 0, fmt.Errorf("invalid expiration")
		}

		return time.Duration(seconds) * time.Second, nil

	case "PX":
		milliseconds, err := strconv.ParseInt(args[4], 10, 64)
		if err != nil || milliseconds <= 0 {
			return 0, fmt.Errorf("invalid expiration")
		}

		return time.Duration(milliseconds) * time.Millisecond, nil

	default:
		return 0, fmt.Errorf("unsupported SET option")
	}
}
