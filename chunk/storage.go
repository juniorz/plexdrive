package chunk

import (
	lru "github.com/hashicorp/golang-lru"

	. "github.com/claudetech/loggo/default"
)

// Storage is a chunk storage
type Storage struct {
	cache *lru.Cache
}

// NewStorage creates a new storage
func NewStorage(chunkSize int64, maxChunks int) *Storage {
	cache, err := lru.NewWithEvict(maxChunks, func(key interface{}, value interface{}) {
		Log.Debugf("Deleted chunk %v", key)
	})

	if err != nil {
		panic(err)
	}

	storage := Storage{
		cache: cache,
	}

	return &storage
}

// Clear removes all old chunks on disk (will be called on each program start)
func (s *Storage) Clear() error {
	s.cache.Purge()
	return nil
}

// Load a chunk from ram or creates it
func (s *Storage) Load(id string) []byte {
	ret, found := s.cache.Get(id)
	if !found {
		return nil
	}

	return ret.([]byte)
}

// Store stores a chunk in the RAM and adds it to the disk storage queue
func (s *Storage) Store(id string, bytes []byte) error {
	s.cache.Add(id, bytes)
	return nil
}
