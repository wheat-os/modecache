package modecache

import (
	"context"
	"sync"

	"golang.org/x/sync/singleflight"
)

const Mutex128Shards = 128

type Mutex128 struct {
	mu [Mutex128Shards]sync.Mutex
}

// Lock locks rw for writing. If the lock is already locked for reading or writing,
// then Lock blocks until the lock is available.
func (rw *Mutex128) Lock(shard uint) {
	rw.mu[shard%Mutex128Shards].Lock()
}

// Unlock unlocks rw for writing. It is a run-time error if rw is not locked for
// writing on entry to Unlock.
func (rw *Mutex128) Unlock(shard uint) {
	rw.mu[shard%Mutex128Shards].Unlock()
}

func (rw *Mutex128) TryLock(shard uint) bool {
	return rw.mu[shard%Mutex128Shards].TryLock()
}

type SingleflightGroup struct {
	singleflight.Group
}

// Do 影子链路支持
func (s *SingleflightGroup) Do(ctx context.Context, key string, fn func() (interface{}, error)) (v interface{}, err error, shared bool) {
	return s.Group.Do(key, fn)
}
