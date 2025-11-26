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
	if s.isErr {
		return errors.New("test err")
	}
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

// TestSetStore 测试 SetStore 函数
func TestSetStore(t *testing.T) {
	lc := getTestLocalCache()
	store := NewCacheStore(lc)
	ctx := context.Background()

	type testCase[T any] struct {
		name    string
		key     string
		value   T
		ttl     time.Duration
		wantErr bool
	}

	tests := []testCase[string]{
		{
			name:    "设置字符串缓存-永久",
			key:     "test_string_forever",
			value:   "hello world",
			ttl:     KeepTTL,
			wantErr: false,
		},
		{
			name:    "设置字符串缓存-有过期时间",
			key:     "test_string_ttl",
			value:   "hello ttl",
			ttl:     time.Minute,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 清理之前的缓存
			_ = store.Del(ctx, tt.key)

			err := SetStore(ctx, store, tt.key, tt.value, tt.ttl)
			if (err != nil) != tt.wantErr {
				t.Errorf("SetStore() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// 验证缓存是否设置成功
			if !tt.wantErr {
				got, err := store.Get(ctx, tt.key)
				if err != nil {
					t.Errorf("Failed to get stored value: %v", err)
					return
				}

				// 由于 IsDirectStore() 返回 true，存储的应该是 AbcBox 结构
				box, ok := got.(*AbcBox[string])
				if !ok {
					t.Errorf("Expected *AbcBox[string], got %T", got)
					return
				}

				if box.T != tt.value {
					t.Errorf("SetStore() stored value = %v, want %v", box.T, tt.value)
				}

				// 验证时间戳不为零
				if box.Timestamp == 0 {
					t.Error("SetStore() timestamp should not be zero")
				}
			}
		})
	}

	// 测试其他数据类型
	t.Run("设置整数缓存", func(t *testing.T) {
		key := "test_int"
		value := 42
		_ = store.Del(ctx, key)

		err := SetStore(ctx, store, key, value, time.Minute)
		require.NoError(t, err)

		got, err := store.Get(ctx, key)
		require.NoError(t, err)

		box, ok := got.(*AbcBox[int])
		require.True(t, ok)
		require.Equal(t, value, box.T)
	})

	t.Run("设置结构体缓存", func(t *testing.T) {
		key := "test_struct"
		value := struct {
			Name string
			Age  int
		}{
			Name: "test",
			Age:  25,
		}
		_ = store.Del(ctx, key)

		err := SetStore(ctx, store, key, value, time.Minute)
		require.NoError(t, err)

		got, err := store.Get(ctx, key)
		require.NoError(t, err)

		box, ok := got.(*AbcBox[struct {
			Name string
			Age  int
		}])
		require.True(t, ok)
		require.Equal(t, value, box.T)
	})
}

// TestGetStore 测试 GetStore 函数
func TestGetStore(t *testing.T) {
	lc := getTestLocalCache()
	store := NewCacheStore(lc)
	ctx := context.Background()

	// 先准备一些测试数据
	testString := "hello world"
	testInt := 42
	testStruct := struct {
		Name string
		Age  int
	}{
		Name: "test",
		Age:  25,
	}

	// 设置测试数据
	stringBox := &AbcBox[string]{
		T:         testString,
		Timestamp: int(time.Now().Unix()),
	}
	intBox := &AbcBox[int]{
		T:         testInt,
		Timestamp: int(time.Now().Unix()),
	}
	structBox := &AbcBox[struct {
		Name string
		Age  int
	}]{
		T:         testStruct,
		Timestamp: int(time.Now().Unix()),
	}

	_ = store.Set(ctx, "test_string", stringBox, time.Minute)
	_ = store.Set(ctx, "test_int", intBox, time.Minute)
	_ = store.Set(ctx, "test_struct", structBox, time.Minute)

	type testCase[T any] struct {
		name        string
		key         string
		expectedT   T
		wantErr     bool
		expectZero  bool // 是否期望零值
		compareFunc func(T, T) bool
	}

	tests := []testCase[string]{
		{
			name:        "获取字符串缓存",
			key:         "test_string",
			expectedT:   testString,
			wantErr:     false,
			compareFunc: func(a, b string) bool { return a == b },
		},
		{
			name:        "获取不存在的键",
			key:         "non_existent_key",
			expectedT:   "",
			wantErr:     true,
			expectZero:  true,
			compareFunc: func(a, b string) bool { return a == b },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, timestamp, err := GetStore[string](ctx, store, tt.key)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetStore() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				if !tt.expectZero {
					t.Error("Expected error but got non-zero value")
				}
				return
			}

			if !tt.compareFunc(got, tt.expectedT) {
				t.Errorf("GetStore() = %v, want %v", got, tt.expectedT)
			}

			if timestamp == 0 {
				t.Error("GetStore() timestamp should not be zero")
			}
		})
	}

	// 测试整数类型
	t.Run("获取整数缓存", func(t *testing.T) {
		got, timestamp, err := GetStore[int](ctx, store, "test_int")
		require.NoError(t, err)
		require.Equal(t, testInt, got)
		require.NotZero(t, timestamp)
	})

	// 测试结构体类型
	t.Run("获取结构体缓存", func(t *testing.T) {
		got, timestamp, err := GetStore[struct {
			Name string
			Age  int
		}](ctx, store, "test_struct")
		require.NoError(t, err)
		require.Equal(t, testStruct, got)
		require.NotZero(t, timestamp)
	})
}

// TestSetStoreAndGetStoreIntegration 测试 SetStore 和 GetStore 集成
func TestSetStoreAndGetStoreIntegration(t *testing.T) {
	lc := getTestLocalCache()
	store := NewCacheStore(lc)
	ctx := context.Background()

	type testData struct {
		ID   int
		Name string
	}

	testCases := []struct {
		name  string
		key   string
		value any
		ttl   time.Duration
	}{
		{
			name:  "字符串类型",
			key:   "integration_string",
			value: "integration test",
			ttl:   time.Minute,
		},
		{
			name:  "整数类型",
			key:   "integration_int",
			value: 12345,
			ttl:   time.Minute,
		},
		{
			name:  "布尔类型",
			key:   "integration_bool",
			value: true,
			ttl:   time.Minute,
		},
		{
			name:  "结构体类型",
			key:   "integration_struct",
			value: testData{ID: 1, Name: "test"},
			ttl:   time.Minute,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_ = store.Del(ctx, tc.key)

			switch v := tc.value.(type) {
			case string:
				// 设置
				err := SetStore(ctx, store, tc.key, v, tc.ttl)
				require.NoError(t, err)

				// 获取
				got, timestamp, err := GetStore[string](ctx, store, tc.key)
				require.NoError(t, err)
				require.Equal(t, v, got)
				require.NotZero(t, timestamp)

			case int:
				// 设置
				err := SetStore(ctx, store, tc.key, v, tc.ttl)
				require.NoError(t, err)

				// 获取
				got, timestamp, err := GetStore[int](ctx, store, tc.key)
				require.NoError(t, err)
				require.Equal(t, v, got)
				require.NotZero(t, timestamp)

			case bool:
				// 设置
				err := SetStore(ctx, store, tc.key, v, tc.ttl)
				require.NoError(t, err)

				// 获取
				got, timestamp, err := GetStore[bool](ctx, store, tc.key)
				require.NoError(t, err)
				require.Equal(t, v, got)
				require.NotZero(t, timestamp)

			case testData:
				// 设置
				err := SetStore(ctx, store, tc.key, v, tc.ttl)
				require.NoError(t, err)

				// 获取
				got, timestamp, err := GetStore[testData](ctx, store, tc.key)
				require.NoError(t, err)
				require.Equal(t, v, got)
				require.NotZero(t, timestamp)
			}
		})
	}
}

// TestSetStoreGetStoreWithNilStore 测试使用 nil store 的错误情况
func TestSetStoreGetStoreWithNilStore(t *testing.T) {
	ctx := context.Background()

	// 使用一个总是返回错误的 mock store
	errorStore := snakeCache{
		result: nil,
		isErr:  true,
	}

	t.Run("SetStore with error store", func(t *testing.T) {
		err := SetStore(ctx, errorStore, "test_key", "test_value", time.Minute)
		require.Error(t, err)
	})

	t.Run("GetStore with error store", func(t *testing.T) {
		got, timestamp, err := GetStore[string](ctx, errorStore, "test_key")
		require.Error(t, err)
		require.Equal(t, "", got) // 零值
		require.Equal(t, 0, timestamp)
	})
}
