package service

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/bits-and-blooms/bloom/v3"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/wanwanzi6/short-link/internal/model"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	err = db.AutoMigrate(&model.URL{})
	require.NoError(t, err)
	return db
}

func setupTestRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	return mr, rdb
}

func setupTestFilter() *bloom.BloomFilter {
	return bloom.NewWithEstimates(1000, 0.01)
}

// TestGetOriginalURL_BloomFilterReject 测试布隆过滤器拦截不存在的短码
func TestGetOriginalURL_BloomFilterReject(t *testing.T) {
	db := setupTestDB(t)
	_, rdb := setupTestRedis(t)
	filter := setupTestFilter()
	svc := NewURLService(db, rdb, filter)

	// 布隆过滤器中不存在任何短码，"nonexistent" 会被直接拦截
	longURL, err := svc.GetOriginalURL("nonexistent")
	assert.Error(t, err)
	assert.Empty(t, longURL)
	assert.Contains(t, err.Error(), "not found")
}

// TestGetOriginalURL_RedisHit 测试 Redis 命中场景
func TestGetOriginalURL_RedisHit(t *testing.T) {
	db := setupTestDB(t)
	_, rdb := setupTestRedis(t)
	filter := setupTestFilter()

	// 先手动写入 Redis（模拟已缓存的场景）
	ctx := context.Background()
	rdb.Set(ctx, cacheKey("abc123"), "https://example.com", 24*time.Hour)
	filter.Add([]byte("abc123"))

	svc := NewURLService(db, rdb, filter)

	// Redis 命中，应直接返回
	longURL, err := svc.GetOriginalURL("abc123")
	assert.NoError(t, err)
	assert.Equal(t, "https://example.com", longURL)
}

// TestGetOriginalURL_DBFallback 测试数据库回写 Redis 场景
func TestGetOriginalURL_DBFallback(t *testing.T) {
	db := setupTestDB(t)
	mr, rdb := setupTestRedis(t)
	filter := setupTestFilter()

	// 预先插入一条数据库记录
	urlRecord := &model.URL{
		OriginalURL: "https://golang.org",
		ShortCode:   "golang",
	}
	db.Create(urlRecord)
	filter.Add([]byte("golang"))

	svc := NewURLService(db, rdb, filter)

	// Redis 没有缓存，会回写到数据库
	longURL, err := svc.GetOriginalURL("golang")
	assert.NoError(t, err)
	assert.Equal(t, "https://golang.org", longURL)

	// 等待异步写入完成
	time.Sleep(100 * time.Millisecond)

	// 验证 Redis 中已有缓存
	ctx := context.Background()
	val, err := rdb.Get(ctx, cacheKey("golang")).Result()
	assert.NoError(t, err)
	assert.Equal(t, "https://golang.org", val)

	mr.Close()
}

// TestShortenURL_Success 测试生成短链接成功
func TestShortenURL_Success(t *testing.T) {
	db := setupTestDB(t)
	_, rdb := setupTestRedis(t)
	filter := setupTestFilter()
	svc := NewURLService(db, rdb, filter)

	code, err := svc.ShortenURL("https://www.example.com/very/long/path")
	assert.NoError(t, err)
	assert.NotEmpty(t, code)

	// 验证数据库中有记录
	var url model.URL
	err = db.Where("original_url = ?", "https://www.example.com/very/long/path").First(&url).Error
	assert.NoError(t, err)
	assert.Equal(t, code, url.ShortCode)

	// 验证 Redis 中有缓存
	ctx := context.Background()
	val, err := rdb.Get(ctx, cacheKey(code)).Result()
	assert.NoError(t, err)
	assert.Equal(t, "https://www.example.com/very/long/path", val)

	// 验证布隆过滤器中有记录
	assert.True(t, filter.Test([]byte(code)))
}

// TestShortenURL_Duplicate 测试重复生成返回同一短码
func TestShortenURL_Duplicate(t *testing.T) {
	db := setupTestDB(t)
	_, rdb := setupTestRedis(t)
	filter := setupTestFilter()
	svc := NewURLService(db, rdb, filter)

	longURL := "https://duplicate-test.com"

	// 第一次生成
	code1, err := svc.ShortenURL(longURL)
	assert.NoError(t, err)

	// 第二次生成相同 URL
	code2, err := svc.ShortenURL(longURL)
	assert.NoError(t, err)

	// 两次应该返回相同的短码
	assert.Equal(t, code1, code2)

	// 数据库中应该只有一条记录
	var count int64
	db.Model(&model.URL{}).Where("original_url = ?", longURL).Count(&count)
	assert.Equal(t, int64(1), count)
}

// TestUpdateClickCount 测试点击数原子自增
func TestUpdateClickCount(t *testing.T) {
	db := setupTestDB(t)
	_, rdb := setupTestRedis(t)
	filter := setupTestFilter()
	svc := NewURLService(db, rdb, filter)

	// 插入一条记录
	urlRecord := &model.URL{
		OriginalURL: "https://click-test.com",
		ShortCode:   "click",
		Clicks:      0,
	}
	db.Create(urlRecord)
	filter.Add([]byte("click"))

	// 模拟两次访问
	err := svc.UpdateClickCount("click")
	assert.NoError(t, err)
	err = svc.UpdateClickCount("click")
	assert.NoError(t, err)

	// 验证点击数
	var url model.URL
	db.Where("short_code = ?", "click").First(&url)
	assert.Equal(t, int64(2), url.Clicks)
}