package service

import (
	"context"
	"errors"
	"time"

	"github.com/allegro/bigcache/v3"
	"github.com/bits-and-blooms/bloom/v3"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/wanwanzi6/short-link/internal/metrics"
	"github.com/wanwanzi6/short-link/internal/model"
	"github.com/wanwanzi6/short-link/pkg/utils"
)

// URLService URL 服务层
// 负责处理 URL 相关的业务逻辑
type URLService struct {
	db     *gorm.DB
	rdb    *redis.Client
	filter *bloom.BloomFilter
	cache  *bigcache.BigCache
}

// NewURLService 创建一个新的 URLService 实例
func NewURLService(db *gorm.DB, rdb *redis.Client, filter *bloom.BloomFilter, cache *bigcache.BigCache) *URLService {
	return &URLService{
		db:     db,
		rdb:    rdb,
		filter: filter,
		cache:  cache,
	}
}

// cacheKey 生成 Redis 缓存的 key
// 格式: short:{shortCode}
func cacheKey(shortCode string) string {
	return "short:" + shortCode
}

// ShortenURL 将长链接转换为短链接
//
// 核心逻辑（查询优先）：
//  1. 先查询该长链接是否已存在（数据库）
//  2. 存在则直接返回已有的短码
//  3. 不存在则创建新记录，生成短码后返回
//  4. 生成成功后同步写入 Redis 和本地缓存
//
// 由于 OriginalURL 字段有唯一索引，可以保证同一长链接不会重复插入
func (s *URLService) ShortenURL(longURL string) (string, error) {
	// 1. 先查询该长链接是否已存在
	var existing model.URL
	if err := s.db.Where("original_url = ?", longURL).First(&existing).Error; err == nil {
		// 查到了，直接返回已有的短码
		return existing.ShortCode, nil
	}

	// 2. 查不到，创建新记录
	urlRecord := &model.URL{
		OriginalURL: longURL,
	}

	if err := s.db.Create(urlRecord).Error; err != nil {
		return "", err
	}

	// 3. 使用 Base62 算法将 ID 编码为短码
	code := utils.Encode(urlRecord.ID)

	// 4. 更新记录，将短码写入数据库
	if err := s.db.Model(urlRecord).Update("short_code", code).Error; err != nil {
		return "", err
	}

	// 5. 同步写入 Redis（设置 24 小时过期时间）
	ctx := context.Background()
	_ = s.rdb.Set(ctx, cacheKey(code), longURL, 24*time.Hour)

	// 5.5. 同步写入本地缓存（BigCache L1）
	if s.cache != nil {
		_ = s.cache.Set(code, []byte(longURL))
	}

	// 6. 将新短码加入布隆过滤器，防止缓存穿透
	s.filter.Add([]byte(code))

	return code, nil
}

// GetOriginalURL 根据短码查询原始长链接
//
// L1(本地缓存) + L2(Redis) 二级缓存策略：
//  1. 先用布隆过滤器快速判断 short_code 是否可能存在
//  2. 如果布隆过滤器判断不存在，直接返回错误，不再查询缓存/数据库
//  3. L1 本地缓存查询：直接从进程内存获取（纳秒级延迟）
//  4. L1 未命中，查 L2 Redis 缓存
//  5. L2 未命中，查数据库
//  6. 回填流程：数据库命中 -> 异步写入 Redis -> 异步写入本地缓存
//
// 热点数据优化：
//   - 本地缓存命中率高的场景，延迟从微秒级降到纳秒级
//   - 减少 Redis 网络往返次数，降低 Redis 负载
//   - 适合高热点的重定向接口
func (s *URLService) GetOriginalURL(shortCode string) (string, error) {
	// 1. 布隆过滤器检查：快速判断 short_code 是否可能存在
	if !s.filter.Test([]byte(shortCode)) {
		return "", errors.New("short code not found")
	}

	// 2. L1 本地缓存查询（BigCache）
	if s.cache != nil {
		if entry, err := s.cache.Get(shortCode); err == nil {
			metrics.CacheHit("local")
			return string(entry), nil
		}
	}

	ctx := context.Background()

	// 3. L2 Redis 缓存查询
	longURL, err := s.rdb.Get(ctx, cacheKey(shortCode)).Result()
	if err == nil {
		// Redis 命中，回填 L1 本地缓存
		metrics.CacheHit("redis")
		if s.cache != nil {
			go func() {
				_ = s.cache.Set(shortCode, []byte(longURL))
			}()
		}
		return longURL, nil
	}

	// 4. Redis 未命中，查数据库
	metrics.CacheMiss()
	var url model.URL
	if err := s.db.Where("short_code = ?", shortCode).First(&url).Error; err != nil {
		return "", err
	}

	// 5. 异步回填 Redis 和本地缓存
	go func() {
		// 回填 Redis
		_ = s.rdb.Set(context.Background(), cacheKey(shortCode), url.OriginalURL, 24*time.Hour)
		// 回填 L1 本地缓存
		if s.cache != nil {
			_ = s.cache.Set(shortCode, []byte(url.OriginalURL))
		}
	}()

	return url.OriginalURL, nil
}

// UpdateClickCount 原子自增点击数
//
// 使用 GORM 的原子更新方式，将指定短码的点击数 +1
// 这种方式比先查后改更高效，且避免并发问题
func (s *URLService) UpdateClickCount(shortCode string) error {
	return s.db.Model(&model.URL{}).
		Where("short_code = ?", shortCode).
		UpdateColumn("clicks", gorm.Expr("clicks + ?", 1)).
		Error
}
