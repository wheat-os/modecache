package modecache

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/pkg/errors"
)

const (
	SHadowKeyPrefix = "shadow:"

	KeepTTL = -1 // 永久存储
)

var (
	ErrKeyNonExistent  = errors.New("modecache: key does not exist")    // ErrKeyNonExistent 缓存键不存在。
	ErrUnpackingFailed = errors.New("modecache: warp unpacking failed") // warp 拆箱失败。
	ErrNil             = errors.New("null pointer")                     // Nil 空指针。
	ErrCircuitOpen     = errors.New("modecache: circuit breaker open")  // ErrCircuitOpen 熔断器开启。
)

type (
	Store interface {
		// Get 获取缓存。当缓存键不存在时返回 ErrKeyNonExistent 错误。
		Get(ctx context.Context, key string) (any, error)
		// Set 设置缓存, ttl 使用 KeepTTL 表示用不过期
		Set(ctx context.Context, key string, data any, ttl time.Duration) error
		// Del 删除缓存。
		Del(ctx context.Context, key string) error

		// IsDirectStore 释放可以直接存储数据，而不需要编码后存储
		// 当 IsDirectStore 为 True 时，存储管理器会少一次编码和解码的操作，以提高缓存读取的性能（本地缓存可用）
		IsDirectStore() bool
	}

	// Query 查询方法类型。
	Query[T any] func(context.Context) (T, error)

	// AbcBox 抽象箱
	AbcBox[T any] struct {
		Timestamp int `json:"Timestamp"`
		T         T   `json:"T"`
	}

	// LoadingForCache 封装查询方法，return：数据, 数据创建时间，错误
	LoadingForCache func(ctx context.Context, key string) (any, int, error)

	// LoadingForQuery 数据库封装方法
	LoadingForQuery func(ctx context.Context, key string, ttl time.Duration) (any, error)

	// Policy 缓存控制策略, 用来控制缓存策略
	Policy func(ctx context.Context, key string, queryFormDB LoadingForQuery, queryFormCache LoadingForCache) (any, error)

	// 访问控制插件
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
)

// CtxStorageKey 上下文存储键,用来存储可变的 storage 实现替换全局 storage
type CtxStorageKey struct{}

type CacheCtr[T any] struct {
	Name    string   // 缓存控制名称
	plugins []Plugin // 缓存控制器插件
	warp    Policy   // 缓存控制策略
	store   Store    // 缓存层
}

// SetStore 设置缓存到 Store
func (c *CacheCtr[T]) SetStore(ctx context.Context, key string, value T, ttl time.Duration) error {
	// 优先使用 上下文中的 Store
	store := c.store
	if ctxStore, ok := ctx.Value(CtxStorageKey{}).(Store); ok {
		store = ctxStore
	}

	// 装箱
	box := AbcBox[T]{
		T:         value,
		Timestamp: int(time.Now().Unix()),
	}
	// 设置缓存, 根据 OriginalStore 检查
	if store.IsDirectStore() {
		return store.Set(ctx, key, &box, ttl)
	}

	// 编码处理
	strVal, err := sonic.MarshalString(&box)
	if err != nil {
		return err
	}
	return store.Set(ctx, key, strVal, ttl)
}

// GetStore 从 Store 中获取缓存
func (c *CacheCtr[T]) GetStore(ctx context.Context, key string) (T, int, error) {
	// 优先使用 上下文中的 Store
	store := c.store
	if ctxStore, ok := ctx.Value(CtxStorageKey{}).(Store); ok {
		store = ctxStore
	}

	value, err := store.Get(ctx, key)
	if err != nil {
		return *new(T), 0, err
	}
	var box = new(AbcBox[T])
	if store.IsDirectStore() {
		cBox, ok := value.(*AbcBox[T])
		if !ok {
			LogErrorf(ctx, "GetStore: assert type to abcBox fail, key=%s, name=%s, actualType=%T", key, c.Name, value)
			return *new(T), 0, fmt.Errorf("%w: assert type to abcBox fail", ErrUnpackingFailed)
		}
		box = cBox
	} else {
		strVal, ok := value.(string)
		if !ok {
			LogErrorf(ctx, "GetStore: directStore need string but got %T, key=%s, name=%s", value, key, c.Name)
			return *new(T), 0, fmt.Errorf("%w: directStore need string but got %s", ErrUnpackingFailed, fmt.Sprintf("%T", strVal))
		}
		if err = sonic.Unmarshal([]byte(strVal), box); err != nil {
			LogErrorf(ctx, "GetStore: directStore unmarshal to abcBox fail, key=%s, name=%s, err=%v", key, c.Name, err)
			return *new(T), 0, fmt.Errorf("%w: directStore unmarshal to abcBox fail, %w", ErrUnpackingFailed, err)
		}
	}
	return box.T, box.Timestamp, nil
}

// Wrap 控制器的包装方法，控制使用 warp 方案
func (c *CacheCtr[T]) Wrap(ctx context.Context, key string, query Query[T]) (p T, err error) {
	loadQuery, err := c.buildTryLoadingQuery(ctx, key, query)
	if err != nil {
		return p, err
	}
	loadCache, err := c.buildTryLoadingCache(ctx, key)
	if err != nil {
		return p, err
	}

	result, err := c.warp(ctx, key, loadQuery, loadCache)
	if err != nil {
		return p, err
	}
	v, ok := result.(T)
	if !ok {
		LogErrorf(ctx, "Wrap: parse for T error, key=%s, name=%s, actualType=%T", key, c.Name, result)
		return p, errors.WithMessage(ErrUnpackingFailed, "parse for T error")
	}
	return v, nil
}

// buildTryLoadingCache 构造缓存加载方法
func (c *CacheCtr[T]) buildTryLoadingCache(ctx context.Context, key string) (LoadingForCache, error) {
	loadCache := func(ctx context.Context, key string) (any, int, error) {
		value, timestamp, err := c.GetStore(ctx, key)
		if err != nil {
			return nil, 0, err
		}
		if isNil(value) {
			return nil, 0, ErrNil
		}
		return value, timestamp, nil
	}

	for _, plugin := range c.plugins {
		plugCache, ok, err := plugin.InterceptCallCache(ctx, key, loadCache)
		if err != nil {
			return nil, err
		}
		loadCache = plugCache
		if !ok {
			break
		}
	}
	return loadCache, nil
}

// 构造 query 加载方法
func (c *CacheCtr[T]) buildTryLoadingQuery(ctx context.Context, key string, query Query[T]) (LoadingForQuery, error) {
	loadQuery := func(ctx context.Context, key string, ttl time.Duration) (any, error) {
		// 调用query方法
		value, err := query(ctx)
		if err != nil {
			return nil, err
		}
		// 装箱
		_ = c.SetStore(ctx, key, value, ttl)

		if isNil(value) {
			return nil, ErrNil
		}
		return value, nil
	}

	for _, plugin := range c.plugins {
		plugQuery, ok, err := plugin.InterceptCallQuery(ctx, key, loadQuery)
		if err != nil {
			return nil, err
		}
		loadQuery = plugQuery
		if !ok {
			break
		}
	}
	return loadQuery, nil
}

// NewCacheController 创建一个缓存控制器, 默认使用简单策略模式，设置 15 秒的缓存过期时间
func NewCacheController[T any](name string, store Store, optionChain ...Option[T]) *CacheCtr[T] {
	ctr := &CacheCtr[T]{
		Name:    name,
		plugins: []Plugin{},
		//nolint:mnd
		warp:  EasyPloy(15 * time.Second),
		store: store,
	}
	for _, opt := range optionChain {
		opt(ctr)
	}
	return ctr
}

var (
	ctrStore = sync.Map{}
)

// Wrap 控制器封装方法，创建默认的控制器, 注意 name 只能够对应一个缓存 T 如果，冲突创建，会引发错误
// 该方法默认使用 PolicyWarp 策略,应该使用 NewCacheController 来创建自定义的缓存控制器
// 使用缓存策略 EasyPloy(15 * time.Second)
func Wrap[T any](ctx context.Context, name string, store Store, key string, query Query[T]) (T, error) {
	ctrIntr, ok := ctrStore.Load(name)
	if ok {
		if ctr, ok := ctrIntr.(*CacheCtr[T]); ok {
			return ctr.Wrap(ctx, key, query)
		}
	}

	// 创建并且使用 ctr
	ctrIntr, _ = ctrStore.LoadOrStore(name, NewCacheController[T](name, store))
	if ctr, ok := ctrIntr.(*CacheCtr[T]); ok {
		return ctr.Wrap(ctx, key, query)
	}
	return *new(T), fmt.Errorf("unable to create a new cache controller, named to be used; name:%s, loadedType:%T", name, ctrIntr)
}

// WrapForReuseIgnoreError 重用缓存封装模型, 注意 name 只能够对应一个缓存 T 如果，冲突创建，会引发错误
// 使用缓存策略 ReuseCachePloy(30 * time.Second)
// # 注意如果命中缓存，那么当 query 执行失败时，这个策略会重复使用缓存数据，直到 query 执行成功为止。
func WrapForReuseIgnoreError[T any](ctx context.Context, name string, store Store, key string, query Query[T]) (T, error) {
	const (
		defaultTTL = 30 * time.Second
	)

	ctrIntr, ok := ctrStore.Load(name)
	if ok {
		if ctr, ok := ctrIntr.(*CacheCtr[T]); ok {
			return ctr.Wrap(ctx, key, query)
		}
	}

	// 创建并且使用 ctr
	ctrIntr, _ = ctrStore.LoadOrStore(name, NewCacheController(name, store,
		WithPolicy[T](ReuseCachePloyIgnoreError(defaultTTL)),
	))
	if ctr, ok := ctrIntr.(*CacheCtr[T]); ok {
		return ctr.Wrap(ctx, key, query)
	}
	return *new(T), fmt.Errorf("unable to create a new cache controller, named to be used; name:%s, loadedType:%T", name, ctrIntr)
}

// WrapForFirst 优先缓存封装模型, 注意 name 只能够对应一个缓存 T 如果，冲突创建，会引发错误
// 使用缓存策略 FirstCachePoly(1 * time.Minute)
// # 注意如果命中缓存，那么当 query 执行失败时，这个策略会重复使用缓存数据，直到 query 执行成功为止。
func WrapForFirstIgnoreError[T any](ctx context.Context, name string, store Store, key string, query Query[T]) (T, error) {
	const (
		defaultTTL = 1 * time.Minute
	)

	ctrIntr, ok := ctrStore.Load(name)
	if ok {
		if ctr, ok := ctrIntr.(*CacheCtr[T]); ok {
			return ctr.Wrap(ctx, key, query)
		}
	}

	// 创建并且使用 ctr
	ctrIntr, _ = ctrStore.LoadOrStore(name, NewCacheController(name, store,
		WithPolicy[T](FirstCachePolyIgnoreError(defaultTTL)),
	))
	if ctr, ok := ctrIntr.(*CacheCtr[T]); ok {
		return ctr.Wrap(ctx, key, query)
	}
	return *new(T), fmt.Errorf("unable to create a new cache controller, named to be used; name:%s, loadedType:%T", name, ctrIntr)
}

// WrapForFirstIgnoreErrorWithTTL
// # 注意如果命中缓存，那么当 query 执行失败时，这个策略会重复使用缓存数据，直到 query 执行成功为止。
func WrapForFirstIgnoreErrorWithTTL[T any](ctx context.Context, store Store, key string, ttl time.Duration, query Query[T]) (T, error) {
	name := fmt.Sprintf("library-modecache-first-default-%T", new(T))

	ctrIntr, ok := ctrStore.Load(name)
	if ok {
		if ctr, ok := ctrIntr.(*CacheCtr[T]); ok {
			return ctr.Wrap(ctx, key, query)
		}
	}
	// 创建并且使用 ctr
	ctrIntr, _ = ctrStore.LoadOrStore(name, NewCacheController(name, store,
		WithPolicy[T](FirstCachePolyIgnoreError(ttl)),
	))
	if ctr, ok := ctrIntr.(*CacheCtr[T]); ok {
		return ctr.Wrap(ctx, key, query)
	}
	return *new(T), fmt.Errorf("unable to create a new cache controller, named to be used; name:%s, loadedType:%T", name, ctrIntr)
}

// WrapForReuseIgnoreErrorWithTTL
// # 注意如果命中缓存，那么当 query 执行失败时，这个策略会重复使用缓存数据，直到 query 执行成功为止。
func WrapForReuseIgnoreErrorWithTTL[T any](ctx context.Context, store Store, key string, ttl time.Duration, query Query[T]) (T, error) {
	name := fmt.Sprintf("library-modecache-reuse-default-%T", new(T))

	ctrIntr, ok := ctrStore.Load(name)
	if ok {
		if ctr, ok := ctrIntr.(*CacheCtr[T]); ok {
			return ctr.Wrap(ctx, key, query)
		}
	}
	// 创建并且使用 ctr
	ctrIntr, _ = ctrStore.LoadOrStore(name, NewCacheController(name, store,
		WithPolicy[T](ReuseCachePloyIgnoreError(ttl)),
	))
	if ctr, ok := ctrIntr.(*CacheCtr[T]); ok {
		return ctr.Wrap(ctx, key, query)
	}
	return *new(T), fmt.Errorf("unable to create a new cache controller, named to be used; name:%s, loadedType:%T", name, ctrIntr)
}

// WrapWithTTL 简单的缓存策略，当 query 执行失败时，直接返回错误。
func WrapWithTTL[T any](ctx context.Context, store Store, key string, ttl time.Duration, query Query[T]) (T, error) {
	name := fmt.Sprintf("library-modecache-easy-default-%T", new(T))

	ctrIntr, ok := ctrStore.Load(name)
	if ok {
		if ctr, ok := ctrIntr.(*CacheCtr[T]); ok {
			return ctr.Wrap(ctx, key, query)
		}
	}
	// 创建并且使用 ctr
	ctrIntr, _ = ctrStore.LoadOrStore(name, NewCacheController(name, store,
		WithPolicy[T](EasyPloy(ttl)),
	))
	if ctr, ok := ctrIntr.(*CacheCtr[T]); ok {
		return ctr.Wrap(ctx, key, query)
	}
	return *new(T), fmt.Errorf("unable to create a new cache controller, named to be used; name:%s, loadedType:%T", name, ctrIntr)
}
