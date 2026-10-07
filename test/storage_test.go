package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/premchandpanku/kairo/internal/storage"
)

func TestStoreSetAndGet(t *testing.T) {
	store := storage.NewStore()

	store.Set("name", storage.Entry{
		Value: "Prem",
	})

	entry, ok := store.Get("name")

	if !ok {
		t.Fatal("expected key to exist")
	}

	if entry.Value != "Prem" {
		t.Fatalf("expected Prem, got %s", entry.Value)
	}
}

func TestStoreDelete(t *testing.T) {
	store := storage.NewStore()

	store.Set("name", storage.Entry{
		Value: "Prem",
	})

	deleted := store.Delete("name")

	if !deleted {
		t.Fatal("expected key to be deleted")
	}

	_, ok := store.Get("name")

	if ok {
		t.Fatal("expected key to not exist")
	}
}

func TestStoreConcurrentAccess(t *testing.T) {
	store := storage.NewStore()

	const goroutines = 100
	const operationsPerGoroutine = 1000

	done := make(chan struct{}, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer func() {
				done <- struct{}{}
			}()

			for j := 0; j < operationsPerGoroutine; j++ {
				key := fmt.Sprintf("key:%d:%d", id, j)

				store.Set(key, storage.Entry{
					Value: "hello",
				})

				entry, ok := store.Get(key)

				if !ok {
					t.Errorf("key %s not found", key)
					return
				}

				if entry.Value != "hello" {
					t.Errorf("unexpected value for %s: %s", key, entry.Value)
					return
				}
			}
		}(i)
	}

	for i := 0; i < goroutines; i++ {
		<-done
	}
}

func TestStoreGetExpired(t *testing.T) {
	store := storage.NewStore()

	store.Set("name", storage.Entry{
		Value:     "Prem",
		ExpiresAt: time.Now().Add(50 * time.Millisecond).UnixNano(),
	})

	entry, ok := store.Get("name")

	if !ok {
		t.Fatal("expected key to exist")
	}

	if entry.Value != "Prem" {
		t.Fatalf("expected Prem, got %s", entry.Value)
	}

	time.Sleep(100 * time.Millisecond)

	_, ok = store.Get("name")

	if ok {
		t.Fatal("expected key to be expired")
	}
}

func TestTTLDoesNotDeleteUpdatedKey(t *testing.T) {
	store := storage.NewStore()

	firstExpiry := time.Now().Add(50 * time.Millisecond).UnixNano()

	store.Set("foo", storage.Entry{
		Value:     "old",
		ExpiresAt: firstExpiry,
	})

	secondExpiry := time.Now().Add(time.Second).UnixNano()

	store.Set("foo", storage.Entry{
		Value:     "new",
		ExpiresAt: secondExpiry,
	})

	store.Expire(firstExpiry/int64(time.Second), firstExpiry)

	entry, ok := store.Get("foo")

	if !ok {
		t.Fatal("foo should still exist")
	}

	if entry.Value != "new" {
		t.Fatalf("expected new, got %s", entry.Value)
	}

	if slots := store.CountWheelSlots("foo"); slots != 1 {
		t.Fatalf("expected 1 wheel slot, got %d", slots)
	}
}

func TestManyRefreshesStayOneSlot(t *testing.T) {
	store := storage.NewStore()

	const keyCount = 1000

	for round := 0; round < 10; round++ {
		expiresAt := time.Now().Add(10 * time.Minute).UnixNano()

		for i := 0; i < keyCount; i++ {
			store.Set(fmt.Sprintf("k:%07d", i), storage.Entry{
				Value:     "v",
				ExpiresAt: expiresAt,
			})
		}
	}

	for i := 0; i < keyCount; i++ {
		key := fmt.Sprintf("k:%07d", i)
		if slots := store.CountWheelSlots(key); slots != 1 {
			t.Fatalf("%s has %d wheel slots", key, slots)
		}
	}
}

func TestRefreshKeepsOneWheelSlot(t *testing.T) {
	store := storage.NewStoreWithShards(1)

	for i := 0; i < 10; i++ {
		store.Set("foo", storage.Entry{
			Value:     "v",
			ExpiresAt: time.Now().Add(time.Duration(30+i) * time.Second).UnixNano(),
		})
	}

	if slots := store.CountWheelSlots("foo"); slots != 1 {
		t.Fatalf("expected 1 wheel slot, got %d", slots)
	}

	store.Set("foo", storage.Entry{Value: "v"})

	if slots := store.CountWheelSlots("foo"); slots != 0 {
		t.Fatalf("expected wheel slot to be cleared, got %d", slots)
	}
}

func TestDeleteRepairsWheelIndex(t *testing.T) {
	store := storage.NewStoreWithShards(1)
	expiry := time.Now().Add(time.Hour).UnixNano()

	store.Set("a", storage.Entry{Value: "a", ExpiresAt: expiry})
	store.Set("b", storage.Entry{Value: "b", ExpiresAt: expiry})

	if !store.Delete("a") {
		t.Fatal("expected a to be deleted")
	}

	if slots := store.CountWheelSlots("b"); slots != 1 {
		t.Fatalf("expected 1 wheel slot for b, got %d", slots)
	}

	store.Expire((expiry+int64(time.Second)-1)/int64(time.Second), expiry)

	if store.Exists("b") {
		t.Fatal("expected b to be physically removed")
	}
}

func TestExpireBucketRequeuesFutureKey(t *testing.T) {
	store := storage.NewStoreWithShards(1)
	expiresAt := time.Now().Add(90 * time.Second).UnixNano()

	store.Set("foo", storage.Entry{Value: "v", ExpiresAt: expiresAt})

	second := (expiresAt + int64(time.Second) - 1) / int64(time.Second)
	store.Expire(second-60, expiresAt-int64(60)*int64(time.Second))

	if _, ok := store.Get("foo"); !ok {
		t.Fatal("expected future key to stay")
	}

	if slots := store.CountWheelSlots("foo"); slots != 1 {
		t.Fatalf("expected 1 wheel slot after requeue, got %d", slots)
	}

	store.Expire(second, expiresAt)

	if store.Exists("foo") {
		t.Fatal("expected key to be removed once it is due")
	}
}

func TestMaxMemoryRejectsNewKey(t *testing.T) {
	store := storage.NewStore()
	store.SetMaxMemory(10)

	if !store.Set("a", storage.Entry{Value: "12345"}) {
		t.Fatal("expected first key to fit")
	}

	if store.Used() != 6 {
		t.Fatalf("used = %d, want 6", store.Used())
	}

	if store.Set("b", storage.Entry{Value: "12345"}) {
		t.Fatal("expected second key to be rejected")
	}

	if _, ok := store.Get("b"); ok {
		t.Fatal("rejected key should not be stored")
	}

	if !store.Set("a", storage.Entry{Value: "12"}) {
		t.Fatal("expected a smaller update to fit")
	}

	if store.Used() != 3 {
		t.Fatalf("used = %d, want 3", store.Used())
	}

	if !store.Delete("a") {
		t.Fatal("expected delete")
	}

	if store.Used() != 0 {
		t.Fatalf("used = %d, want 0", store.Used())
	}
}
