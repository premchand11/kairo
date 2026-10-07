package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type config struct {
	keys     int
	rounds   int
	clients  int
	value    int
	ttl      int
	warmup   time.Duration
	duration time.Duration
	root     string
}

type memoryResult struct {
	Baseline   uint64
	AfterLoad  uint64
	AfterWrite uint64
	Keys       int
	Rounds     int
}

func (m memoryResult) BytesPerKey() float64 {
	if m.Keys == 0 || m.AfterLoad < m.Baseline {
		return 0
	}

	return float64(m.AfterLoad-m.Baseline) / float64(m.Keys)
}

func (m memoryResult) OverwriteGrowth() int64 {
	return int64(m.AfterWrite) - int64(m.AfterLoad)
}

type serverResult struct {
	Name    string
	Memory  memoryResult
	One     latencyResult
	Many    latencyResult
	Pipe    latencyResult
	Version string
}

func main() {
	cfg := config{}
	flag.IntVar(&cfg.keys, "keys", 50000, "distinct keys in the working set")
	flag.IntVar(&cfg.rounds, "rounds", 10, "extra full overwrites after the initial load")
	flag.IntVar(&cfg.clients, "clients", 32, "concurrent connections for the multi-client run")
	flag.IntVar(&cfg.value, "value-size", 64, "value size in bytes")
	flag.IntVar(&cfg.ttl, "ttl", 600, "SET EX ttl in seconds")
	flag.DurationVar(&cfg.warmup, "warmup", 2*time.Second, "unmeasured mixed-workload warmup")
	flag.DurationVar(&cfg.duration, "duration", 10*time.Second, "measured mixed-workload duration")
	flag.StringVar(&cfg.root, "root", ".", "module root used to build Kairo")
	flag.Parse()

	if cfg.keys < 1 || cfg.rounds < 1 || cfg.clients < 1 || cfg.value < 1 || cfg.ttl < 1 {
		fmt.Fprintln(os.Stderr, "keys, rounds, clients, value-size, and ttl must be positive")
		os.Exit(2)
	}

	fmt.Printf("machine: %s/%s cpus=%d go=%s redis=%s\n", runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), goVersion(), redisVersion())
	fmt.Printf("workload: %d keys, %d byte values, SET EX %d, %d overwrite rounds, mixed 50%% GET / 50%% SET, plus pipeline 16\n", cfg.keys, cfg.value, cfg.ttl, cfg.rounds)

	bin, err := buildKairo(cfg.root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	kairo, err := measureServer("Kairo", cfg, func() (*runningServer, error) {
		return startKairo(bin, "127.0.0.1:18989")
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "kairo:", err)
		os.Exit(1)
	}

	redis, err := measureServer("Redis", cfg, func() (*runningServer, error) {
		return startRedis("127.0.0.1:16379")
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "redis:", err)
		os.Exit(1)
	}

	printReport(kairo, redis)
}

func measureServer(name string, cfg config, start func() (*runningServer, error)) (serverResult, error) {
	fmt.Printf("\n--- %s ---\n", name)

	server, err := start()
	if err != nil {
		return serverResult{}, err
	}
	defer server.Stop()

	time.Sleep(200 * time.Millisecond)

	baseline, err := server.RSS()
	if err != nil {
		return serverResult{}, err
	}

	payloads := buildPayloads(cfg.keys, strings.Repeat("x", cfg.value), cfg.ttl)
	loadClients := clampClients(cfg.clients, cfg.keys)

	fmt.Printf("loading %d keys with %d clients\n", cfg.keys, loadClients)
	if err := loadKeys(server.addr, payloads, loadClients); err != nil {
		return serverResult{}, err
	}

	time.Sleep(200 * time.Millisecond)

	afterLoad, err := server.RSS()
	if err != nil {
		return serverResult{}, err
	}

	for round := 1; round <= cfg.rounds; round++ {
		fmt.Printf("overwrite %d/%d\n", round, cfg.rounds)
		if err := overwriteKeys(server.addr, payloads, loadClients); err != nil {
			return serverResult{}, err
		}
	}

	time.Sleep(200 * time.Millisecond)

	afterWrite, err := server.RSS()
	if err != nil {
		return serverResult{}, err
	}

	fmt.Printf("mixed warmup %s then measure %s with 1 client\n", cfg.warmup, cfg.duration)
	one, err := measureMixed(server.addr, payloads, 1, cfg.warmup, cfg.duration)
	if err != nil {
		return serverResult{}, err
	}

	fmt.Printf("mixed warmup %s then measure %s with %d clients\n", cfg.warmup, cfg.duration, cfg.clients)
	many, err := measureMixed(server.addr, payloads, cfg.clients, cfg.warmup, cfg.duration)
	if err != nil {
		return serverResult{}, err
	}

	fmt.Printf("pipelined 16, warmup %s then measure %s with %d clients\n", cfg.warmup, cfg.duration, cfg.clients)
	pipe, err := measurePipelined(server.addr, payloads, cfg.clients, 16, cfg.warmup, cfg.duration)
	if err != nil {
		return serverResult{}, err
	}

	if one.Errors > 0 || many.Errors > 0 || pipe.Errors > 0 {
		return serverResult{}, fmt.Errorf("protocol errors: one=%d many=%d pipe=%d", one.Errors, many.Errors, pipe.Errors)
	}

	return serverResult{
		Name: name,
		Memory: memoryResult{
			Baseline:   baseline,
			AfterLoad:  afterLoad,
			AfterWrite: afterWrite,
			Keys:       cfg.keys,
			Rounds:     cfg.rounds,
		},
		One:  one,
		Many: many,
		Pipe: pipe,
	}, nil
}

func printReport(kairo, redis serverResult) {
	fmt.Println()
	fmt.Println("| metric | Kairo | Redis |")
	fmt.Println("| --- | ---: | ---: |")
	fmt.Printf("| ops/s, 1 client | %s | %s |\n", formatOps(kairo.One), formatOps(redis.One))
	fmt.Printf("| p50, 1 client | %s | %s |\n", kairo.One.P50.Round(time.Microsecond), redis.One.P50.Round(time.Microsecond))
	fmt.Printf("| p99, 1 client | %s | %s |\n", kairo.One.P99.Round(time.Microsecond), redis.One.P99.Round(time.Microsecond))
	fmt.Printf("| ops/s, %d clients | %s | %s |\n", kairo.Many.Clients, formatOps(kairo.Many), formatOps(redis.Many))
	fmt.Printf("| p50, %d clients | %s | %s |\n", kairo.Many.Clients, kairo.Many.P50.Round(time.Microsecond), redis.Many.P50.Round(time.Microsecond))
	fmt.Printf("| p99, %d clients | %s | %s |\n", kairo.Many.Clients, kairo.Many.P99.Round(time.Microsecond), redis.Many.P99.Round(time.Microsecond))
	fmt.Printf("| ops/s, %d clients, pipeline 16 | %s | %s |\n", kairo.Pipe.Clients, formatOps(kairo.Pipe), formatOps(redis.Pipe))
	fmt.Printf("| p99 batch, pipeline 16 | %s | %s |\n", kairo.Pipe.P99.Round(time.Microsecond), redis.Pipe.P99.Round(time.Microsecond))
	fmt.Printf("| RSS after load | %s | %s |\n", formatMiB(kairo.Memory.AfterLoad), formatMiB(redis.Memory.AfterLoad))
	fmt.Printf("| bytes/key after load | %.0f | %.0f |\n", kairo.Memory.BytesPerKey(), redis.Memory.BytesPerKey())
	fmt.Printf("| RSS after %d overwrites | %s | %s |\n", kairo.Memory.Rounds, formatMiB(kairo.Memory.AfterWrite), formatMiB(redis.Memory.AfterWrite))
	fmt.Printf("| RSS growth from overwrites | %s | %s |\n", formatSignedMiB(kairo.Memory.OverwriteGrowth()), formatSignedMiB(redis.Memory.OverwriteGrowth()))
	fmt.Printf("| GET misses, 1 client | %d | %d |\n", kairo.One.Misses, redis.One.Misses)
	fmt.Printf("| GET misses, %d clients | %d | %d |\n", kairo.Many.Clients, kairo.Many.Misses, redis.Many.Misses)
}

func formatOps(r latencyResult) string {
	return fmt.Sprintf("%.0f", r.OpsPerSec())
}

func formatMiB(bytes uint64) string {
	return fmt.Sprintf("%.1f MiB", float64(bytes)/(1024*1024))
}

func formatSignedMiB(bytes int64) string {
	sign := ""
	if bytes > 0 {
		sign = "+"
	}

	return fmt.Sprintf("%s%.1f MiB", sign, float64(bytes)/(1024*1024))
}

func goVersion() string {
	out, err := exec.Command("go", "version").Output()
	if err != nil {
		return runtime.Version()
	}

	fields := strings.Fields(string(out))
	if len(fields) >= 3 {
		return fields[2]
	}

	return runtime.Version()
}

func redisVersion() string {
	out, err := exec.Command("redis-server", "--version").Output()
	if err != nil {
		return "unknown"
	}

	fields := strings.Fields(string(out))
	if len(fields) >= 3 {
		return strings.TrimPrefix(fields[2], "v=")
	}

	return strings.TrimSpace(string(out))
}
