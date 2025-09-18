package modecache

import (
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// SnakeStore represents a concurrent SnakeStore for testing
type SnakeStore interface {
	Set(int64, string)
	Get(int64) string
}

func BenchmarkLockUnlock(b *testing.B) {
	b.Run("mutex", func(b *testing.B) {
		var mu sync.Mutex
		for i := 0; i < b.N; i++ {
			mu.Lock()
			mu.Unlock()
		}
	})

	b.Run("smutex", func(b *testing.B) {
		var mu Mutex128
		for i := 0; i < b.N; i++ {
			mu.Lock(uint(i % Mutex128Shards))
			mu.Unlock(uint(i % Mutex128Shards))
		}
	})
}

func TestMutex(t *testing.T) {
	var mu Mutex128
	var wg sync.WaitGroup
	var resource, out string

	// Acquire a write lock
	mu.Lock(1)

	// Concurrently, start a reader
	wg.Add(1)
	go func() {
		mu.Lock(1)
		defer mu.Unlock(1)
		out = resource
		wg.Done()
	}()

	// Write the resource
	resource = "hello"
	mu.Unlock(1)

	// Wait for the reader to finish
	wg.Wait()
	assert.Equal(t, "hello", out)
}

// --------------------------- Locked Map ----------------------------

const work = 1000

// An implementation of a locked map using a mutex
type lockedMap struct {
	mu   sync.RWMutex
	data map[int64]string
}

func newLocked() *lockedMap {
	return &lockedMap{data: make(map[int64]string)}
}

// Set sets the value into a locked map
func (l *lockedMap) Set(k int64, v string) {
	l.mu.Lock()
	for i := 0; i < work; i++ {
		l.data[k] = v
	}
	runtime.Gosched()
	for i := 0; i < work; i++ {
		l.data[k] = v
	}
	l.mu.Unlock()
}

// Get gets a value from a locked map
func (l *lockedMap) Get(k int64) (v string) {
	l.mu.RLock()
	for i := 0; i < work; i++ {
		v, _ = l.data[k]
	}
	runtime.Gosched()
	for i := 0; i < work; i++ {
		v, _ = l.data[k]
	}
	l.mu.RUnlock()
	return
}

// --------------------------- Sharded Map ----------------------------

// An implementation of a locked map using a smutex
type shardedMap struct {
	mu   Mutex128
	data []map[int64]string
}

func newSharded() *shardedMap {
	m := &shardedMap{}
	for i := 0; i < Mutex128Shards; i++ {
		m.data = append(m.data, map[int64]string{})
	}
	return m
}

// Set sets the value into a locked map
func (l *shardedMap) Set(k int64, v string) {
	l.mu.Lock(uint(k))
	for i := 0; i < work; i++ {
		l.data[k%Mutex128Shards][k] = v
	}
	runtime.Gosched()
	for i := 0; i < work; i++ {
		l.data[k%Mutex128Shards][k] = v
	}
	l.mu.Unlock(uint(k))
}

// Get gets a value from a locked map
func (l *shardedMap) Get(k int64) (v string) {
	l.mu.Lock(uint(k))
	for i := 0; i < work; i++ {
		v, _ = l.data[k%Mutex128Shards][k]
	}
	runtime.Gosched()
	for i := 0; i < work; i++ {
		v, _ = l.data[k%Mutex128Shards][k]
	}
	l.mu.Unlock(uint(k))
	return
}
