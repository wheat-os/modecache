# library.cache.modecache

## 更新特性
1. redis storage 支持 hash 存储用例参考
```go
key := "role" // 创建 redis key
hashKey := "role_name"  // 创建 redis hash key
// 创建一个 redis hash 存储器, rds: library redis 连接池(可复用
// nCtx 用来存储临时 store
nCtx, store := modecache.NewRedisHashStore(rds, rdsKey, hashKey)
// cacheKey modecache key 用来标记缓存的唯一性（对同一个 key 会共享返回结果）
cacheKey := fmt.Sprintf("%s_%s", rdsKey, hashKey)
// 使用默认缓存策略，缓存 5minute
// rds command-> HGET role role_name -> test_role
userName, err := modecache.WrapWithTTL(nCtx, storage, cacheKey, time.Minute*5, func(ctx context.Context) (string, error) {
	return "test_role", nil
})
```



## Principle

## 介绍
`modecache` 是一个通用的封装缓存击穿，缓存降级，的通用缓存封装组件，用于合理的控制对数据库等有限资源，和缓存的使用时机。

`modecache` 一共分为 4 个组件，分别是控制器 (CacheCtr), 缓存策略 (Policy), 插件 (Plugin)，存储器(Store)
1. 控制器, 控制器主要负责包装外部访问的 API，提供缓存访问保护，泛型的处理等工作, 负责提供最终对外的接口。 `需要注意，同一组缓存资源，应该使用同一个缓存控制器`

2. 缓存策略，缓存策略用于描述缓存的访问，通俗来说我们的程序什么时机使用缓存，什么时机使用 DB 等资源就由缓存策略来决定

3. 插件，插件是缓存策略的扩展，插件可以自由扩展缓存策略，如在缓存策略上增加 SRE 熔断机制(在中间件无法稳定服务时候,对缓存策略降级)。插件的使用表现像拦截器，允许我们在访问缓存，或者访问 DB 前执行一些自定义的操作。

4. 存储器，存储器用来存储缓存数据，目前支持 `redis` 和 `本地缓存` 两种存储方式


## 任何选择缓存策略和存储器
我们可以首先对我们的缓存策略做简单的抽象，得到以下的几个建议指标
> 1. 一致性、2. 访问性能、3. 容错以及降级、4. 时效性

`Store` 的选择，共享缓存(Redis), 本地缓存(Cache) 决定了一致性和性能，`Policy` 的选择，决定了时效性和容错性。参考下的关系来查看已实现的策略和业务的关系。


如上图，我们简单描述场景2 个使用场景
1. 如果我们的业务比如 `用户信息` 等需要强一致性（需要全局一致），高时效性的参加（数据过期不可用），那么我们应该选择使用 `EasyPoly` 和 `共享缓存(redis)` 来实现用户信息的缓存。

2. 如果我们的业务比如 `后台业务配置信息` 访问频率非常高，对时效性没有特别要求的场景，我们可以使用 `本地缓存` 和 `FirstCachePoly` 来实现配置信息的缓存。



## Usage
快速开始，使用默认的 EasyCacheStore 缓存策略, 创建一个 5 秒的缓存，更多缓存策略见 `policy.go`
```go
package modecache_test

import(
    libCache "git.nd.com.cn/go-common/library/cache"
)
// 快速使用
def TestEasy(t *testing.T) {
   	// 创建一个平台封装本地的 Store，注意这里的 store 是可以多次复用，不一定需要每次都重新创建
	lc, _, _ := libCache.NewCache()
	store := modecache.NewCacheStore(lc)

	// 注意 modecache.Wrap 是简易调用的封装方法，无法调整缓存策略，且会额外增加获取缓存控制器的过程，大多数情况下不建议使用。如果需要更丰富的缓存使用，请使用 modecache.NewCacheController。

	// 这里模拟 DB 访问，从 DB 中查询一个为 1 的结果
	id, err := modecache.Wrap(context.Background(), "test", store, "test-key", func(ctx context.Context) (int, error) {
		return 1, nil
	})

	require.NoError(t, err)
	require.EqualValues(t, id, 1)
}
```
自定义使用缓存策略, 默认的缓存策略无法定义缓存时间，过期时间等，并且有名称和泛型的绑定限制，可以通过使用自定义控制器，来高度定义缓穿逻辑
```go
package modecache_test

import (
	"context"
	"testing"
	"time"

	"git.nd.com.cn/go-common/library/cache/modecache"
	"git.nd.com.cn/go-common/library/redis"
	"github.com/stretchr/testify/require"
)

// 这里模拟构建 Redis 缓存， Store 可以共享给全部的 CacheController
func getRedis() (modecache.Store, func()) {
	r, cleanup, _ := redis.NewRedis(
		// host:port（默认为 127.0.0.1:6379）
		redis.Addr("172.24.133.14:6379"),
		// AddrShadow("172.24.133.14:6380"),
		// 集群地址 host:port
		// ClusterAddr([]string{"peer1", "peer2"}),
		// 用户名
		redis.Username(""),
		// 密码
		redis.Password("nddev"),
	)
	return modecache.NewRedisStore(r), cleanup
}

// 对同一组缓存对象 int 应该全局唯一
var ctr1 = modecache.NewCacheController[int]("test-business", store)

func TestCtr(t *testing.T) {
	store, clean := getRedis()
	defer clean()

	// 这里构建了一个缓存控制器，名称为 test-business，指定存储 int 类型，使用默认的 EasyPloy(15 * time.Second) 策略
	msg, err := ctr1.Wrap(context.Background(), "name1", func(ctx context.Context) (int, error) {
		return 1, nil
	})
	require.NoError(t, err)
	require.Equal(t, msg, 1)
}

type MockStrut struct {
	Name  string
	Value int
}

// 对同一组缓存对象 *MockStrut 应该全局唯一
var ctr2 = modecache.NewCacheController[*MockStrut](
	"test-name",
	store,
	modecache.WithPolicy[*MockStrut](modecache.ReuseCachePloy(100*time.Second)),
)

func Test2(t *testing.T) {
	store, clean := getRedis()
	defer clean()

	// 构建一个名称为 test-name 的缓存控制器，它使用 redis store 存储器，缓存 *MockStrut 类型，使用 ReuseCachePloy(100 * time.Second) 策略
	msg, err := ctr2.Wrap(context.Background(), "name1", func(ctx context.Context) (*MockStrut, error) {
		return &MockStrut{
			Name:  "test",
			Value: 1,
		}, nil
	})
	require.NoError(t, err)
	require.Equal(t, msg.Name, "test")
	require.Equal(t, msg.Value, 1)
}
```

## 缓存策略
见：[policy.go]("policy.go")


## 如何使用插件
上文描述了，插件实际上是对缓存和 quey(有限资源) 的拦截器，其接口定义如下
```go
Plugin interface {
	// InterceptCallQuery 查询 query 前拦截调用
	// return: LoadingForQuery: 不为空的场景,替换执行的 LoadingForQuery
	// return: bool：是否允许继续执行插件，还是提前熔断
	// return: error: 错误, 会导流程结束返回 error
	InterceptCallQuery(ctx context.Context, key string, loadQuery LoadingForQuery) (LoadingForQuery, bool, error)

	// InterceptCallCache 查询 cache 前拦截调用
	// return: LoadingForCache: 不为空的场景,替换执行的 LoadingForCache
	// return: bool：是否允许继续执行插件，还是提前熔断
	// return: error: 错误, 会导流程结束返回 error
	InterceptCallCache(ctx context.Context, key string, loadCache LoadingForCache) (LoadingForCache, bool, error)
}
```
基于插件，我们可以实现简易访问日志输出插件
```go
// MetricsPlugin 指标插件
type PrintPlugin struct {

}

func (m *PrintPlugin) InterceptCallQuery(ctx context.Context, key string, loadQuery LoadingForQuery) (LoadingForQuery, bool, error) {
	return func(ctx context.Context, key string, ttl time.Duration) (any, error) {
		// 这里我们使用函数劫持掉加载有限资源的方法
		value, err := loadQuery(ctx, key, ttl)
		if err != nil{
			// 输出错误
			fmt.Print(err)
		}
		return value, err
	}, true, nil
}

func (m *PrintPlugin) InterceptCallCache(ctx context.Context, key string, loadCache LoadingForCache) (LoadingForCache, bool, error) {
	// 缓存的访问保持不变
	return loadCache, true, nil
}
```
同样基于上面的方案，我们还可以实现比如对数据库的访问限流, 见：[plugins.go]("plugins.go") 或者布隆过滤器等缓存保障机制的实现。

## 默认实现
已基于平台 `redis` 和 `cache` 包实现，请看`store_cache.go` 和 `store_redis.go`。

## 自定义实现
如果需要自定义数据存储方式，请实现`Store`接口。

## Q&A
