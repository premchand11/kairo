package storage

import (
	"strconv"
	"testing"
	"time"
)

func BenchmarkStoreSet(b *testing.B) {
	store := NewStore()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		key := "key:" + strconv.Itoa(i)

		store.Set(key, Entry{
			Value: "hello",
		})
	}
}

func BenchmarkStoreGet(b *testing.B) {
	store := NewStore()

	for i := 0; i < 1000; i++ {
		key := "key:" + strconv.Itoa(i)

		store.Set(key, Entry{
			Value: "hello",
		})
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		key := "key:" + strconv.Itoa(i%1000)

		_, _ = store.Get(key)
	}
}

func BenchmarkStoreConcurrentGet(b *testing.B) {
	store := NewStore()

	for i := 0; i < 1000; i++ {
		key := "key:" + strconv.Itoa(i)

		store.Set(key, Entry{
			Value: "hello",
		})
	}

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0

		for pb.Next() {
			key := "key:" + strconv.Itoa(i%1000)

			_, _ = store.Get(key)

			i++
		}
	})
}

func BenchmarkStoreConcurrentSet(b *testing.B) {
	store := NewStore()

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0

		for pb.Next() {
			key := "key:" + strconv.Itoa(i%1000)

			store.Set(key, Entry{
				Value: "hello",
			})

			i++
		}
	})
}

func BenchmarkStoreConcurrentGet1Shard(b *testing.B) {
	store := NewStoreWithShards(1)

	for i := 0; i < 1000; i++ {
		key := "key:" + strconv.Itoa(i)

		store.Set(key, Entry{
			Value: "hello",
		})
	}

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0

		for pb.Next() {
			key := "key:" + strconv.Itoa(i%1000)

			_, _ = store.Get(key)

			i++
		}
	})
}

func BenchmarkStoreConcurrentGet16Shards(b *testing.B) {
	store := NewStoreWithShards(16)

	for i := 0; i < 1000; i++ {
		key := "key:" + strconv.Itoa(i)

		store.Set(key, Entry{
			Value: "hello",
		})
	}

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0

		for pb.Next() {
			key := "key:" + strconv.Itoa(i%1000)

			_, _ = store.Get(key)

			i++
		}
	})
}

func BenchmarkStoreConcurrentGet16ShardsPrecomputed(b *testing.B) {
	store := NewStoreWithShards(16)

	keys := make([]string, 1000)

	for i := range keys {
		keys[i] = "key:" + strconv.Itoa(i)

		store.Set(keys[i], Entry{
			Value: "hello",
		})
	}

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0

		for pb.Next() {
			_, _ = store.Get(keys[i%len(keys)])
			i++
		}
	})
}

func BenchmarkStoreShardCount(b *testing.B) {
	counts := []int{1, 2, 4, 8, 16, 32, 64}

	for _, n := range counts {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			store := NewStoreWithShards(n)
			keys := make([]string, 1000)
			for i := range keys {
				keys[i] = "key:" + strconv.Itoa(i)
				store.Set(keys[i], Entry{Value: "hello"})
			}

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				i := 0
				for pb.Next() {
					_, _ = store.Get(keys[i%len(keys)])
					i++
				}
			})
		})
	}
}

func BenchmarkStoreRefreshTTL(b *testing.B) {
	store := NewStore()

	const keyCount = 1000

	keys := make([]string, keyCount)
	expiry := time.Now().Add(time.Minute).UnixNano()

	for i := 0; i < keyCount; i++ {
		keys[i] = "key:" + strconv.Itoa(i)
		store.Set(keys[i], Entry{
			Value:     "hello",
			ExpiresAt: expiry,
		})
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		store.Set(keys[i%keyCount], Entry{
			Value:     "hello",
			ExpiresAt: expiry,
		})
	}
}
