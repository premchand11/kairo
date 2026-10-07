package storage

import "sync/atomic"

const defaultShardCount = 16

type Store struct {
	shards   []*Shard
	used     atomic.Int64
	maxBytes int64
}

func NewStoreWithShards(shardCount int) *Store {
	if shardCount <= 0 {
		panic("shard count must be greater than zero")
	}

	shards := make([]*Shard, shardCount)

	for i := range shards {
		shards[i] = NewShard()
	}

	store := &Store{
		shards: shards,
	}
	for _, shard := range shards {
		shard.used = &store.used
	}

	return store
}

func NewStore() *Store {
	return NewStoreWithShards(defaultShardCount)
}

func hashKey(key string) uint32 {
	hash := uint32(2166136261)
	for i := 0; i < len(key); i++ {
		hash ^= uint32(key[i])
		hash *= 16777619
	}
	if hash == 0 {
		return 1
	}

	return hash
}

// SetMaxMemory limits the sum of key and value lengths.
// Zero means unlimited. A SET that would pass the limit is rejected.
func (s *Store) SetMaxMemory(n int64) {
	s.maxBytes = n
	for _, shard := range s.shards {
		shard.maxBytes = n
	}
}

// Used reports the sum of key and value lengths currently stored.
func (s *Store) Used() int64 {
	return s.used.Load()
}

func (s *Store) getShard(key string) *Shard {
	index := hashKey(key) % uint32(len(s.shards))

	return s.shards[index]
}

func (s *Store) Get(key string) (Entry, bool) {
	return s.getShard(key).Get(key)
}

func (s *Store) Set(key string, entry Entry) bool {
	return s.getShard(key).Set(key, entry)
}

func (s *Store) Delete(key string) bool {
	return s.getShard(key).Delete(key)
}

func (s *Store) Exists(key string) bool {
	return s.getShard(key).Exists(key)
}

func (s *Store) SetExpiration(key string, expiresAt int64) bool {
	return s.getShard(key).SetExpiration(key, expiresAt)
}

func (s *Store) Expire(bucketSecond int64, now int64) {
	for _, shard := range s.shards {
		shard.expire(bucketSecond, now)
	}
}

func (s *Store) slots(key string) int {
	return s.getShard(key).slots(key)
}

// CountWheelSlots reports how many timing-wheel entries reference key.
func (s *Store) CountWheelSlots(key string) int {
	return s.slots(key)
}
