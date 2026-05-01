package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/bits-and-blooms/bloom/v3"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/wanwanzi6/short-link/internal/model"
)

// benchmarkContext 压测上下文
type benchmarkContext struct {
	db     *gorm.DB
	rdb    *redis.Client
	filter *bloom.BloomFilter
	mr     *miniredis.Miniredis
}

// setupBenchmarkContext 初始化压测环境
func setupBenchmarkContext() (*benchmarkContext, func()) {
	ctx := &benchmarkContext{}

	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db.AutoMigrate(&model.URL{})
	ctx.db = db

	mr, _ := miniredis.Run()
	ctx.mr = mr

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx.rdb = rdb

	ctx.filter = bloom.NewWithEstimates(1000000, 0.0001)

	return ctx, func() {
		mr.Close()
		rdb.Close()
	}
}

// prepareBenchmarkData 准备测试数据
func prepareBenchmarkData(ctx *benchmarkContext, addToFilter bool) []string {
	codes := make([]string, 0, 100)
	for i := 0; i < 100; i++ {
		code := fmt.Sprintf("bench%d", i)
		ctx.db.Create(&model.URL{
			OriginalURL: fmt.Sprintf("https://example.com/item/%d", i),
			ShortCode:   code,
		})
		if addToFilter {
			ctx.filter.Add([]byte(code))
		}
		codes = append(codes, code)
	}
	return codes
}

// createTestEngineWithBloom 创建使用布隆过滤器的 Gin 引擎
func createTestEngineWithBloom(svc *URLService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/:short_code", func(c *gin.Context) {
		code := c.Param("short_code")
		longURL, err := svc.GetOriginalURL(code)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		_ = svc.UpdateClickCount(code)
		c.Redirect(http.StatusFound, longURL)
	})
	return r
}

// createTestEngineWithoutBloom 创建不使用布隆过滤器的 Gin 引擎
// 模拟真实场景：先查 Redis -> 查 DB -> 返回结果
func createTestEngineWithoutBloom(svc *URLService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/:short_code", func(c *gin.Context) {
		code := c.Param("short_code")
		ctx := context.Background()

		// 1. 先查 Redis
		longURL, err := svc.rdb.Get(ctx, cacheKey(code)).Result()
		if err == nil {
			_ = svc.UpdateClickCount(code)
			c.Redirect(http.StatusFound, longURL)
			return
		}

		// 2. Redis 未命中，查数据库
		var url model.URL
		if err := svc.db.Where("short_code = ?", code).First(&url).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}

		// 3. 异步回写 Redis
		go svc.rdb.Set(context.Background(), cacheKey(code), url.OriginalURL, 24*time.Hour)
		_ = svc.UpdateClickCount(code)
		c.Redirect(http.StatusFound, url.OriginalURL)
	})
	return r
}

// BenchmarkRedirectWithBloom 测试有布隆过滤器的重定向性能
//
// 场景：95% 请求访问不存在的短码
// 预期：布隆过滤器快速拦截无效请求，不查 Redis/数据库
func BenchmarkRedirectWithBloom(b *testing.B) {
	ctx, cleanup := setupBenchmarkContext()
	defer cleanup()

	codes := prepareBenchmarkData(ctx, true) // 添加到布隆过滤器
	svc := NewURLService(ctx.db, ctx.rdb, ctx.filter)
	r := createTestEngineWithBloom(svc)

	// 预热
	req := httptest.NewRequest(http.MethodGet, "/"+codes[0], nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	b.ResetTimer()

	var wg sync.WaitGroup
	concurrency := runtime.NumCPU()
	perWorker := b.N / concurrency

	for worker := 0; worker < concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				var code string
				if i%100 < 95 { // 95% 不存在
					code = fmt.Sprintf("nonexist_%d", i)
				} else {
					code = codes[i%len(codes)]
				}
				req := httptest.NewRequest(http.MethodGet, "/"+code, nil)
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
			}
		}()
	}

	wg.Wait()
}

// BenchmarkRedirectWithoutBloom 测试无布隆过滤器的重定向性能
//
// 场景：95% 请求访问不存在的短码
// 预期：无布隆过滤器时，请求先到 Redis（未命中），再穿透到数据库，性能大幅下降
//
// 关键点：WithoutBloom 版本每次请求都会完整走 Redis -> DB 流程
// 这样才能真实对比布隆过滤器的拦截价值
func BenchmarkRedirectWithoutBloom(b *testing.B) {
	ctx, cleanup := setupBenchmarkContext()
	defer cleanup()

	codes := prepareBenchmarkData(ctx, false) // 不添加到布隆过滤器
	svc := NewURLService(ctx.db, ctx.rdb, ctx.filter)
	r := createTestEngineWithoutBloom(svc)

	// 预热
	req := httptest.NewRequest(http.MethodGet, "/"+codes[0], nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	b.ResetTimer()

	var wg sync.WaitGroup
	concurrency := runtime.NumCPU()
	perWorker := b.N / concurrency

	for worker := 0; worker < concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				var code string
				if i%100 < 95 { // 95% 不存在
					code = fmt.Sprintf("nonexist_%d", i)
				} else {
					code = codes[i%len(codes)]
				}
				req := httptest.NewRequest(http.MethodGet, "/"+code, nil)
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
			}
		}()
	}

	wg.Wait()
}

// BenchmarkRedirectCacheHit 测试缓存命中场景
//
// 场景：所有请求都访问存在的短码，Redis 缓存命中
// 预期：QPS 非常高，延迟极低
func BenchmarkRedirectCacheHit(b *testing.B) {
	ctx, cleanup := setupBenchmarkContext()
	defer cleanup()

	codes := prepareBenchmarkData(ctx, true)
	svc := NewURLService(ctx.db, ctx.rdb, ctx.filter)
	r := createTestEngineWithBloom(svc)

	// 预热：建立 Redis 缓存
	for _, code := range codes {
		req := httptest.NewRequest(http.MethodGet, "/"+code, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}

	b.ResetTimer()

	var wg sync.WaitGroup
	concurrency := runtime.NumCPU()
	perWorker := b.N / concurrency

	for worker := 0; worker < concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				code := codes[i%len(codes)]
				req := httptest.NewRequest(http.MethodGet, "/"+code, nil)
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
			}
		}()
	}

	wg.Wait()
}

// BenchmarkRedirectMixed 混合场景测试
//
// 场景：50% 存在请求，50% 不存在请求
// 预期：介于纯缓存命中和纯缓存穿透之间
func BenchmarkRedirectMixed(b *testing.B) {
	ctx, cleanup := setupBenchmarkContext()
	defer cleanup()

	codes := prepareBenchmarkData(ctx, true)
	svc := NewURLService(ctx.db, ctx.rdb, ctx.filter)
	r := createTestEngineWithBloom(svc)

	// 预热
	req := httptest.NewRequest(http.MethodGet, "/"+codes[0], nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	b.ResetTimer()

	var wg sync.WaitGroup
	concurrency := runtime.NumCPU()
	perWorker := b.N / concurrency

	for worker := 0; worker < concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				var code string
				if i%2 == 0 {
					code = codes[i%len(codes)]
				} else {
					code = fmt.Sprintf("nonexist_%d", i)
				}
				req := httptest.NewRequest(http.MethodGet, "/"+code, nil)
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
			}
		}()
	}

	wg.Wait()
}