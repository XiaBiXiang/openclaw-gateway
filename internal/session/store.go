package session

import (
	"sync"
	"time"
)

type entry struct {
	Mode      string
	ExpiresAt time.Time
}

type Store struct {
	mu      sync.RWMutex
	entries map[string]entry
}

func NewStore() *Store {
	return &Store{
		entries: make(map[string]entry),
	}
}

func (s *Store) Get(id string) (string, bool) {
	if id == "" {
		return "", false
	}

	s.mu.RLock()
	item, ok := s.entries[id]
	s.mu.RUnlock()
	if !ok {
		return "", false
	}

	if time.Now().After(item.ExpiresAt) {
		s.mu.Lock()
		delete(s.entries, id)
		s.mu.Unlock()
		return "", false
	}

	return item.Mode, true
}

func (s *Store) Put(id, mode string, ttl time.Duration) {
	if id == "" || mode == "" || ttl <= 0 {
		return
	}

	s.mu.Lock()
	s.entries[id] = entry{
		Mode:      mode,
		ExpiresAt: time.Now().Add(ttl),
	}
	s.mu.Unlock()
}
