// Package store is the key value map behind a small interface.
//
// The server depends on the interface only, so the locking strategy can
// change without touching any network code.
package store

import "sync"

// Store is everything the server is allowed to know about storage.
type Store interface {
	Get(key string) (string, bool)
	Set(key, value string)
	Delete(key string) bool
}

// MemoryStore guards a plain map with one RWMutex, reads run concurrently
// and only writes serialize. Sharding would cut write contention but is
// not worth the complexity until a benchmark says this lock is hot.
//
// The benchmark now exists and says it is cold. A read costs about 27ns
// here against roughly 8.6us of per-operation budget at the throughput
// the server actually reaches, so under 0.4% of the cost is in this
// file. See bench/RESULTS.md. Sharding stays unbuilt on purpose.
type MemoryStore struct {
	mu   sync.RWMutex
	data map[string]string
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: make(map[string]string)}
}

func (s *MemoryStore) Get(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[key]
	return v, ok
}

func (s *MemoryStore) Set(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
}

// Delete reports whether the key was present, the protocol layer turns
// that into the 1 or 0 the client sees.
func (s *MemoryStore) Delete(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.data[key]
	if ok {
		delete(s.data, key)
	}
	return ok
}
