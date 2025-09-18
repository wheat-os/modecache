package modecache

import (
	"hash/crc32"
	"reflect"
	"time"
)

func usePrecise(dur time.Duration) bool {
	return dur < time.Second || dur%time.Second != 0
}

// 检查是否使用 毫秒
func formatMs(dur time.Duration) int64 {
	if dur > 0 && dur < time.Millisecond {
		return 1
	}
	return int64(dur / time.Millisecond)
}

// 检查使用秒
func formatSec(dur time.Duration) int64 {
	if dur > 0 && dur < time.Second {
		return 1
	}
	return int64(dur / time.Second)
}

func isNil(v any) bool {
	return v == nil || (reflect.ValueOf(v).Kind() == reflect.Ptr && reflect.ValueOf(v).IsNil())
}

func hashCrc32ToUint(key string) uint {
	crc := crc32.NewIEEE()
	_, _ = crc.Write([]byte(key))
	return uint(crc.Sum32())
}

func GO(fn func()) {
	go func() {
		fn()
	}()
}
