package main

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type latencyResult struct {
	Clients int
	Ops     int64
	Misses  int64
	Errors  int64
	Elapsed time.Duration
	P50     time.Duration
	P99     time.Duration
}

func (r latencyResult) OpsPerSec() float64 {
	if r.Elapsed <= 0 {
		return 0
	}

	return float64(r.Ops) / r.Elapsed.Seconds()
}

type payloads struct {
	sets [][]byte
	gets [][]byte
}

func buildPayloads(keyCount int, value string, ttlSeconds int) payloads {
	sets := make([][]byte, keyCount)
	gets := make([][]byte, keyCount)
	ttl := fmt.Sprintf("%d", ttlSeconds)

	for i := 0; i < keyCount; i++ {
		key := fmt.Sprintf("k:%07d", i)
		sets[i] = encodeArray("SET", key, value, "EX", ttl)
		gets[i] = encodeArray("GET", key)
	}

	return payloads{sets: sets, gets: gets}
}

func runRange(addr string, clients int, fn func(worker int, c *client) error) error {
	if clients < 1 {
		return fmt.Errorf("client count must be positive")
	}

	errCh := make(chan error, clients)
	var wg sync.WaitGroup

	for worker := 0; worker < clients; worker++ {
		wg.Add(1)

		go func(worker int) {
			defer wg.Done()

			c, err := dial(addr)
			if err != nil {
				errCh <- err
				return
			}
			defer c.Close()

			if err := fn(worker, c); err != nil {
				errCh <- err
			}
		}(worker)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			return err
		}
	}

	return nil
}

func loadKeys(addr string, p payloads, clients int) error {
	keyCount := len(p.sets)
	clients = clampClients(clients, keyCount)

	return runRange(addr, clients, func(worker int, c *client) error {
		for i := worker; i < keyCount; i += clients {
			if _, err := c.Do(p.sets[i]); err != nil {
				return err
			}
		}

		return nil
	})
}

func overwriteKeys(addr string, p payloads, clients int) error {
	return loadKeys(addr, p, clients)
}

func measureMixed(addr string, p payloads, clients int, warmup, duration time.Duration) (latencyResult, error) {
	clients = clampClients(clients, len(p.sets))

	conns := make([]*client, clients)
	for i := 0; i < clients; i++ {
		c, err := dial(addr)
		if err != nil {
			closeClients(conns[:i])
			return latencyResult{}, err
		}

		conns[i] = c
	}
	defer closeClients(conns)

	if err := runMixed(conns, p, time.Now().Add(warmup), 1, false, nil, nil, nil, nil); err != nil {
		return latencyResult{}, err
	}

	var ops atomic.Int64
	var misses atomic.Int64
	var errors atomic.Int64
	samples := make([][]time.Duration, clients)

	start := time.Now()
	err := runMixed(conns, p, start.Add(duration), 1, true, &ops, &misses, &errors, samples)
	elapsed := time.Since(start)
	if err != nil {
		return latencyResult{}, err
	}

	merged := make([]time.Duration, 0, int(ops.Load()))
	for _, sample := range samples {
		merged = append(merged, sample...)
	}

	sort.Slice(merged, func(i, j int) bool { return merged[i] < merged[j] })

	return latencyResult{
		Clients: clients,
		Ops:     ops.Load(),
		Misses:  misses.Load(),
		Errors:  errors.Load(),
		Elapsed: elapsed,
		P50:     nearestRank(merged, 50),
		P99:     nearestRank(merged, 99),
	}, nil
}

func measurePipelined(addr string, p payloads, clients, depth int, warmup, duration time.Duration) (latencyResult, error) {
	clients = clampClients(clients, len(p.sets))

	conns := make([]*client, clients)
	for i := 0; i < clients; i++ {
		c, err := dial(addr)
		if err != nil {
			closeClients(conns[:i])
			return latencyResult{}, err
		}
		conns[i] = c
	}
	defer closeClients(conns)

	if err := runMixed(conns, p, time.Now().Add(warmup), depth, false, nil, nil, nil, nil); err != nil {
		return latencyResult{}, err
	}

	var ops atomic.Int64
	var misses atomic.Int64
	var errors atomic.Int64
	samples := make([][]time.Duration, clients)

	start := time.Now()
	err := runMixed(conns, p, start.Add(duration), depth, true, &ops, &misses, &errors, samples)
	elapsed := time.Since(start)
	if err != nil {
		return latencyResult{}, err
	}

	merged := make([]time.Duration, 0, int(ops.Load()))
	for _, sample := range samples {
		merged = append(merged, sample...)
	}
	sort.Slice(merged, func(i, j int) bool { return merged[i] < merged[j] })

	return latencyResult{
		Clients: clients,
		Ops:     ops.Load(),
		Misses:  misses.Load(),
		Errors:  errors.Load(),
		Elapsed: elapsed,
		P50:     nearestRank(merged, 50),
		P99:     nearestRank(merged, 99),
	}, nil
}

func runMixed(
	conns []*client,
	p payloads,
	deadline time.Time,
	depth int,
	record bool,
	ops *atomic.Int64,
	misses *atomic.Int64,
	errors *atomic.Int64,
	samples [][]time.Duration,
) error {
	errCh := make(chan error, len(conns))
	var wg sync.WaitGroup

	for worker := range conns {
		wg.Add(1)

		go func(worker int) {
			defer wg.Done()

			rng := rand.New(rand.NewPCG(uint64(worker+1), uint64(len(p.sets))))
			c := conns[worker]

			batch := make([][]byte, depth)
			gets := make([]bool, depth)

			for time.Now().Before(deadline) {
				n := depth
				if n < 1 {
					n = 1
				}

				for i := 0; i < n; i++ {
					index := rng.IntN(len(p.sets))
					gets[i] = rng.IntN(2) == 0
					if gets[i] {
						batch[i] = p.gets[index]
					} else {
						batch[i] = p.sets[index]
					}
				}

				began := time.Now()
				var kinds []replyKind
				var err error
				if n == 1 {
					var kind replyKind
					kind, err = c.Do(batch[0])
					kinds = []replyKind{kind}
				} else {
					kinds, err = c.DoMany(batch[:n])
				}
				took := time.Since(began)

				if err != nil {
					if errors != nil {
						errors.Add(1)
					}
					errCh <- err
					return
				}

				if !record {
					continue
				}

				ops.Add(int64(n))
				samples[worker] = append(samples[worker], took)
				for i := 0; i < n; i++ {
					if gets[i] && kinds[i] == replyNil {
						misses.Add(1)
					}
				}
			}
		}(worker)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			return err
		}
	}

	return nil
}

func closeClients(conns []*client) {
	for _, c := range conns {
		if c != nil {
			_ = c.Close()
		}
	}
}

func nearestRank(sorted []time.Duration, percentile float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}

	rank := int(math.Ceil((percentile/100)*float64(len(sorted)))) - 1
	if rank < 0 {
		rank = 0
	}

	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}

	return sorted[rank]
}

func clampClients(clients, keyCount int) int {
	if clients < 1 {
		return 1
	}

	if keyCount > 0 && clients > keyCount {
		return keyCount
	}

	return clients
}
