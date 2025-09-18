package modecache

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/time/rate"
)

// 使用 go 限流器实现 query 访问限流插件
type LimitQueryPlugin struct {
	limit *rate.Limiter
}

// DB 限流器
func (m *LimitQueryPlugin) InterceptCallQuery(ctx context.Context, key string, loadQuery LoadingForQuery) (LoadingForQuery, bool, error) {
	// 等待限流器
	return func(ctx context.Context, key string, ttl time.Duration) (any, error) {
		if err := m.limit.Wait(ctx); err != nil {
			return nil, err
		}
		return loadQuery(ctx, key, ttl)
	}, true, nil
}

func (m *LimitQueryPlugin) InterceptCallCache(ctx context.Context, key string, loadCache LoadingForCache) (LoadingForCache, bool, error) {
	return loadCache, true, nil
}

func NewLimitQueryPlugin(r rate.Limit, b int) Plugin {
	return &LimitQueryPlugin{
		limit: rate.NewLimiter(r, b),
	}
}

var (
	_metricControllerCallCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "cache",
		Subsystem: "modecache",
		Name:      "modecache_controller_count",
		Help:      "Count the number of accesses to the  mode controller",
	}, []string{"name", "query", "error"})

	_metricControllerCallSeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "cache",
		Subsystem: "modecache",
		Name:      "modecache_controller_sec",
		Help:      "mode cache duration(sec).",
		Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.250, 0.5, 1},
	}, []string{"name", "query", "error"})
)

// MetricsPlugin 指标插件
type MetricsPlugin struct {
	name string
}

func (m *MetricsPlugin) InterceptCallQuery(ctx context.Context, key string, loadQuery LoadingForQuery) (LoadingForQuery, bool, error) {
	return func(ctx context.Context, key string, ttl time.Duration) (any, error) {
		startTime := time.Now()
		isTest := "0"
		value, err := loadQuery(ctx, key, ttl)
		isError := "0"
		if err != nil {
			isError = "1"
		}

		_metricControllerCallCount.WithLabelValues(m.name, isTest, "1", isError).Inc()
		_metricControllerCallSeconds.WithLabelValues(m.name, isTest, "1", isError).Observe(time.Since(startTime).Seconds())

		return value, err
	}, true, nil
}

func (m *MetricsPlugin) InterceptCallCache(ctx context.Context, key string, loadCache LoadingForCache) (LoadingForCache, bool, error) {
	return func(ctx context.Context, key string) (any, int, error) {
		startTime := time.Now()
		value, dataTime, err := loadCache(ctx, key)
		isError := "0"
		if err != nil {
			isError = "1"
		}
		_metricControllerCallCount.WithLabelValues(m.name, "0", isError).Inc()
		_metricControllerCallSeconds.WithLabelValues(m.name, "0", isError).Observe(time.Since(startTime).Seconds())

		return value, dataTime, err
	}, true, nil
}

func NewMetricsPlugin(name string) Plugin {
	return &MetricsPlugin{
		name: name,
	}
}
