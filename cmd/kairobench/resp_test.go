package main

import (
	"bufio"
	"strings"
	"testing"
	"time"
)

func TestReadReply(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		kind    replyKind
		wantErr bool
	}{
		{name: "simple string", input: "+OK\r\n", kind: replyValue},
		{name: "integer", input: ":1\r\n", kind: replyValue},
		{name: "nil bulk", input: "$-1\r\n", kind: replyNil},
		{name: "bulk string", input: "$3\r\nbar\r\n", kind: replyValue},
		{name: "error", input: "-ERR nope\r\n", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kind, err := readReply(bufio.NewReader(strings.NewReader(tc.input)))
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}

			if err != nil {
				t.Fatal(err)
			}

			if kind != tc.kind {
				t.Fatalf("kind %d, want %d", kind, tc.kind)
			}
		})
	}
}

func TestEncodeArraySet(t *testing.T) {
	got := string(encodeArray("SET", "k:0000000", "xx", "EX", "600"))
	want := "*5\r\n$3\r\nSET\r\n$9\r\nk:0000000\r\n$2\r\nxx\r\n$2\r\nEX\r\n$3\r\n600\r\n"

	if got != want {
		t.Fatalf("got %q", got)
	}
}

func TestNearestRank(t *testing.T) {
	samples := make([]time.Duration, 100)
	for i := range samples {
		samples[i] = time.Duration(i+1) * time.Microsecond
	}

	if got := nearestRank(samples, 50); got != 50*time.Microsecond {
		t.Fatalf("p50 = %s", got)
	}

	if got := nearestRank(samples, 99); got != 99*time.Microsecond {
		t.Fatalf("p99 = %s", got)
	}
}

func TestMemoryBytesPerKey(t *testing.T) {
	m := memoryResult{Baseline: 1_000, AfterLoad: 6_000, Keys: 50}

	if got := m.BytesPerKey(); got != 100 {
		t.Fatalf("bytes/key = %v", got)
	}

	m.AfterWrite = 8_000
	if got := m.OverwriteGrowth(); got != 2_000 {
		t.Fatalf("growth = %d", got)
	}
}
