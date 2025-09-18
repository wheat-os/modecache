package modecache

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/spf13/cast"
)

type testSnakeCache struct {
	mp map[string]any
}

func (s testSnakeCache) Get(ctx context.Context, key string) (any, error) {
	if s.mp[key] == nil {
		return nil, ErrKeyNonExistent
	}
	return s.mp[key], nil
}

func (s testSnakeCache) Set(ctx context.Context, key string, data any, ttl time.Duration) error {
	s.mp[key] = data
	return nil
}

func (s testSnakeCache) Del(ctx context.Context, key string) error {
	delete(s.mp, key)
	return nil
}

func (s testSnakeCache) IsDirectStore() bool {
	return true
}

func BenchmarkWrap(b *testing.B) {
	store := testSnakeCache{mp: make(map[string]any)}

	for i := 0; i < b.N; i++ {
		key := cast.ToString(rand.Int63())
		// 未命中缓存情况，包含读取和设置两级缓存
		_, _ = Wrap(context.Background(), "test-name", store, key, func(ctx context.Context) (int64, error) {
			return 0, nil
		})
	}
}

func BenchmarkWrapCtr(b *testing.B) {
	store := testSnakeCache{mp: make(map[string]any)}
	ctr := NewCacheController("test-name", store, WithPlugins[int64]())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := cast.ToString(rand.Int63())
		// 未命中缓存情况，包含读取和设置两级缓存
		_, _ = ctr.Wrap(context.Background(), key, func(ctx context.Context) (int64, error) {
			return 0, nil
		})
	}
}

func BenchmarkWrapReuseCtr(b *testing.B) {
	store := testSnakeCache{mp: make(map[string]any)}
	ctr := NewCacheController("test-name", store, WithPlugins[int64](), WithPolicy[int64](ReuseCachePloyIgnoreError(time.Minute)))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := cast.ToString(rand.Int63())
		// 未命中缓存情况，包含读取和设置两级缓存
		_, _ = ctr.Wrap(context.Background(), key, func(ctx context.Context) (int64, error) {
			return 0, nil
		})
	}
}

func BenchmarkWrapFirstCacheCtr(b *testing.B) {
	store := testSnakeCache{mp: make(map[string]any)}
	ctr := NewCacheController("test-name", store, WithPlugins[int64](), WithPolicy[int64](FirstCachePolyIgnoreError(time.Minute)))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := cast.ToString(rand.Int63())
		// 未命中缓存情况，包含读取和设置两级缓存
		_, _ = ctr.Wrap(context.Background(), key, func(ctx context.Context) (int64, error) {
			return 0, nil
		})
	}
}

func BenchmarkWrapRedisCtr(b *testing.B) {
	store, cancel := getRedis()
	defer cancel()
	ctr := NewCacheController("test-name", store, WithPlugins[int64](), WithPolicy[int64](FirstCachePolyIgnoreError(time.Minute)))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := cast.ToString(rand.Int63())
		// 未命中缓存情况，包含读取和设置两级缓存
		_, _ = ctr.Wrap(context.Background(), key, func(ctx context.Context) (int64, error) {
			return 0, nil
		})
	}
}
