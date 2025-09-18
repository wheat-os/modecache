package modecache

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/spf13/cast"
	"github.com/stretchr/testify/require"
)

var testQueryCount = 0

func testQuery(value any) func(ctx context.Context) (any, error) {
	return func(ctx context.Context) (any, error) {
		testQueryCount++
		if _, ok := value.(error); ok {
			return nil, fmt.Errorf("error")
		}
		return value, nil
	}
}

func getTestLocalCache() *cache.Cache {
	cache := cache.New(cache.NoExpiration, 5*time.Minute)
	return cache
}

func testCacheCtr(w Policy) *CacheCtr[any] {
	lc := getTestLocalCache()
	store := NewCacheStore(lc)
	return &CacheCtr[any]{
		store: store,
		warp:  w,
	}
}

func testCtrByStore(w Policy, store Store) *CacheCtr[any] {
	return &CacheCtr[any]{
		store: store,
		warp:  w,
	}
}

func TestCacheTypeValueCtr(t *testing.T) {
	redisStore, c := getRedis()
	defer c()

	lc := getTestLocalCache()

	type args struct {
		store Store
	}

	tests := []struct {
		name string
		args args
	}{
		{
			name: "test-redis",
			args: args{
				store: redisStore,
			},
		},
		{
			name: "test-localCache",
			args: args{
				store: NewCacheStore(lc),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// bool
			tt.args.store.Del(context.Background(), "test")
			ctr := NewCacheController[bool]("test", tt.args.store)
			b, err := ctr.Wrap(context.Background(), "test", func(ctx context.Context) (bool, error) {
				return true, nil
			})
			require.NoError(t, err)
			require.Equal(t, true, b)

			// 实现 int， string，结构体，结构体指针的测试
			tt.args.store.Del(context.Background(), "test")
			ctrInt := NewCacheController[int]("test", tt.args.store)
			rInt, err := ctrInt.Wrap(context.Background(), "test", func(ctx context.Context) (int, error) {
				return 1, nil
			})
			require.NoError(t, err)
			require.Equal(t, 1, rInt)

			// float
			tt.args.store.Del(context.Background(), "test")
			ctrFloat := NewCacheController[float64]("test", tt.args.store)
			rFloat, err := ctrFloat.Wrap(context.Background(), "test", func(ctx context.Context) (float64, error) {
				return 1.1, nil
			})
			require.NoError(t, err)
			require.Equal(t, 1.1, rFloat)

			// string
			tt.args.store.Del(context.Background(), "test")
			ctrString := NewCacheController[string]("test", tt.args.store)
			rString, err := ctrString.Wrap(context.Background(), "test", func(ctx context.Context) (string, error) {
				return "test", nil
			})
			require.NoError(t, err)
			require.Equal(t, "test", rString)

			// struct
			tt.args.store.Del(context.Background(), "test")
			ctrStruct := NewCacheController[struct{ test string }]("test", tt.args.store)
			rStruct, err := ctrStruct.Wrap(context.Background(), "test", func(ctx context.Context) (struct{ test string }, error) {
				return struct{ test string }{"test"}, nil
			})
			require.NoError(t, err)
			require.EqualValues(t, struct{ test string }{"test"}, rStruct)

			// struct point
			tt.args.store.Del(context.Background(), "test")
			ctrStructPtr := NewCacheController[*struct{ test string }]("test", tt.args.store)
			rStructPtr, err := ctrStructPtr.Wrap(context.Background(), "test", func(ctx context.Context) (*struct{ test string }, error) {
				return &struct{ test string }{"test"}, nil
			})
			require.NoError(t, err)
			require.EqualValues(t, &struct{ test string }{"test"}, rStructPtr)
		})
	}
}

func TestControlWrapForCtr(t *testing.T) {
	store, c := getRedis()
	defer c()

	type args struct {
		query     Query[any]
		cacheCtr  *CacheCtr[any]
		cycles    int           // 循环次数
		sleepTime time.Duration // 每次等待时间
	}
	tests := []struct {
		name       string
		args       args
		want       any
		queryCount int // 查询 query 次数
		wantErr    bool
	}{
		{
			name: "PolicyWarp 正确用例-字符串,查库 2 次",
			args: args{
				query:     testQuery("test"),
				cacheCtr:  testCacheCtr(EasyPloy(time.Millisecond * 100)),
				cycles:    5,
				sleepTime: time.Millisecond * 30,
			},
			want:       "test",
			wantErr:    false,
			queryCount: 2,
		},
		{
			name: "PolicyWarp 正确用例-bool,查库 2 次",
			args: args{
				query:     testQuery(true),
				cacheCtr:  testCtrByStore(EasyPloy(time.Millisecond*100), store),
				cycles:    5,
				sleepTime: time.Millisecond * 30,
			},
			want:       true,
			wantErr:    false,
			queryCount: 2,
		},
		{
			name: "ReuseCacheWarp 正确用例-float,查库 1 次",
			args: args{
				query:     testQuery(0.12),
				cacheCtr:  testCtrByStore(ReuseCachePloyIgnoreError(2*time.Second), store),
				cycles:    5,
				sleepTime: time.Millisecond * 5,
			},
			want:       0.12,
			wantErr:    false,
			queryCount: 1,
		},
		{
			name: "ReuseCacheWarp 正确用例-int,查库 1 次",
			args: args{
				query:     testQuery(2),
				cacheCtr:  testCacheCtr(ReuseCachePloyIgnoreError(2 * time.Second)),
				cycles:    5,
				sleepTime: time.Millisecond * 5,
			},
			want:       2,
			wantErr:    false,
			queryCount: 1,
		},
		{
			name: "ReuseCacheWarp 正确用例-string,查库 1 次",
			args: args{
				query:     testQuery("test-string"),
				cacheCtr:  testCacheCtr(FirstCachePolyIgnoreError(2 * time.Second)),
				cycles:    5,
				sleepTime: time.Millisecond * 5,
			},
			want:       "test-string",
			wantErr:    false,
			queryCount: 1,
		},
		{
			name: "ReuseCacheWarp 正确用例-结构体,查库 1 次",
			args: args{
				query:     testQuery(AbcBox[string]{T: "test-string"}),
				cacheCtr:  testCacheCtr(FirstCachePolyIgnoreError(2 * time.Second)),
				cycles:    5,
				sleepTime: time.Millisecond * 5,
			},
			want:       AbcBox[string]{T: "test-string"},
			wantErr:    false,
			queryCount: 1,
		},
		{
			name: "ReuseCacheWarp 正确用例-指针,查库 1 次",
			args: args{
				query:     testQuery(&AbcBox[string]{T: "test-string"}),
				cacheCtr:  testCacheCtr(FirstCachePolyIgnoreError(2 * time.Second)),
				cycles:    5,
				sleepTime: time.Millisecond * 5,
			},
			want:       &AbcBox[string]{T: "test-string"},
			wantErr:    false,
			queryCount: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testQueryCount = 0
			_ = store.Del(context.Background(), "test")

			for i := 0; i < tt.args.cycles; i++ {
				got, err := tt.args.cacheCtr.Wrap(context.Background(), "test", tt.args.query)
				if (err != nil) != tt.wantErr {
					t.Errorf("Wrap() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				require.EqualValues(t, got, tt.want)
				if tt.args.sleepTime > 0 {
					time.Sleep(tt.args.sleepTime)
				}
			}

			require.LessOrEqual(t, testQueryCount, tt.queryCount)
		})
	}
}

func TestControlWrap(t *testing.T) {
	rdsStore, c := getRedis()
	defer c()
	lc := getTestLocalCache()
	cStore := NewCacheStore(lc)

	type args struct {
		query     Query[any]
		store     Store
		name      string
		cycles    int           // 循环次数
		sleepTime time.Duration // 每次等待时间
	}
	tests := []struct {
		name       string
		args       args
		want       any
		queryCount int // 查询 query 次数
		wantErr    bool
	}{
		{
			name: "PolicyWarp 正确用例-字符串,查库 1 次",
			args: args{
				query:     testQuery("test"),
				cycles:    5,
				sleepTime: time.Millisecond * 30,
				store:     cStore,
				name:      "c",
			},
			want:       "test",
			wantErr:    false,
			queryCount: 1,
		},
		{
			name: "PolicyWarp 正确用例-字符串,查库 1 次, Redis 缓存",
			args: args{
				query:     testQuery("test"),
				cycles:    5,
				sleepTime: time.Millisecond * 30,
				store:     rdsStore,
				name:      "r",
			},
			want:       "test",
			wantErr:    false,
			queryCount: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testQueryCount = 0
			_ = rdsStore.Del(context.Background(), "test")
			_ = cStore.Del(context.Background(), "test")

			for i := 0; i < tt.args.cycles; i++ {
				got, err := Wrap(context.Background(), tt.args.name, cStore, "test", tt.args.query)
				if (err != nil) != tt.wantErr {
					t.Errorf("Wrap() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("Wrap() = %v, want %v", got, tt.want)
				}
				if tt.args.sleepTime > 0 {
					time.Sleep(tt.args.sleepTime)
				}
			}

			require.Equal(t, testQueryCount, tt.queryCount)
		})
	}
}

type snakeCache struct {
	result any
	isErr  bool
}

func (s snakeCache) Get(ctx context.Context, key string) (any, error) {
	if s.isErr {
		return s.result, errors.New("test err")
	}
	return s.result, nil
}

func (s snakeCache) Set(ctx context.Context, key string, data any, ttl time.Duration) error {
	return nil
}

func (s snakeCache) Del(ctx context.Context, key string) error {
	return nil
}

func (s snakeCache) IsDirectStore() bool {
	return true
}

func TestControlWrapErrorTest(t *testing.T) {
	rdsStore, c := getRedis()
	defer c()

	lc := getTestLocalCache()
	lcStore := NewCacheStore(lc)

	testErr := errors.New("test error")
	testTTLSecond := 1 * time.Second

	type args struct {
		policy Policy
		store  Store
	}
	tests := []struct {
		name           string
		args           args
		wantErr        error
		queryFailErr   error // query 读取失败后的 error
		notCacheKeyErr error // 缓存键不存在且 query 读取失败的 error
	}{
		{
			name: "EasyPloy-localCache",
			args: args{
				policy: EasyPloy(testTTLSecond),
				store:  lcStore,
			},
			queryFailErr:   testErr,
			notCacheKeyErr: testErr,
		},
		{
			name: "EasyPloy-redisCache",
			args: args{
				policy: EasyPloy(testTTLSecond),
				store:  rdsStore,
			},
			queryFailErr:   testErr,
			notCacheKeyErr: testErr,
		},
		{
			name: "FirstCache-redisCache",
			args: args{
				policy: FirstCachePolyIgnoreError(testTTLSecond),
				store:  rdsStore,
			},
			queryFailErr:   nil,
			notCacheKeyErr: testErr,
		},
		{
			name: "FirstCache-localCache",
			args: args{
				policy: FirstCachePolyIgnoreError(testTTLSecond),
				store:  lcStore,
			},
			queryFailErr:   nil,
			notCacheKeyErr: testErr,
		},
		{
			name: "ReuseCache-redisCache",
			args: args{
				policy: ReuseCachePloyIgnoreError(testTTLSecond),
				store:  rdsStore,
			},
			queryFailErr:   nil,
			notCacheKeyErr: testErr,
		},
		{
			name: "ReuseCache-localCache",
			args: args{
				policy: ReuseCachePloyIgnoreError(testTTLSecond),
				store:  lcStore,
			},
			queryFailErr:   nil,
			notCacheKeyErr: testErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctr := NewCacheController[int](tt.name, tt.args.store, WithPolicy[int](tt.args.policy))
			_ = tt.args.store.Del(context.Background(), "test_error")

			// 读取正确情况
			res, err := ctr.Wrap(context.Background(), "test_error", func(ctx context.Context) (int, error) {
				return 1, nil
			})
			require.ErrorIs(t, err, tt.wantErr)
			require.Equal(t, res, 1)

			// 读取错误缓存未过期
			res, err = ctr.Wrap(context.Background(), "test_error", func(ctx context.Context) (int, error) {
				return 1, testErr
			})
			require.ErrorIs(t, err, tt.wantErr)
			require.Equal(t, res, 1)

			// 缓存丢失后异常
			_ = tt.args.store.Del(context.Background(), "test_error")
			_, err = ctr.Wrap(context.Background(), "test_error", func(ctx context.Context) (int, error) {
				return 1, testErr
			})
			require.ErrorIs(t, err, tt.notCacheKeyErr)

			// 恢复正常读取
			res, err = ctr.Wrap(context.Background(), "test_error", func(ctx context.Context) (int, error) {
				return 1, nil
			})
			require.ErrorIs(t, err, tt.wantErr)
			require.Equal(t, res, 1)
			res, err = ctr.Wrap(context.Background(), "test_error", func(ctx context.Context) (int, error) {
				return 1, testErr
			})
			require.ErrorIs(t, err, tt.wantErr)
			require.Equal(t, res, 1)

			// 测试 cache Error 的场景
			ctr.store = snakeCache{
				result: 1,
				isErr:  true,
			}
			res, err = ctr.Wrap(context.Background(), "test_error", func(ctx context.Context) (int, error) {
				return 1, nil
			})
			require.ErrorIs(t, err, tt.wantErr)
			require.Equal(t, res, 1)
		})
	}
}

func TestControlWrapConcurrency(t *testing.T) {
	lc := getTestLocalCache()
	lcStore := NewCacheStore(lc)

	testTTLSecond := 1 * time.Second

	type args struct {
		policy Policy
		store  Store
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "EasyPloy-localCache",
			args: args{
				policy: EasyPloy(testTTLSecond),
				store:  lcStore,
			},
		},
		{
			name: "FirstCache-localCache",
			args: args{
				policy: FirstCachePolyIgnoreError(testTTLSecond),
				store:  lcStore,
			},
		},
		{
			name: "ReuseCache-localCache",
			args: args{
				policy: ReuseCachePloyIgnoreError(testTTLSecond),
				store:  lcStore,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctr := NewCacheController[int](tt.name, tt.args.store, WithPolicy[int](tt.args.policy))
			_ = tt.args.store.Del(context.Background(), "test_error")

			goNum := 10
			execNum := 1000

			// 秒内执行 query 测试
			wg := sync.WaitGroup{}
			var queryCount int64 = 0
			for i := 0; i < goNum; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for j := 0; j < execNum; j++ {
						msg, err := ctr.Wrap(context.Background(), "test_error", func(ctx context.Context) (int, error) {
							atomic.AddInt64(&queryCount, 1)
							return 1, nil
						})
						require.NoError(t, err)
						require.Equal(t, msg, 1)
					}
				}()
			}
			wg.Wait()

			require.LessOrEqual(t, queryCount, int64(2))
		})
	}
}

func TestUseContextStoreWrapConcurrency(t *testing.T) {
	rds, cleanup := getTestRedis()
	defer cleanup()
	testTTLSecond := 10 * time.Second

	tests := []struct {
		name     string
		wrapFunc func(ctx context.Context, store Store, key string, ttl time.Duration, query Query[int]) (int, error)
	}{
		{
			name:     "EasyPloy-localCache",
			wrapFunc: WrapWithTTL[int],
		},
		{
			name:     "FirstPloy-localCache",
			wrapFunc: WrapForFirstIgnoreErrorWithTTL[int],
		},
		{
			name:     "ReusePloy-localCache",
			wrapFunc: WrapForReuseIgnoreErrorWithTTL[int],
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			goNum := 10
			execNum := 1000
			mod := 5

			// 秒内执行 query 测试
			wg := sync.WaitGroup{}
			var queryCount int64 = 0
			for i := 0; i < goNum; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for j := 0; j < execNum; j++ {
						ctx, store := NewRedisHashStore(context.Background(), rds, "test-key", cast.ToString(j%mod))
						msg, err := tt.wrapFunc(ctx, store, fmt.Sprintf("test-key-%d", j%mod), testTTLSecond, func(ctx context.Context) (int, error) {
							atomic.AddInt64(&queryCount, 1)
							return j % mod, nil
						})
						require.NoError(t, err)
						require.Equal(t, msg, j%5)
					}
				}()
			}
			wg.Wait()
			require.LessOrEqual(t, queryCount, int64(mod))
		})
	}

}
