package modecache

import (
	"context"
	"time"
)

type Option[T any] func(m *CacheCtr[T])

// WithPolicy 设置需要使用的缓存策略
func WithPolicy[T any](p Policy) Option[T] {
	return func(m *CacheCtr[T]) {
		m.warp = p
	}
}

// WithAddPlugin 设置想要使用的缓存插件
func WithPlugins[T any](p ...Plugin) Option[T] {
	return func(m *CacheCtr[T]) {
		m.plugins = append(m.plugins, p...)
	}
}

type TaskResult[T any] struct {
	Key string        // 缓存 Key
	T   T             // 缓存内容
	TTL time.Duration // 缓存的过期时间, KeepTTL 永久存储
}

type TimerJobList[T any] func(ctx context.Context) ([]*TaskResult[T], error)
