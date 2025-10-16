package modecache

import (
	"context"
	"log"
)

// CtxTraceIDKey 上下文中用于存储 trace ID 的键
type CtxTraceIDKey struct{}

// WithTraceID 将 trace ID 添加到 context 中
func WithTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, CtxTraceIDKey{}, id)
}

// GetTraceID 从 context 中获取 trace ID，如果不存在返回空字符串
func GetTraceID(ctx context.Context) string {
	if id, ok := ctx.Value(CtxTraceIDKey{}).(string); ok {
		return id
	}
	return ""
}

// LogDebugf 输出 Debug 级别日志，包含可选的 trace ID
func LogDebugf(ctx context.Context, format string, args ...any) {
	traceID := GetTraceID(ctx)
	if traceID != "" {
		log.Printf("[DEBUG][trace:%s] "+format, append([]any{traceID}, args...)...)
	} else {
		log.Printf("[DEBUG] "+format, args...)
	}
}

// LogInfof 输出 Info 级别日志，包含可选的 trace ID
func LogInfof(ctx context.Context, format string, args ...any) {
	traceID := GetTraceID(ctx)
	if traceID != "" {
		log.Printf("[INFO][trace:%s] "+format, append([]any{traceID}, args...)...)
	} else {
		log.Printf("[INFO] "+format, args...)
	}
}

// LogErrorf 输出 Error 级别日志，包含可选的 trace ID
func LogErrorf(ctx context.Context, format string, args ...any) {
	traceID := GetTraceID(ctx)
	if traceID != "" {
		log.Printf("[ERROR][trace:%s] "+format, append([]any{traceID}, args...)...)
	} else {
		log.Printf("[ERROR] "+format, args...)
	}
}
