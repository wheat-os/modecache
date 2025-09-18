package modecache

import (
	"context"
	"time"
)

// EasyPloy 创建简单策略模型
// 该模式会先尝试访问缓存，如果缓存发生过期则尝试访问数据库，如果数据库也获取失败则返回错误。
func EasyPloy(ttl time.Duration) Policy {
	sg := SingleflightGroup{}

	return func(ctx context.Context, key string, loadingQuery LoadingForQuery, loadingCache LoadingForCache) (any, error) {
		value, err, _ := sg.Do(ctx, key, func() (interface{}, error) {
			value, _, qErr := loadingCache(ctx, key)
			if qErr == nil {
				return value, nil
			}
			value, dErr := loadingQuery(ctx, key, ttl)
			return value, dErr
		})

		if err != nil {
			return nil, err
		}
		return value, nil
	}
}

// ReuseCachePloyIgnoreError 创建一个使用重用缓存的访问模式
// 重用缓存模型，会把数据长时间的存储到缓存中，使用业务过期时间 expireTime 来控制缓存的过期，
// 并且在 下游 query 接口无法调用成功的场景，使用缓存数据完成服务
// # 注意如果命中缓存，那么当 query 执行失败时，这个策略会重复使用缓存数据，直到 query 执行成功为止。
func ReuseCachePloyIgnoreError(expireTime time.Duration) Policy {
	const ttl = KeepTTL // 默认存储 7 天
	sg := SingleflightGroup{}

	return func(ctx context.Context, key string, loadingQuery LoadingForQuery, loadingCache LoadingForCache) (any, error) {
		value, err, _ := sg.Do(ctx, key, func() (interface{}, error) {
			var isReuse = false
			result, timestamp, cErr := loadingCache(ctx, key)
			if cErr == nil {
				isReuse = true
				if time.Now().Unix()-int64(timestamp) < int64(expireTime.Seconds()) {
					return result, nil
				}
			}

			value, qErr := loadingQuery(ctx, key, ttl)
			if qErr == nil {
				return value, nil
			}

			if isReuse {
				return result, nil
			}
			return nil, qErr
		})

		if err != nil {
			return nil, err
		}
		return value, err
	}
}

// FirstCachePolyIgnoreError 创建一个快速缓存模型
// 快速缓存模型，会长时间保存缓存，并且优先使用缓存，使用业务过期时间 expireTime 来控制缓存是否过期，如果缓存过期会
// 拉起一个单例携程来访问 query 异步刷新缓存，并且返回本次获取到的缓存中的数据，如果访问缓存失败，则退化为简单缓存模型
// # 注意如果命中缓存，那么当 query 执行失败时，这个策略会重复使用缓存数据，直到 query 执行成功为止。
func FirstCachePolyIgnoreError(expireTime time.Duration) Policy {
	const ttl = KeepTTL
	sg := SingleflightGroup{}
	mu := Mutex128{}

	return func(ctx context.Context, key string, loadingQuery LoadingForQuery, loadingCache LoadingForCache) (any, error) {
		value, err, _ := sg.Do(ctx, key, func() (interface{}, error) {
			var isReuse bool
			result, timestamp, cErr := loadingCache(ctx, key)
			if cErr == nil {
				isReuse = true
				if time.Now().Unix()-int64(timestamp) < int64(expireTime.Seconds()) {
					return result, nil
				}
			}

			// 无法重用缓存, 降级为策略模式
			if !isReuse {
				return loadingQuery(ctx, key, ttl)
			}

			// 创建一个携程只允许同时存在 1 个 执行 loadingQuery
			// 对 key 计算 hash 输出 uint
			shard := hashCrc32ToUint(key)
			if mu.TryLock(shard) {
				GO(func() {
					defer mu.Unlock(shard)
					nCtx := context.WithoutCancel(ctx)
					nCtx, cancel := context.WithTimeout(nCtx, expireTime)
					defer cancel()
					_, _ = loadingQuery(nCtx, key, ttl)
				})
			}

			return result, nil
		})

		if err != nil {
			return nil, err
		}
		return value, err
	}
}
