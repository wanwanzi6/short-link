package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/bits-and-blooms/bloom/v3"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/wanwanzi6/short-link/internal/model"
	"github.com/wanwanzi6/short-link/internal/response"
	"github.com/wanwanzi6/short-link/internal/service"
)

// benchmarkCtx holds test context
type benchmarkCtx struct {
	db     *gorm.DB
	rdb    *redis.Client
	filter *bloom.BloomFilter
	mr     *miniredis.Miniredis
}

// setupBenchCtx creates benchmark context
func setupBenchCtx() (*benchmarkCtx, func()) {
	ctx := &benchmarkCtx{}
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

// noPoolHandler mimics original handler without sync.Pool
type noPoolHandler struct {
	svc *service.URLService
}

func (h *noPoolHandler) ShortenURL(c *gin.Context) {
	var req ShortenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}
	if req.LongURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "long_url cannot be empty"})
		return
	}
	code, err := h.svc.ShortenURL(req.LongURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to shorten URL: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"short_code": code})
}

func (h *noPoolHandler) Redirect(c *gin.Context) {
	shortCode := c.Param("short_code")
	if shortCode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "short_code is required"})
		return
	}
	longURL, err := h.svc.GetOriginalURL(shortCode)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "short code not found"})
		return
	}
	_ = h.svc.UpdateClickCount(shortCode)
	c.Redirect(http.StatusFound, longURL)
}

// PoolHandler uses sync.Pool
type PoolHandler struct {
	svc *service.URLService
}

func (h *PoolHandler) ShortenURL(c *gin.Context) {
	var req ShortenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp := response.GetBadRequestResponse()
		resp.Error = "invalid request: " + err.Error()
		c.JSON(resp.Code(), resp)
		response.PutBadRequestResponse(resp)
		return
	}
	if req.LongURL == "" {
		resp := response.GetBadRequestResponse()
		resp.Error = "long_url cannot be empty"
		c.JSON(resp.Code(), resp)
		response.PutBadRequestResponse(resp)
		return
	}
	code, err := h.svc.ShortenURL(req.LongURL)
	if err != nil {
		resp := response.GetErrorResponse()
		resp.Error = "failed to shorten URL: " + err.Error()
		c.JSON(resp.Code(), resp)
		response.PutErrorResponse(resp)
		return
	}
	resp := response.GetShortenResponse()
	resp.ShortCode = code
	c.JSON(resp.Code(), resp)
	response.PutShortenResponse(resp)
}

func (h *PoolHandler) Redirect(c *gin.Context) {
	shortCode := c.Param("short_code")
	if shortCode == "" {
		resp := response.GetBadRequestResponse()
		resp.Error = "short_code is required"
		c.JSON(resp.Code(), resp)
		response.PutBadRequestResponse(resp)
		return
	}
	longURL, err := h.svc.GetOriginalURL(shortCode)
	if err != nil {
		resp := response.GetNotFoundResponse()
		resp.Error = "short code not found"
		c.JSON(resp.Code(), resp)
		response.PutNotFoundResponse(resp)
		return
	}
	_ = h.svc.UpdateClickCount(shortCode)
	c.Redirect(http.StatusFound, longURL)
}

// createNoPoolEngine creates gin engine without pool
func createNoPoolEngine(svc *service.URLService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &noPoolHandler{svc: svc}
	r.POST("/api/shorten", h.ShortenURL)
	r.GET("/:short_code", h.Redirect)
	return r
}

// createPoolEngine creates gin engine with pool
func createPoolEngine(svc *service.URLService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &PoolHandler{svc: svc}
	r.POST("/api/shorten", h.ShortenURL)
	r.GET("/:short_code", h.Redirect)
	return r
}

// ---- ShortenURL Benchmarks ----

// BenchmarkShortenURL_NoPool benchmarks without sync.Pool
func BenchmarkShortenURL_NoPool(b *testing.B) {
	ctx, cleanup := setupBenchCtx()
	defer cleanup()

	svc := service.NewURLService(ctx.db, ctx.rdb, ctx.filter)
	r := createNoPoolEngine(svc)

	body := []byte(`{"long_url": "https://example.com/very/long/url/that/needs/shortening"}`)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest(http.MethodPost, "/api/shorten", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
		}
	})
}

// BenchmarkShortenURL_WithPool benchmarks with sync.Pool
func BenchmarkShortenURL_WithPool(b *testing.B) {
	ctx, cleanup := setupBenchCtx()
	defer cleanup()

	svc := service.NewURLService(ctx.db, ctx.rdb, ctx.filter)
	r := createPoolEngine(svc)

	body := []byte(`{"long_url": "https://example.com/very/long/url/that/needs/shortening"}`)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest(http.MethodPost, "/api/shorten", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
		}
	})
}

// ---- Redirect Benchmarks ----

func prepareRedirectTestData(b *testing.B, ctx *benchmarkCtx, r *gin.Engine) []string {
	codes := make([]string, 0, 100)
	for i := 0; i < 100; i++ {
		code := fmt.Sprintf("bench%d", i)
		ctx.db.Create(&model.URL{
			OriginalURL: fmt.Sprintf("https://example.com/item/%d", i),
			ShortCode:   code,
		})
		ctx.filter.Add([]byte(code))
		codes = append(codes, code)
	}
	// warmup - cache in redis
	for _, code := range codes {
		req := httptest.NewRequest(http.MethodGet, "/"+code, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}
	return codes
}

// BenchmarkRedirect_NotFound_NoPool benchmarks not found with bloom filter (no pool)
func BenchmarkRedirect_NotFound_NoPool(b *testing.B) {
	ctx, cleanup := setupBenchCtx()
	defer cleanup()

	svc := service.NewURLService(ctx.db, ctx.rdb, ctx.filter)
	r := createNoPoolEngine(svc)
	codes := prepareRedirectTestData(b, ctx, r)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			code := fmt.Sprintf("nonexist_%d", randInt())
			req := httptest.NewRequest(http.MethodGet, "/"+code, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
		}
		_ = codes
	})
}

// BenchmarkRedirect_NotFound_WithPool benchmarks not found with bloom filter (with pool)
func BenchmarkRedirect_NotFound_WithPool(b *testing.B) {
	ctx, cleanup := setupBenchCtx()
	defer cleanup()

	svc := service.NewURLService(ctx.db, ctx.rdb, ctx.filter)
	r := createPoolEngine(svc)
	codes := prepareRedirectTestData(b, ctx, r)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			code := fmt.Sprintf("nonexist_%d", randInt())
			req := httptest.NewRequest(http.MethodGet, "/"+code, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
		}
		_ = codes
	})
}

// BenchmarkRedirect_CacheHit_NoPool benchmarks cache hit (no pool)
func BenchmarkRedirect_CacheHit_NoPool(b *testing.B) {
	ctx, cleanup := setupBenchCtx()
	defer cleanup()

	svc := service.NewURLService(ctx.db, ctx.rdb, ctx.filter)
	r := createNoPoolEngine(svc)
	codes := prepareRedirectTestData(b, ctx, r)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			code := codes[randInt()%len(codes)]
			req := httptest.NewRequest(http.MethodGet, "/"+code, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
		}
	})
}

// BenchmarkRedirect_CacheHit_WithPool benchmarks cache hit (with pool)
func BenchmarkRedirect_CacheHit_WithPool(b *testing.B) {
	ctx, cleanup := setupBenchCtx()
	defer cleanup()

	svc := service.NewURLService(ctx.db, ctx.rdb, ctx.filter)
	r := createPoolEngine(svc)
	codes := prepareRedirectTestData(b, ctx, r)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			code := codes[randInt()%len(codes)]
			req := httptest.NewRequest(http.MethodGet, "/"+code, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
		}
	})
}

var randSrc sync.Mutex
var randVal int

func randInt() int {
	randSrc.Lock()
	defer randSrc.Unlock()
	randVal++
	return randVal
}

// Helper to avoid compiler optimizing away
var _ = json.Marshal
var _ = context.Background
var _ = runtime.NumCPU