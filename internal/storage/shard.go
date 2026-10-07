package storage

import (
	"sync"
	"sync/atomic"
	"time"
)

type wheelSlot struct {
	Key       string
	ExpiresAt int64
}

type Shard struct {
	mu       sync.RWMutex
	data     map[string]Entry
	wheel    [wheelBuckets][]wheelSlot
	maxBytes int64
	used     *atomic.Int64
}

func NewShard() *Shard {
	return &Shard{
		data: make(map[string]Entry),
	}
}

func bucketIndex(expiresAt int64) int {
	second := (expiresAt + int64(time.Second) - 1) / int64(time.Second)
	return int(second % wheelBuckets)
}

func (s *Shard) Get(key string) (Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.data[key]
	if !ok {
		return Entry{}, false
	}

	if entry.ExpiresAt > 0 && entry.ExpiresAt < time.Now().UnixNano() {
		return Entry{}, false
	}

	return entry, true
}

func (s *Shard) Set(key string, incoming Entry) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	old, exists := s.data[key]
	delta := int64(len(incoming.Value))
	if exists {
		delta = int64(len(incoming.Value) - len(old.Value))
	} else {
		delta += int64(len(key))
	}
	if !s.reserve(delta) {
		return false
	}

	if exists && incoming.ExpiresAt > 0 && s.sameBucket(key, old, incoming.ExpiresAt) {
		b := int(old.wheelBucket)
		i := int(old.wheelIndex)
		s.wheel[b][i].ExpiresAt = incoming.ExpiresAt
		incoming.wheelBucket = old.wheelBucket
		incoming.wheelIndex = old.wheelIndex
		s.data[key] = incoming
		return true
	}

	if exists {
		s.unscheduleLocked(key, old)
	}

	incoming.wheelBucket = noWheel
	incoming.wheelIndex = noWheel
	if incoming.ExpiresAt > 0 {
		incoming = s.scheduleLocked(key, incoming)
	}

	s.data[key] = incoming
	return true
}

func (s *Shard) Delete(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.data[key]
	if !ok {
		return false
	}

	s.unscheduleLocked(key, entry)
	s.reserve(-int64(len(key) + len(entry.Value)))
	delete(s.data, key)
	return true
}

func (s *Shard) SetExpiration(key string, expiresAt int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.data[key]
	if !ok {
		return false
	}

	if expiresAt > 0 && s.sameBucket(key, entry, expiresAt) {
		s.wheel[entry.wheelBucket][entry.wheelIndex].ExpiresAt = expiresAt
		entry.ExpiresAt = expiresAt
		s.data[key] = entry
		return true
	}

	s.unscheduleLocked(key, entry)
	entry.ExpiresAt = expiresAt
	entry.wheelBucket = noWheel
	entry.wheelIndex = noWheel
	if expiresAt > 0 {
		entry = s.scheduleLocked(key, entry)
	}

	s.data[key] = entry
	return true
}

func (s *Shard) Exists(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, ok := s.data[key]
	return ok
}

func (s *Shard) expire(bucketSecond int64, now int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	b := int(bucketSecond % wheelBuckets)
	if b < 0 {
		b += wheelBuckets
	}

	bucket := s.wheel[b]
	s.wheel[b] = nil

	for _, slot := range bucket {
		entry, ok := s.data[slot.Key]
		if !ok || entry.ExpiresAt != slot.ExpiresAt || int(entry.wheelBucket) != b {
			continue
		}

		if slot.ExpiresAt > now {
			entry.wheelBucket = int32(b)
			entry.wheelIndex = int32(len(s.wheel[b]))
			s.wheel[b] = append(s.wheel[b], slot)
			s.data[slot.Key] = entry
			continue
		}

		s.reserve(-int64(len(slot.Key) + len(entry.Value)))
		delete(s.data, slot.Key)
	}
}

func (s *Shard) slots(key string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	n := 0
	for _, bucket := range s.wheel {
		for _, slot := range bucket {
			if slot.Key == key {
				n++
			}
		}
	}

	return n
}

func (s *Shard) reserve(delta int64) bool {
	if delta == 0 {
		return true
	}

	if delta < 0 || s.maxBytes <= 0 || s.used == nil {
		if s.used != nil {
			s.used.Add(delta)
		}
		return true
	}

	for {
		current := s.used.Load()
		if current+delta > s.maxBytes {
			return false
		}
		if s.used.CompareAndSwap(current, current+delta) {
			return true
		}
	}
}

func (s *Shard) sameBucket(key string, entry Entry, expiresAt int64) bool {
	if entry.wheelBucket < 0 || entry.wheelIndex < 0 {
		return false
	}

	b := int(entry.wheelBucket)
	i := int(entry.wheelIndex)
	if bucketIndex(expiresAt) != b || b >= wheelBuckets || i >= len(s.wheel[b]) {
		return false
	}

	return s.wheel[b][i].Key == key
}

func (s *Shard) scheduleLocked(key string, entry Entry) Entry {
	b := bucketIndex(entry.ExpiresAt)
	entry.wheelBucket = int32(b)
	entry.wheelIndex = int32(len(s.wheel[b]))
	s.wheel[b] = append(s.wheel[b], wheelSlot{
		Key:       key,
		ExpiresAt: entry.ExpiresAt,
	})
	return entry
}

func (s *Shard) unscheduleLocked(key string, entry Entry) {
	if entry.wheelBucket < 0 {
		return
	}

	b := int(entry.wheelBucket)
	i := int(entry.wheelIndex)
	if b < wheelBuckets && i >= 0 && i < len(s.wheel[b]) && s.wheel[b][i].Key == key {
		s.removeSlotLocked(b, i)
		return
	}

	for bi := range s.wheel {
		for si := 0; si < len(s.wheel[bi]); si++ {
			if s.wheel[bi][si].Key != key {
				continue
			}

			s.removeSlotLocked(bi, si)
			si--
		}
	}
}

func (s *Shard) removeSlotLocked(b, i int) {
	bucket := s.wheel[b]
	last := len(bucket) - 1
	if i != last {
		moved := bucket[last]
		bucket[i] = moved
		if ent, ok := s.data[moved.Key]; ok && int(ent.wheelBucket) == b && int(ent.wheelIndex) == last {
			ent.wheelIndex = int32(i)
			s.data[moved.Key] = ent
		}
	}

	s.wheel[b] = bucket[:last]
	if len(s.wheel[b]) == 0 {
		s.wheel[b] = nil
	}
}
