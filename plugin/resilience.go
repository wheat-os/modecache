package plugin

import (
	"context"
	"sync"
	"time"

	"github.com/wheat-os/modecache"
)

// cstate 表示单个键的熔断器状态
type cstate struct {
	failures      int       // 连续失败次数
	circuitOpen   bool      // 熔断器是否打开
	openedAt      time.Time // 熔断器打开时间
	mu            sync.Mutex
	resetAfter    time.Duration // 熔断器重置时间
	failThreshold int           // 失败阈值
}

// ResiliencePlugin 实现 Plugin 接口，提供重试和熔断功能
type ResiliencePlugin struct {
	MaxRetries              int           // 最大重试次数
	Backoff                 time.Duration // 重试间隔
	CircuitFailureThreshold int           // 熔断器失败阈值
	CircuitResetAfter       time.Duration // 熔断器重置时间
	mu                      sync.Mutex
	circuits                map[string]*cstate // 每个键的熔断器状态
}

// NewResiliencePlugin 创建一个新的 ResiliencePlugin
func NewResiliencePlugin(maxRetries int, backoff time.Duration, circuitFailureThreshold int, circuitResetAfter time.Duration) *ResiliencePlugin {
	return &ResiliencePlugin{
		MaxRetries:              maxRetries,
		Backoff:                 backoff,
		CircuitFailureThreshold: circuitFailureThreshold,
		CircuitResetAfter:       circuitResetAfter,
		circuits:                make(map[string]*cstate),
	}
}

// getCircuitState 获取或创建指定键的熔断器状态
func (r *ResiliencePlugin) getCircuitState(key string) *cstate {
	r.mu.Lock()
	defer r.mu.Unlock()

	if state, exists := r.circuits[key]; exists {
		return state
	}

	state := &cstate{
		failures:      0,
		circuitOpen:   false,
		resetAfter:    r.CircuitResetAfter,
		failThreshold: r.CircuitFailureThreshold,
	}
	r.circuits[key] = state
	return state
}

// checkCircuit 检查熔断器状态，如果已打开且未到重置时间则返回 true
func (r *ResiliencePlugin) checkCircuit(state *cstate) bool {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.circuitOpen {
		// 检查是否到了重置时间
		if time.Since(state.openedAt) >= state.resetAfter {
			state.circuitOpen = false
			state.failures = 0
			return false
		}
		return true
	}
	return false
}

// recordFailure 记录失败，如果达到阈值则打开熔断器
func (r *ResiliencePlugin) recordFailure(state *cstate) {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.failures++
	if state.failures >= state.failThreshold {
		state.circuitOpen = true
		state.openedAt = time.Now()
	}
}

// recordSuccess 记录成功，重置失败计数
func (r *ResiliencePlugin) recordSuccess(state *cstate) {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.failures = 0
	state.circuitOpen = false
}

// InterceptCallCache 拦截缓存调用，实现重试和熔断功能
func (r *ResiliencePlugin) InterceptCallCache(ctx context.Context, key string, loadCache modecache.LoadingForCache) (modecache.LoadingForCache, bool, error) {
	wrapped := func(ctx context.Context, key string) (any, int, error) {
		state := r.getCircuitState(key)

		// 检查熔断器是否打开
		if r.checkCircuit(state) {
			return nil, 0, modecache.ErrCircuitOpen
		}

		var lastErr error
		// 尝试执行，包括重试
		for attempt := 0; attempt <= r.MaxRetries; attempt++ {
			if attempt > 0 {
				// 在重试之前等待
				time.Sleep(r.Backoff)
			}

			value, timestamp, err := loadCache(ctx, key)
			if err == nil {
				// 成功，重置熔断器状态
				r.recordSuccess(state)
				return value, timestamp, nil
			}

			lastErr = err
		}

		// 所有重试都失败了，记录失败
		r.recordFailure(state)
		return nil, 0, lastErr
	}

	return wrapped, true, nil
}

// InterceptCallQuery 拦截查询调用，实现重试和熔断功能
func (r *ResiliencePlugin) InterceptCallQuery(ctx context.Context, key string, loadQuery modecache.LoadingForQuery) (modecache.LoadingForQuery, bool, error) {
	wrapped := func(ctx context.Context, key string, ttl time.Duration) (any, error) {
		state := r.getCircuitState(key)

		// 检查熔断器是否打开
		if r.checkCircuit(state) {
			return nil, modecache.ErrCircuitOpen
		}

		var lastErr error
		// 尝试执行，包括重试
		for attempt := 0; attempt <= r.MaxRetries; attempt++ {
			if attempt > 0 {
				// 在重试之前等待
				time.Sleep(r.Backoff)
			}

			value, err := loadQuery(ctx, key, ttl)
			if err == nil {
				// 成功，重置熔断器状态
				r.recordSuccess(state)
				return value, nil
			}

			lastErr = err
		}

		// 所有重试都失败了，记录失败
		r.recordFailure(state)
		return nil, lastErr
	}

	return wrapped, true, nil
}
