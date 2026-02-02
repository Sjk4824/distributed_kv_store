package kv

import (
	"fmt"
	"sync"
)

type Store struct {
	mu   sync.RWMutex
	data map[string][]byte
	seen map[string]struct{}
}

// constructor functions
func NewStore() *Store {
	return &Store{
		data: make(map[string][]byte),
		seen: make(map[string]struct{}),
	}
}

// we generate a unique key for each client request to track it in the map.
func dedupeKey(clientID string, reqID uint64) string {
	return fmt.Sprintf("%s:%d", clientID, reqID)
}

func (s *Store) ApplyPut(clientID string, reqId uint64, key string, val []byte) bool {
	opKey := dedupeKey(clientID, reqId)

	s.mu.Lock() //we will lock it during write because at a time only one shud be able to write to avoud race conditions
	defer s.mu.Unlock()

	if _, ok := s.seen[opKey]; ok {
		return false //this request was triggered before
	}

	cpy := make([]byte, len(val))
	copy(cpy, val) //we will copy val to cpy

	s.data[key] = cpy
	s.seen[opKey] = struct{}{}
	return true

}

func (s *Store) Get(key string) ([]byte, bool) {
	s.mu.RLock() //obtain the read lock -> this is shared across multiple go routines
	v, ok := s.data[key]
	s.mu.RUnlock()
	if !ok {
		return nil, false
	}

	cpy := make([]byte, len(v))
	copy(cpy, v)
	return cpy, true
}

func (s *Store) ApplyDelete(clientID string, reqId uint64, key string) bool {
	opKey := dedupeKey(clientID, reqId)
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.seen[opKey]; ok {
		return false //we already have this request thats running ---> Idempotent
	}

	delete(s.data, key)
	s.seen[opKey] = struct{}{}
	return true
}
