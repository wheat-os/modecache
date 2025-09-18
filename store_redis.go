package modecache

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/spf13/cast"
)

// 影子链路方案使用 redis 实现
type redisStore struct {
	rds *redis.Client
}

func (r redisStore) Get(ctx context.Context, key string) (any, error) {
	cmd := r.rds.Do(ctx, "get", key)
	res, err := cmd.Result()
	switch {
	case err == nil:
	case errors.Is(err, redis.Nil):
		return nil, ErrKeyNonExistent
	default:
		return nil, err
	}

	return cast.ToString(res), nil
}

// Set 设置缓存。
func (r redisStore) Set(ctx context.Context, key string, data any, ttl time.Duration) error {
	//nolint:mnd
	args := make([]any, 3, 5)
	args[0] = "set"
	args[1] = key
	args[2] = data

	// 过期时间设置
	if ttl > 0 {
		if usePrecise(ttl) {
			args = append(args, "px", formatMs(ttl))
		} else {
			args = append(args, "ex", formatSec(ttl))
		}
	}

	cmd := r.rds.Do(ctx, args...)
	return cmd.Err()
}

// Del 删除缓存。
func (r redisStore) Del(ctx context.Context, key string) error {
	cmd := r.rds.Do(ctx, "del", key)
	return cmd.Err()
}

func (r redisStore) IsDirectStore() bool {
	return false
}

// NewRedisCache 新创建应该 redis cache
func NewRedisStore(rd *redis.Client) Store {
	return redisStore{rds: rd}
}

// 显示实现接口
var _ Store = (*RedisHashStore)(nil)

// NewRedisHashStore 创建 redis hash cache
// 注意 NewHashStore 设置过期时间会对整个 hash 进行设置
type RedisHashStore struct {
	rds     *redis.Client
	rdsKey  string
	hashKey string
}

// Get 获取缓存, 使用外部给定的 rds key 作为存储 key，避免和 modecache_key 冲突
func (r *RedisHashStore) Get(ctx context.Context, _ string) (any, error) {
	cmd := r.rds.Do(ctx, "hget", r.rdsKey, r.hashKey)
	res, err := cmd.Result()
	switch {
	case err == nil:
	case errors.Is(err, redis.Nil):
		return nil, ErrKeyNonExistent
	default:
		return nil, err
	}
	return cast.ToString(res), nil
}

// Set 设置缓存。
func (r *RedisHashStore) Set(ctx context.Context, _ string, data any, ttl time.Duration) error {
	//nolint:mnd
	args := make([]any, 4)
	args[0] = "hset"
	args[1] = r.rdsKey
	args[2] = r.hashKey
	args[3] = data
	cmd := r.rds.Do(ctx, args...)
	if cmd.Err() != nil {
		return cmd.Err()
	}
	// 过期时间设置
	// hash 类型无法直接设置过期时间，这里需要单独设置整个 hash 的过期时间
	if ttl > 0 {
		if usePrecise(ttl) {
			_ = r.rds.Do(ctx, "pexpire", r.rdsKey, formatMs(ttl)).Err()
		} else {
			_ = r.rds.Do(ctx, "expire", r.rdsKey, formatSec(ttl)).Err()
		}
	}
	return nil
}

func (r *RedisHashStore) Del(ctx context.Context, _ string) error {
	cmd := r.rds.Do(ctx, "hdel", r.rdsKey, r.hashKey)
	return cmd.Err()
}

// IsDirectStore 判断是否是直接存储
func (r *RedisHashStore) IsDirectStore() bool {
	return false
}

// DelAll 删除整个 hash
func (r *RedisHashStore) DelAll(ctx context.Context) error {
	cmd := r.rds.Do(ctx, "del", r.rdsKey)
	return cmd.Err()
}

// NewRedisHashStoreWithPrefix 新创建 hashKey redis 其中
// key: redis key, 最后存储的 redis key 注意这里不应该使用 modecache_key
// hashKey: redis hash key,注意不是 redis key
// return: ctx 需要向下传递用来替换默认的 storage
func NewRedisHashStore(ctx context.Context, rd *redis.Client, rdsKey string, rdsHashKey string) (context.Context, *RedisHashStore) {
	if rdsKey == "" || rdsHashKey == "" {
		panic("redis key or hash key is empty")
	}
	store := &RedisHashStore{rds: rd, hashKey: rdsHashKey, rdsKey: rdsKey}
	ctx = context.WithValue(ctx, CtxStorageKey{}, store)
	return ctx, store
}
