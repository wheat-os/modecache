# ModeCache

English | [简体中文](README.zh.md)

[![Go Report Card](https://goreportcard.com/badge/github.com/wheat-os/modecache)](https://goreportcard.com/report/github.com/wheat-os/modecache)
[![GoDoc](https://godoc.org/github.com/wheat-os/modecache?status.svg)](https://godoc.org/github.com/wheat-os/modecache)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

ModeCache is a universal cache encapsulation component designed to handle scenarios such as cache penetration and cache degradation, while reasonably controlling the timing of using limited resources like databases and cache.

## Features

- 🚀 High-performance cache access with cache penetration protection
- 🛡️ Multiple cache strategies to meet different business scenario requirements
- 🔌 Flexible plugin mechanism supporting custom extensions
- 💾 Support for multiple storage backends (Redis, local cache)
- 📊 Built-in monitoring metrics for easy observation of cache usage
- 🧩 Generic support with type safety

## Table of Contents

- [Installation](#installation)
- [Quick Start](#quick-start)
- [Core Concepts](#core-concepts)
- [Cache Strategies](#cache-strategies)
- [Storage](#storage)
- [Plugin System](#plugin-system)
- [Best Practices](#best-practices)
- [API Documentation](#api-documentation)
- [Contributing](#contributing)
- [License](#license)

## Installation

Install ModeCache using go get:

```bash
go get github.com/wheat-os/modecache
```

## Quick Start

### Basic Usage

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
	// Create local cache storage
	localCache := cache.New(5*time.Minute, 10*time.Minute)
	store := modecache.NewCacheStore(localCache)

	// Get data using default cache strategy
	ctx := context.Background()
	key := "user:123"

	userID, err := modecache.WrapWithTTL(ctx, store, key, 30*time.Second, func(ctx context.Context) (int, error) {
		// Simulate fetching data from database
		fmt.Println("Fetching from database...")
		return 123, nil
	})

	if err != nil {
		panic(err)
	}
	fmt.Printf("User ID: %d", userID)
}
```

## Core Concepts

ModeCache consists of four core components:

### 1. Controller (CacheCtr)

The controller is mainly responsible for wrapping external access APIs, providing cache access protection, generic processing, and is responsible for providing the final external interface.

**Note**: The same set of cache resources should use the same cache controller.

### 2. Cache Strategy (Policy)

Cache strategy is used to describe cache access logic, determining when the program uses cache and when to use resources like databases.

### 3. Plugin

Plugins are extensions of cache strategies that can freely extend cache strategies, such as adding SRE circuit breaker mechanisms to cache strategies. The use of plugins behaves like interceptors, allowing custom operations to be performed before accessing cache or database.

### 4. Storage

Storage is used to store cache data, currently supporting two storage methods: Redis and local cache.

## Cache Strategies

ModeCache provides multiple cache strategies suitable for different business scenarios:

### 1. EasyPloy

Simple strategy model that first tries to access cache, and if cache expires, tries to access database. If database access also fails, it returns an error.

```go
// Create a simple strategy with 15 seconds expiration
policy := modecache.EasyPloy(15 * time.Second)
```

Applicable scenarios: Businesses that require strong consistency (globally consistent) and high timeliness (data is unusable after expiration), such as user information.

### 2. ReuseCachePloyIgnoreError

Reuse cache model that stores data in cache for a long time, uses business expiration time to control cache expiration, and uses cache data to complete service when database query fails.

```go
// Create a reuse cache strategy with 30 seconds business expiration
policy := modecache.ReuseCachePloyIgnoreError(30 * time.Second)
```

**Note**: If cache is hit, when database query execution fails, this strategy will reuse cache data until the query execution succeeds.

### 3. FirstCachePolyIgnoreError

Fast cache model that saves cache for a long time, prioritizes cache usage, and uses business expiration time to control whether cache expires. If cache expires, it will start a singleton coroutine to access database asynchronously to refresh cache and return the cache data obtained this time.

```go
// Create a fast cache strategy with 1 minute business expiration
policy := modecache.FirstCachePolyIgnoreError(1 * time.Minute)
```

**Note**: If cache is hit, when database query execution fails, this strategy will reuse cache data until the query execution succeeds.

Applicable scenarios: Scenarios with very high access frequency and no special requirements for timeliness, such as background business configuration information.

## Storage

### Redis Storage

Storage implemented based on Redis, supporting distributed cache.

```go
// Create Redis storage
store := modecache.NewRedisStore(redisClient)
```

### Redis Hash Storage

Storage implemented based on Redis Hash, suitable for scenarios that need to organize related data together.

```go
// Create Redis Hash storage
ctx, store := modecache.NewRedisHashStore(context.Background(), redisClient, "redisKey", "hashKey")
```

### Local Cache Storage

Local cache storage based on memory, with higher performance but not supporting distributed.

```go
// Create local cache storage
store := modecache.NewCacheStore(cacheInstance)
```

## Plugin System

ModeCache provides a flexible plugin mechanism that allows custom logic to be executed before and after cache access and database queries.

### Plugin Interface

```go
type Plugin interface {
	// InterceptCallQuery Intercept call before querying database
	// return: LoadingForQuery: If not empty, replace the executed LoadingForQuery
	// return: bool: Whether to allow continued plugin execution or early circuit breaking
	// return: error: Error will cause the process to end and return error
	InterceptCallQuery(ctx context.Context, key string, loadQuery LoadingForQuery) (LoadingForQuery, bool, error)

	// InterceptCallCache Intercept call before querying cache
	// return: LoadingForCache: If not empty, replace the executed LoadingForCache
	// return: bool: Whether to allow continued plugin execution or early circuit breaking
	// return: error: Error will cause the process to end and return error
	InterceptCallCache(ctx context.Context, key string, loadCache LoadingForCache) (LoadingForCache, bool, error)
}
```

## Best Practices

### Choosing Appropriate Cache Strategy and Storage

When choosing cache strategy and storage, consider the following metrics:

1. Consistency
2. Access performance
3. Fault tolerance and degradation
4. Timeliness

The choice of storage (shared cache Redis or local cache Cache) determines consistency and performance, while the choice of cache strategy determines timeliness and fault tolerance.

#### Scenario 1: User Information Cache

For businesses that require strong consistency (globally consistent) and high timeliness (data is unusable after expiration), such as user information:

```go
// Use EasyPloy strategy and Redis storage
ctr := modecache.NewCacheController[User]("user-service", redisStore,
    modecache.WithPolicy[User](modecache.EasyPloy(5*time.Minute)),
)
```

#### Scenario 2: Configuration Information Cache

For scenarios with very high access frequency and no special requirements for timeliness, such as background business configuration information:

```go
// Use FirstCachePolyIgnoreError strategy and local cache storage
ctr := modecache.NewCacheController[Config]("config-service", localCacheStore,
    modecache.WithPolicy[Config](modecache.FirstCachePolyIgnoreError(30*time.Minute)),
)
```


## API Documentation

For detailed API documentation, please refer to [GoDoc](https://godoc.org/github.com/wheat-os/modecache).

### Main Types and Functions

- `CacheCtr[T]`: Cache controller for managing cache access of specific types.
- `Store`: Storage interface defining basic cache operations.
- `Policy`: Cache strategy type defining cache access logic.
- `Plugin`: Plugin interface allowing custom extension of cache behavior.
- `NewCacheController[T]`: Create new cache controller.
- `NewRedisStore`: Create Redis storage.
- `NewRedisHashStore`: Create Redis Hash storage.
- `NewCacheStore`: Create local cache storage.
- `EasyPloy`: Create simple strategy model.
- `ReuseCachePloyIgnoreError`: Create reuse cache strategy model.
- `FirstCachePolyIgnoreError`: Create fast cache strategy model.

## Contributing

Contributions are welcome! Please ensure:

1. Follow the project's code style
2. Add appropriate tests
3. Update documentation

Before submitting a Pull Request, please ensure all tests pass:

```bash
go test ./...
```

## License

ModeCache is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.
