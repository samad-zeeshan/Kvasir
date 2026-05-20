// Unit and concurrency tests for the memory store.
//
// The concurrent test exists to fail under go test -race if the locking
// is ever wrong.
package store

import (
	"fmt"
	"sync"
	"testing"
)

func TestSetGet(t *testing.T) {
	s := NewMemoryStore()
	s.Set("name", "alice")
	got, ok := s.Get("name")
	if !ok || got != "alice" {
		t.Fatalf("Get(name) = %q, %v, want alice, true", got, ok)
	}
}

func TestGetMissing(t *testing.T) {
	s := NewMemoryStore()
	if _, ok := s.Get("nope"); ok {
		t.Fatal("Get on a missing key reported ok")
	}
}

func TestSetOverwrites(t *testing.T) {
	s := NewMemoryStore()
	s.Set("k", "one")
	s.Set("k", "two")
	if got, _ := s.Get("k"); got != "two" {
		t.Fatalf("Get(k) = %q, want two", got)
	}
}

func TestDelete(t *testing.T) {
	s := NewMemoryStore()
	s.Set("k", "v")
	if !s.Delete("k") {
		t.Fatal("Delete of an existing key returned false")
	}
	if _, ok := s.Get("k"); ok {
		t.Fatal("key still present after Delete")
	}
	if s.Delete("k") {
		t.Fatal("Delete of a missing key returned true")
	}
}

func TestEmptyValue(t *testing.T) {
	s := NewMemoryStore()
	s.Set("k", "")
	got, ok := s.Get("k")
	if !ok || got != "" {
		t.Fatalf("empty value round trip failed, got %q, %v", got, ok)
	}
}

// Readers, writers, and deleters hammer the same store at once. The real
// assertion is the race detector staying quiet.
func TestConcurrentAccess(t *testing.T) {
	s := NewMemoryStore()
	const goroutines = 16
	const ops = 200

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// Each goroutine owns its key, so its final value is deterministic
			// even while everyone shares the map and the lock.
			key := fmt.Sprintf("key-%d", id)
			for i := 0; i < ops; i++ {
				s.Set(key, fmt.Sprintf("v%d", i))
				s.Get(key)
				s.Get(fmt.Sprintf("key-%d", (id+1)%goroutines))
				if i%50 == 49 {
					s.Delete(key)
				}
			}
			s.Set(key, "final")
		}(g)
	}
	wg.Wait()

	for g := 0; g < goroutines; g++ {
		key := fmt.Sprintf("key-%d", g)
		if got, ok := s.Get(key); !ok || got != "final" {
			t.Fatalf("Get(%s) = %q, %v, want final, true", key, got, ok)
		}
	}
}

func BenchmarkGet(b *testing.B) {
	s := NewMemoryStore()
	s.Set("k", "v")
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			s.Get("k")
		}
	})
}

func BenchmarkSet(b *testing.B) {
	s := NewMemoryStore()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			s.Set("k", "v")
		}
	})
}
