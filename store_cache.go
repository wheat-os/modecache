package modecache

import (
	"context"
	"time"

	"github.com/patrickmn/go-cache"
)

type cacheStore struct {
	libCache *cache.Cache
}

// Get 获取缓存。当缓存键不存在时返回 ErrKeyNonExistent 错误。
// return: 数据，数据创建时间，错误
func (c cacheStore) Get(ctx context.Context, key string) (any, error) {
	value, ok := c.libCache.Get(key)
	if !ok {
		return nil, ErrKeyNonExistent
	}

	return value, nil
}

// Set 设置缓存。
func (c cacheStore) Set(ctx context.Context, key string, data any, ttl time.Duration) error {
	if ttl == KeepTTL {
		c.libCache.Set(key, data, KeepTTL)
		return nil
	}
	c.libCache.Set(key, data, ttl)
	return nil
}

// Del 删除缓存。
func (c cacheStore) Del(ctx context.Context, key string) error {
	c.libCache.Delete(key)
	return nil
}

func (c cacheStore) IsDirectStore() bool {
	return true
}

func NewCacheStore(c *cache.Cache) Store {
	return cacheStore{libCache: c}
}
