package engine

import (
	"strconv"
	"testing"
	"time"

	"github.com/premchandpanku/kairo/internal/storage"
)

func ceilingSecond(expiresAt int64) int64 {
	return (expiresAt + int64(time.Second) - 1) / int64(time.Second)
}

func BenchmarkEngineSetWithoutTTL(b *testing.B) {
	e := New()
	defer e.Close()

	const keyCount = 1000

	keys := make([]string, keyCount)
	for i := 0; i < keyCount; i++ {
		keys[i] = "key:" + strconv.Itoa(i)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		e.Set(keys[i%keyCount], "hello", 0)
	}
}

func BenchmarkEngineSetWithTTL(b *testing.B) {
	e := New()
	defer e.Close()

	const keyCount = 1000

	keys := make([]string, keyCount)
	for i := 0; i < keyCount; i++ {
		keys[i] = "key:" + strconv.Itoa(i)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		e.Set(keys[i%keyCount], "hello", time.Minute)
	}
}

func BenchmarkEngineExpire100K(b *testing.B) {
	e := New()
	defer e.Close()

	const keyCount = 100_000

	keys := make([]string, keyCount)

	for i := 0; i < keyCount; i++ {
		keys[i] = "key:" + strconv.Itoa(i)
	}

	for i := 0; i < b.N; i++ {
		expiry := time.Now().Add(time.Hour).UnixNano()

		b.StopTimer()

		for _, key := range keys {
			e.store.Set(key, storage.Entry{
				Value:     "hello",
				ExpiresAt: expiry,
			})
		}

		b.StartTimer()

		e.expire(ceilingSecond(expiry), expiry)
	}
}

func BenchmarkEngineExpireDistributed100K(b *testing.B) {
	e := New()
	defer e.Close()

	const keyCount = 100_000
	const bucketCount = 60

	keys := make([]string, keyCount)

	for i := 0; i < keyCount; i++ {
		keys[i] = "key:" + strconv.Itoa(i)
	}

	for i := 0; i < b.N; i++ {
		b.StopTimer()

		now := time.Now()

		for keyIndex, key := range keys {
			offset := time.Duration(keyIndex%bucketCount+1) * time.Second
			expiry := now.Add(offset).UnixNano()

			e.store.Set(key, storage.Entry{
				Value:     "hello",
				ExpiresAt: expiry,
			})
		}

		b.StartTimer()

		// Expire one second's worth of keys.
		expiry := now.Add(time.Second).UnixNano()

		e.expire(ceilingSecond(expiry), expiry)
	}
}

func BenchmarkEngineExpireBucket(b *testing.B) {
	const keyCount = 1667

	e := New()
	defer e.Close()

	keys := make([]string, keyCount)

	for i := range keys {
		keys[i] = "key:" + strconv.Itoa(i)
	}

	for i := 0; i < b.N; i++ {
		b.StopTimer()

		expiry := time.Now().Add(time.Hour).UnixNano()

		for _, key := range keys {
			e.store.Set(key, storage.Entry{
				Value:     "hello",
				ExpiresAt: expiry,
			})
		}

		b.StartTimer()

		e.expire(ceilingSecond(expiry), expiry)
	}
}
