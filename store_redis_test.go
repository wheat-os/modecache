package modecache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func getRedis() (Store, func()) {
	s, err := miniredis.Run()
	if err != nil {
		panic(err)
	}
	client := redis.NewClient(&redis.Options{Addr: s.Addr()})

	return NewRedisStore(client), func() {
		client.Close()
		s.Close()
	}
}

func getTestRedis() (*redis.Client, func()) {
	s, err := miniredis.Run()
	if err != nil {
		panic(err)
	}
	client := redis.NewClient(&redis.Options{Addr: s.Addr()})

	return client, func() {
		client.Close()
		s.Close()
	}
}

func TestRedisStore_Get(t *testing.T) {
	// 创建 cacheStore 对象
	store, cleanup := getRedis()
	defer cleanup()

	// 设置缓存
	err := store.Set(context.Background(), "key", 123, time.Hour)
	assert.NoError(t, err)

	// 获取缓存
	value, err := store.Get(context.Background(), "key")
	assert.NoError(t, err)
	assert.Equal(t, "123", value)

	// 设置缓存 String
	err = store.Set(context.Background(), "key", "123", time.Hour)
	assert.NoError(t, err)

	// 获取缓存
	value, err = store.Get(context.Background(), "key")
	assert.NoError(t, err)
	assert.Equal(t, "123", value)

	// 设置缓存使用 KeepTTL
	err = store.Set(context.Background(), "key", "123", KeepTTL)
	assert.NoError(t, err)

	// 获取缓存
	value, err = store.Get(context.Background(), "key")
	assert.NoError(t, err)
	assert.Equal(t, "123", value)
}

func TestRedisStore_Get_NonExistent(t *testing.T) {
	// 创建 cacheStore 对象
	store, cleanup := getRedis()
	defer cleanup()

	// 获取不存在的缓存
	value, err := store.Get(context.Background(), "key_not_is")
	assert.EqualError(t, err, ErrKeyNonExistent.Error())
	assert.Zero(t, value)
}

func TestRedisStore_Del(t *testing.T) {
	// 创建缓存对象
	store, cleanup := getRedis()
	defer cleanup()

	// 设置缓存
	err := store.Set(context.Background(), "key", 123, time.Hour)
	assert.NoError(t, err)

	// 删除缓存
	err = store.Del(context.Background(), "key")
	assert.NoError(t, err)

	// 验证缓存是否删除成功
	_, err = store.Get(context.Background(), "key")
	assert.Error(t, err, ErrKeyNonExistent)
}

func TestRedisStore_Set(t *testing.T) {
	// 创建 cacheStore 对象
	rds, cleanup := getTestRedis()
	defer cleanup()

	// 设置缓存并验证
	_, store := NewRedisHashStore(context.Background(), rds, "key", "hashKey")
	err := store.Set(context.Background(), "key-str", "value", time.Hour)
	assert.NoError(t, err)
	// 获取缓存并验证
	value, err := store.Get(context.Background(), "key")
	assert.NoError(t, err)
	assert.Equal(t, "value", value)

	// 设置缓存为整数并验证
	err = store.Set(context.Background(), "key-int", 123, time.Hour)
	assert.NoError(t, err)

	value, err = store.Get(context.Background(), "key")
	assert.NoError(t, err)
	assert.Equal(t, "123", value)

	// 设置缓存为浮点数并验证
	err = store.Set(context.Background(), "key-float", 123.45, time.Hour)
	assert.NoError(t, err)

	value, err = store.Get(context.Background(), "key")
	assert.NoError(t, err)
	assert.Equal(t, "123.45", value)

	// 设置缓存为布尔值并验证
	err = store.Set(context.Background(), "key-bool", true, time.Hour)
	assert.NoError(t, err)

	value, err = store.Get(context.Background(), "key")
	assert.NoError(t, err)
	assert.Equal(t, "1", value)

	// 验证过期
	// err = store.Set(context.Background(), "key-ext", "value-ext", 1*time.Second)
	// assert.NoError(t, err)
	// // 查询
	// value, err = store.Get(context.Background(), "key-ext")
	// assert.NoError(t, err)
	// assert.Equal(t, "value-ext", value)
	// time.Sleep(2 * time.Second)
	// // 查询
	// _, err = store.Get(context.Background(), "key-ext")
	// assert.EqualError(t, err, ErrKeyNonExistent.Error())
}

func TestRedisHashStore_Get(t *testing.T) {
	// 创建 hashStore 对象
	rds, cleanup := getTestRedis()
	defer cleanup()

	// 设置缓存
	rdsKey := "library-hash-key"
	_, store := NewRedisHashStore(context.Background(), rds, rdsKey, "field")

	err := store.Set(context.Background(), rdsKey, "value", time.Hour)
	assert.NoError(t, err)

	// 获取缓存并验证
	value, err := store.Get(context.Background(), rdsKey)
	assert.NoError(t, err)
	assert.Equal(t, "value", value)

	// 获取不存在的缓存
	_, store = NewRedisHashStore(context.Background(), rds, rdsKey, "nonexistent_field")
	value, err = store.Get(context.Background(), rdsKey)
	assert.EqualError(t, err, ErrKeyNonExistent.Error())
	assert.Zero(t, value)
}

func TestRedisHashStore_Del(t *testing.T) {
	// 创建 hashStore 对象
	rds, cleanup := getTestRedis()
	defer cleanup()

	// 设置缓存
	rdsKey := "library-hash-key"
	_, store := NewRedisHashStore(context.Background(), rds, rdsKey, "field")
	err := store.Set(context.Background(), rdsKey, "value", time.Hour)
	assert.NoError(t, err)

	// 删除缓存
	err = store.Del(context.Background(), rdsKey)
	assert.NoError(t, err)

	// 验证缓存是否删除成功
	_, err = store.Get(context.Background(), "field")
	assert.EqualError(t, err, ErrKeyNonExistent.Error())
}

func TestRedisHashStore_DelAll(t *testing.T) {
	// 创建 hashStore 对象
	rds, cleanup := getTestRedis()
	defer cleanup()

	// 设置缓存
	rdsKey := "library-hash-key"
	_, store := NewRedisHashStore(context.Background(), rds, rdsKey, "field")
	err := store.Set(context.Background(), rdsKey, "value", time.Hour)
	assert.NoError(t, err)

	// 删除缓存
	err = store.DelAll(context.Background())
	assert.NoError(t, err)

	// 验证缓存是否删除成功
	_, err = store.Get(context.Background(), "field")
	assert.EqualError(t, err, ErrKeyNonExistent.Error())
}
