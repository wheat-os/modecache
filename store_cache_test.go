package modecache

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCacheStore_Get(t *testing.T) {
	// 创建缓存对象
	cache := getTestLocalCache()
	// 创建 cacheStore 对象
	store := NewCacheStore(cache)

	// 设置缓存
	err := store.Set(context.Background(), "key", 123, time.Hour)
	assert.NoError(t, err)

	// 获取缓存
	value, err := store.Get(context.Background(), "key")
	assert.NoError(t, err)
	assert.Equal(t, 123, value)
}

func TestCacheStore_Get_NonExistent(t *testing.T) {
	// 创建缓存对象
	cache := getTestLocalCache()
	// 创建 cacheStore 对象
	store := NewCacheStore(cache)

	// 获取不存在的缓存
	value, err := store.Get(context.Background(), "key")
	assert.EqualError(t, err, ErrKeyNonExistent.Error())
	assert.Zero(t, value)
}

func TestCacheStore_Set(t *testing.T) {
	// 创建缓存对象
	cache := getTestLocalCache()

	// 创建 cacheStore 对象
	store := NewCacheStore(cache)

	// 设置缓存
	err := store.Set(context.Background(), "key", 123, time.Hour)
	assert.NoError(t, err)

	// 验证缓存是否设置成功
	value, ok := cache.Get("key")
	assert.True(t, ok)
	assert.Equal(t, 123, value)
}

func TestCacheStore_Del(t *testing.T) {
	// 创建缓存对象
	cache := getTestLocalCache()

	// 创建 cacheStore 对象
	store := NewCacheStore(cache)

	// 设置缓存
	err := store.Set(context.Background(), "key", 123, time.Hour)
	assert.NoError(t, err)

	// 删除缓存
	err = store.Del(context.Background(), "key")
	assert.NoError(t, err)

	// 验证缓存是否删除成功
	_, ok := cache.Get("key")
	assert.False(t, ok)
}
