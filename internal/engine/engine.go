package engine

import (
	"time"

	"github.com/premchandpanku/kairo/internal/storage"
)

type Engine struct {
	store     *storage.Store
	ttlWorker *TTLWorker
}

func New() *Engine {
	return NewWithShards(0)
}

// NewWithShards builds an engine with n map shards.
// Zero uses the default, 16.
func NewWithShards(n int) *Engine {
	var store *storage.Store
	if n <= 0 {
		store = storage.NewStore()
	} else {
		store = storage.NewStoreWithShards(n)
	}

	engine := &Engine{
		store: store,
	}

	engine.ttlWorker = NewTTLWorker(engine)
	engine.ttlWorker.Start()

	return engine
}

func (e *Engine) Set(key string, value string, ttl time.Duration) bool {
	expiresAt := int64(0)

	if ttl > 0 {
		expiresAt = time.Now().Add(ttl).UnixNano()
	}

	return e.store.Set(key, storage.Entry{
		Value:     value,
		ExpiresAt: expiresAt,
	})
}

// SetMaxMemory limits the sum of key and value lengths. Zero means unlimited.
func (e *Engine) SetMaxMemory(n int64) {
	e.store.SetMaxMemory(n)
}

func (e *Engine) Get(key string) (storage.Entry, bool) {
	return e.store.Get(key)
}

func (e *Engine) expire(bucketSecond int64, now int64) {
	e.store.Expire(bucketSecond, now)
}

func (e *Engine) Close() {
	e.ttlWorker.Stop()
}

func (e *Engine) Delete(key string) bool {
	return e.store.Delete(key)
}

// Resident reports whether the key is still stored.
// Get hides a key after its deadline; Resident stays true until the expiry worker deletes it.
func (e *Engine) Resident(key string) bool {
	return e.store.Exists(key)
}

func (e *Engine) Expire(key string, ttl time.Duration) bool {
	expiresAt := time.Now().Add(ttl).UnixNano()

	return e.store.SetExpiration(key, expiresAt)
}

func (e *Engine) TTL(key string) int64 {
	entry, ok := e.store.Get(key)

	if !ok {
		return -2
	}

	if entry.ExpiresAt == 0 {
		return -1
	}

	remaining := entry.ExpiresAt - time.Now().UnixNano()

	if remaining <= 0 {
		return -2
	}

	return int64((remaining + int64(time.Second) - 1) / int64(time.Second))
}
