# ModeCache

[![Go Report Card](https://goreportcard.com/badge/github.com/wheat-os/modecache)](https://goreportcard.com/report/github.com/wheat-os/modecache)
[![GoDoc](https://godoc.org/github.com/wheat-os/modecache?status.svg)](https://godoc.org/github.com/wheat-os/modecache)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

ModeCache 是一个通用的缓存封装组件，用于处理缓存击穿、缓存降级等场景，合理控制对数据库等有限资源和缓存的使用时机。

## 特性

- 🚀 高性能缓存访问，支持缓存击穿保护
- 🛡️ 多种缓存策略，满足不同业务场景需求
- 🔌 灵活的插件机制，支持自定义扩展
- 💾 支持多种存储后端（Redis、本地缓存）
- 📊 内置监控指标，便于观察缓存使用情况
- 🧩 泛型支持，类型安全

## 目录

- [安装](#安装)
- [快速开始](#快速开始)
- [核心概念](#核心概念)
- [缓存策略](#缓存策略)
- [存储器](#存储器)
- [插件系统](#插件系统)
- [最佳实践](#最佳实践)
- [API 文档](#api-文档)
- [贡献](#贡献)
- [许可证](#许可证)

## 安装

使用 go get 安装 ModeCache：

```bash
go get github.com/wheat-os/modecache
```

## 快速开始

### 基本用法

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/wheat-os/modecache"
	"github.com/patrickmn/go-cache"
)

func main() {
	// 创建本地缓存存储器
	localCache := cache.New(5*time.Minute, 10*time.Minute)
	store := modecache.NewCacheStore(localCache)

	// 使用默认缓存策略获取数据
	ctx := context.Background()
	key := "user:123"

	userID, err := modecache.Wrap(ctx, "user-service", store, key, func(ctx context.Context) (int, error) {
		// 模拟从数据库获取数据
		fmt.Println("Fetching from database...")
		return 123, nil
	})

	if err != nil {
		panic(err)
	}

	fmt.Printf("User ID: %d
", userID)
}
```

### 使用 Redis 存储器

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/wheat-os/modecache"
	"github.com/redis/go-redis/v9"
)

func main() {
	// 创建 Redis 客户端
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // 无密码
		DB:       0,  // 默认数据库
	})

	// 创建 Redis 存储器
	store := modecache.NewRedisStore(rdb)

	// 使用自定义缓存控制器
	ctr := modecache.NewCacheController[string]("user-service", store,
		modecache.WithPolicy[string](modecache.EasyPloy(10*time.Minute)),
	)

	ctx := context.Background()
	key := "user:123:name"

	userName, err := ctr.Wrap(ctx, key, func(ctx context.Context) (string, error) {
		// 模拟从数据库获取数据
		fmt.Println("Fetching user name from database...")
		return "John Doe", nil
	})

	if err != nil {
		panic(err)
	}

	fmt.Printf("User Name: %s
", userName)
}
```

### 使用 Redis Hash 存储器

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/wheat-os/modecache"
	"github.com/redis/go-redis/v9"
)

func main() {
	// 创建 Redis 客户端
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // 无密码
		DB:       0,  // 默认数据库
	})

	// 创建 Redis Hash 存储器
	rdsKey := "roles"      // Redis key
	hashKey := "role_name" // Redis hash key
	ctx, store := modecache.NewRedisHashStore(context.Background(), rdb, rdsKey, hashKey)

	// 使用默认缓存策略获取数据
	cacheKey := fmt.Sprintf("%s_%s", rdsKey, hashKey)
	roleName, err := modecache.WrapWithTTL(ctx, store, cacheKey, 5*time.Minute, func(ctx context.Context) (string, error) {
		// 模拟从数据库获取数据
		fmt.Println("Fetching role name from database...")
		return "admin", nil
	})

	if err != nil {
		panic(err)
	}

	fmt.Printf("Role Name: %s
", roleName)
}
```

## 核心概念

ModeCache 由四个核心组件组成：

### 1. 控制器 (CacheCtr)

控制器主要负责包装外部访问的 API，提供缓存访问保护、泛型处理等工作，负责提供最终对外的接口。

**注意**：同一组缓存资源，应该使用同一个缓存控制器。

### 2. 缓存策略 (Policy)

缓存策略用于描述缓存的访问逻辑，决定程序何时使用缓存，何时使用数据库等资源。

### 3. 插件 (Plugin)

插件是缓存策略的扩展，可以自由扩展缓存策略，如在缓存策略上增加 SRE 熔断机制。插件的使用表现像拦截器，允许在访问缓存或访问数据库前执行自定义操作。

### 4. 存储器 (Store)

存储器用来存储缓存数据，目前支持 Redis 和本地缓存两种存储方式。

## 缓存策略

ModeCache 提供了多种缓存策略，适用于不同的业务场景：

### 1. EasyPloy

简单策略模型，先尝试访问缓存，如果缓存过期则尝试访问数据库，如果数据库也获取失败则返回错误。

```go
// 创建一个 15 秒过期时间的简单策略
policy := modecache.EasyPloy(15 * time.Second)
```

适用场景：需要强一致性（全局一致）、高时效性（数据过期不可用）的业务，如用户信息。

### 2. ReuseCachePloyIgnoreError

重用缓存模型，长时间存储数据到缓存中，使用业务过期时间控制缓存过期，并且在数据库查询失败时使用缓存数据完成服务。

```go
// 创建一个 30 秒业务过期时间的重用缓存策略
policy := modecache.ReuseCachePloyIgnoreError(30 * time.Second)
```

**注意**：如果命中缓存，当数据库查询执行失败时，这个策略会重复使用缓存数据，直到查询执行成功为止。

### 3. FirstCachePolyIgnoreError

快速缓存模型，长时间保存缓存，优先使用缓存，使用业务过期时间控制缓存是否过期。如果缓存过期，会拉起一个单例协程来访问数据库异步刷新缓存，并返回本次获取到的缓存数据。

```go
// 创建一个 1 分钟业务过期时间的快速缓存策略
policy := modecache.FirstCachePolyIgnoreError(1 * time.Minute)
```

**注意**：如果命中缓存，当数据库查询执行失败时，这个策略会重复使用缓存数据，直到查询执行成功为止。

适用场景：访问频率非常高，对时效性没有特别要求的场景，如后台业务配置信息。

## 存储器

### Redis 存储器

基于 Redis 实现的存储器，支持分布式缓存。

```go
// 创建 Redis 存储器
store := modecache.NewRedisStore(redisClient)
```

### Redis Hash 存储器

基于 Redis Hash 实现的存储器，适用于需要将相关数据组织在一起的场景。

```go
// 创建 Redis Hash 存储器
ctx, store := modecache.NewRedisHashStore(context.Background(), redisClient, "redisKey", "hashKey")
```

### 本地缓存存储器

基于内存的本地缓存存储器，性能更高，但不支持分布式。

```go
// 创建本地缓存存储器
store := modecache.NewCacheStore(cacheInstance)
```

## 插件系统

ModeCache 提供了灵活的插件机制，允许在缓存访问和数据库查询前后执行自定义逻辑。

### 插件接口

```go
type Plugin interface {
    // InterceptCallQuery 查询数据库前拦截调用
    // return: LoadingForQuery: 不为空时，替换执行的 LoadingForQuery
    // return: bool：是否允许继续执行插件，还是提前熔断
    // return: error: 错误，会导致流程结束返回 error
    InterceptCallQuery(ctx context.Context, key string, loadQuery LoadingForQuery) (LoadingForQuery, bool, error)

    // InterceptCallCache 查询缓存前拦截调用
    // return: LoadingForCache: 不为空时，替换执行的 LoadingForCache
    // return: bool：是否允许继续执行插件，还是提前熔断
    // return: error: 错误，会导致流程结束返回 error
    InterceptCallCache(ctx context.Context, key string, loadCache LoadingForCache) (LoadingForCache, bool, error)
}
```

### 内置插件

#### 限流插件

对数据库访问进行限流，防止缓存击穿时数据库压力过大。

```go
// 创建限流插件，限制每秒 100 次查询，突发 200 次
plugin := modecache.NewLimitQueryPlugin(100, 200)

// 创建缓存控制器并添加插件
ctr := modecache.NewCacheController[User]("user-service", store,
    modecache.WithPlugin(plugin),
)
```

#### 监控插件

收集缓存访问指标，便于监控和观察缓存使用情况。

```go
// 创建监控插件
plugin := modecache.NewMetricsPlugin("user-service")

// 创建缓存控制器并添加插件
ctr := modecache.NewCacheController[User]("user-service", store,
    modecache.WithPlugin(plugin),
)
```

### 自定义插件

```go
// 自定义日志插件
type LogPlugin struct{}

func (p *LogPlugin) InterceptCallQuery(ctx context.Context, key string, loadQuery modecache.LoadingForQuery) (modecache.LoadingForQuery, bool, error) {
    return func(ctx context.Context, key string, ttl time.Duration) (interface{}, error) {
        start := time.Now()
        fmt.Printf("Querying database for key: %s
", key)

        value, err := loadQuery(ctx, key, ttl)

        fmt.Printf("Database query completed in %v, error: %v
", time.Since(start), err)
        return value, err
    }, true, nil
}

func (p *LogPlugin) InterceptCallCache(ctx context.Context, key string, loadCache modecache.LoadingForCache) (modecache.LoadingForCache, bool, error) {
    return func(ctx context.Context, key string) (interface{}, int, error) {
        start := time.Now()
        fmt.Printf("Querying cache for key: %s
", key)

        value, timestamp, err := loadCache(ctx, key)

        fmt.Printf("Cache query completed in %v, error: %v
", time.Since(start), err)
        return value, timestamp, err
    }, true, nil
}

// 使用自定义插件
plugin := &LogPlugin{}
ctr := modecache.NewCacheController[User]("user-service", store,
    modecache.WithPlugin(plugin),
)
```

## 最佳实践

### 选择合适的缓存策略和存储器

选择缓存策略和存储器时，可以考虑以下几个指标：

1. 一致性
2. 访问性能
3. 容错以及降级
4. 时效性

存储器的选择（共享缓存 Redis 或本地缓存 Cache）决定了一致性和性能，缓存策略的选择决定了时效性和容错性。

#### 场景一：用户信息缓存

对于需要强一致性（全局一致）、高时效性（数据过期不可用）的业务，如用户信息：

```go
// 使用 EasyPloy 策略和 Redis 存储器
ctr := modecache.NewCacheController[User]("user-service", redisStore,
    modecache.WithPolicy[User](modecache.EasyPloy(5*time.Minute)),
)
```

#### 场景二：配置信息缓存

对于访问频率非常高，对时效性没有特别要求的场景，如后台业务配置信息：

```go
// 使用 FirstCachePolyIgnoreError 策略和本地缓存存储器
ctr := modecache.NewCacheController[Config]("config-service", localCacheStore,
    modecache.WithPolicy[Config](modecache.FirstCachePolyIgnoreError(30*time.Minute)),
)
```

### 避免缓存控制器冲突

同一组缓存对象应该使用同一个缓存控制器，避免创建多个相同名称但泛型不同的缓存控制器：

```go
// 正确：全局唯一的缓存控制器
var userController = modecache.NewCacheController[User]("user-service", store)

// 错误：可能会与上面的控制器冲突
var badController = modecache.NewCacheController[Profile]("user-service", store)
```

### 合理使用插件

根据业务需求选择合适的插件组合：

```go
// 组合使用限流插件和监控插件
limitPlugin := modecache.NewLimitQueryPlugin(100, 200)
metricsPlugin := modecache.NewMetricsPlugin("user-service")

ctr := modecache.NewCacheController[User]("user-service", store,
    modecache.WithPlugin(limitPlugin),
    modecache.WithPlugin(metricsPlugin),
)
```

## API 文档

详细的 API 文档请参考 [GoDoc](https://godoc.org/github.com/wheat-os/modecache)。

### 主要类型和函数

- `CacheCtr[T]`：缓存控制器，用于管理特定类型的缓存访问。
- `Store`：存储器接口，定义了缓存的基本操作。
- `Policy`：缓存策略类型，定义了缓存访问逻辑。
- `Plugin`：插件接口，允许自定义扩展缓存行为。
- `NewCacheController[T]`：创建新的缓存控制器。
- `NewRedisStore`：创建 Redis 存储器。
- `NewRedisHashStore`：创建 Redis Hash 存储器。
- `NewCacheStore`：创建本地缓存存储器。
- `EasyPloy`：创建简单策略模型。
- `ReuseCachePloyIgnoreError`：创建重用缓存策略模型。
- `FirstCachePolyIgnoreError`：创建快速缓存策略模型。

## 贡献

欢迎贡献代码！请确保：

1. 遵循项目的代码风格
2. 添加适当的测试
3. 更新文档

提交 Pull Request 前，请确保所有测试都通过：

```bash
go test ./...
```

## 许可证

ModeCache 使用 MIT 许可证。详情请参见 [LICENSE](LICENSE) 文件。

