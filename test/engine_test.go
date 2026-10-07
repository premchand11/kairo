package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/premchandpanku/kairo/internal/engine"
)

func TestEngineAutomaticallyExpiresKey(t *testing.T) {
	e := engine.New()
	defer e.Close()

	e.Set("foo", "bar", 100*time.Millisecond)

	entry, ok := e.Get("foo")

	if !ok {
		t.Fatal("expected key to exist")
	}

	if entry.Value != "bar" {
		t.Fatalf("expected bar, got %s", entry.Value)
	}

	time.Sleep(1200 * time.Millisecond)

	_, ok = e.Get("foo")

	if ok {
		t.Fatal("expected key to be expired")
	}
}

func TestEngineTTLWorkerCleansExpiredKey(t *testing.T) {
	e := engine.New()
	defer e.Close()

	e.Set("foo", "bar", 100*time.Millisecond)

	time.Sleep(150 * time.Millisecond)

	if _, ok := e.Get("foo"); ok {
		t.Fatal("expected key to be logically expired")
	}

	time.Sleep(2 * time.Second)

	if e.Resident("foo") {
		t.Fatal("expected background worker to physically remove the key")
	}
}

func TestEngineConcurrentTTL(t *testing.T) {
	e := engine.New()
	defer e.Close()

	const goroutines = 20
	const operations = 500

	done := make(chan struct{}, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer func() {
				done <- struct{}{}
			}()

			for j := 0; j < operations; j++ {
				key := fmt.Sprintf("key:%d:%d", id, j)

				e.Set(
					key,
					"hello",
					time.Duration(100+j%5*100)*time.Millisecond,
				)

				_, _ = e.Get(key)
			}
		}(i)
	}

	for i := 0; i < goroutines; i++ {
		<-done
	}
}

func TestSetAndGet(t *testing.T) {
	e := engine.New()
	defer e.Close()

	e.Set("name", "Prem", 0)

	entry, ok := e.Get("name")

	if !ok {
		t.Fatal("expected key to exist")
	}

	if entry.Value != "Prem" {
		t.Fatalf("expected Prem, got %s", entry.Value)
	}
}

func TestSetWithTTL(t *testing.T) {
	e := engine.New()
	defer e.Close()

	e.Set("name", "Prem", 100*time.Millisecond)

	entry, ok := e.Get("name")

	if !ok {
		t.Fatal("expected key to exist")
	}

	if entry.Value != "Prem" {
		t.Fatalf("expected Prem, got %s", entry.Value)
	}

	time.Sleep(150 * time.Millisecond)

	_, ok = e.Get("name")

	if ok {
		t.Fatal("expected key to expire")
	}
}

func TestDelete(t *testing.T) {
	e := engine.New()
	defer e.Close()

	e.Set("name", "Prem", 0)

	deleted := e.Delete("name")

	if !deleted {
		t.Fatal("expected key to be deleted")
	}

	_, ok := e.Get("name")

	if ok {
		t.Fatal("expected key to not exist")
	}
}

func TestExpire(t *testing.T) {
	e := engine.New()
	defer e.Close()

	e.Set("name", "Prem", 0)

	ok := e.Expire("name", time.Second)

	if !ok {
		t.Fatal("expected EXPIRE to succeed")
	}

	ttl := e.TTL("name")

	if ttl < 0 {
		t.Fatalf("expected positive TTL, got %d", ttl)
	}
}

func TestExpireNonexistentKey(t *testing.T) {
	e := engine.New()
	defer e.Close()

	ok := e.Expire("missing", time.Second)

	if ok {
		t.Fatal("expected EXPIRE to fail for nonexistent key")
	}
}

func TestTTLWithoutExpiration(t *testing.T) {
	e := engine.New()
	defer e.Close()

	e.Set("name", "Prem", 0)

	ttl := e.TTL("name")

	if ttl != -1 {
		t.Fatalf("expected TTL -1, got %d", ttl)
	}
}

func TestTTLNonexistentKey(t *testing.T) {
	e := engine.New()
	defer e.Close()

	ttl := e.TTL("missing")

	if ttl != -2 {
		t.Fatalf("expected TTL -2, got %d", ttl)
	}
}

func TestExpireAndGet(t *testing.T) {
	e := engine.New()
	defer e.Close()

	e.Set("name", "Prem", 100*time.Millisecond)

	_, ok := e.Get("name")

	if !ok {
		t.Fatal("expected key to exist")
	}

	time.Sleep(150 * time.Millisecond)

	_, ok = e.Get("name")

	if ok {
		t.Fatal("expected key to be expired")
	}
}
