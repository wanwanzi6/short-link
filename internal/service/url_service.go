package service

import (
	"context"
	"errors"
	"time"

	"github.com/bits-and-blooms/bloom/v3"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/wanwanzi6/short-link/internal/model"
	"github.com/wanwanzi6/short-link/pkg/utils"
)

// URLService URL 服务层
// 负责处理 URL 相关的业务逻辑
type URLService struct {
	db     *gorm.DB
	rdb    *redis.Client
	filter *bloom.BloomFilter
}

// NewURLService 创建一个新的 URLService 实例
func NewURLService(db *gorm.DB, rdb *redis.Client, filter *bloom.BloomFilter) *URLService {
	return &URLService{
		db:     db,
		rdb:    rdb,
		filter: filter,
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
//  4. 生成成功后同步写入 Redis
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

	// 6. 将新短码加入布隆过滤器，防止缓存穿透
	s.filter.Add([]byte(code))

	return code, nil
}

// GetOriginalURL 根据短码查询原始长链接
//
// 布隆过滤器防穿透策略：
//  1. 先用布隆过滤器快速判断 short_code 是否可能存在
//  2. 如果布隆过滤器判断不存在，直接返回错误，不再查询 Redis/数据库
//  3. 如果布隆过滤器判断可能存在，继续查询 Redis 和数据库
//
// 为什么用布隆过滤器？
//   - 存储效率高，100万短码只需约 1.44 MB
//   - 查询速度快，O(k) 哈希操作，k 为哈希函数数量
//   - 可以快速过滤掉大量不存在的查询，减少 Redis/数据库压力
func (s *URLService) GetOriginalURL(shortCode string) (string, error) {
	// 1. 布隆过滤器检查：快速判断 short_code 是否可能存在
	if !s.filter.Test([]byte(shortCode)) {
		// 布隆过滤器判断不存在，直接返回错误
		return "", errors.New("short code not found")
	}

	ctx := context.Background()

	// 2. 查 Redis 缓存
	longURL, err := s.rdb.Get(ctx, cacheKey(shortCode)).Result()
	if err == nil {
		// Redis 命中，直接返回
		return longURL, nil
	}

	// 3. Redis 未命中，查数据库
	var url model.URL
	if err := s.db.Where("short_code = ?", shortCode).First(&url).Error; err != nil {
		return "", err
	}

	// 4. 异步写入 Redis（不阻塞返回）
	go func() {
		_ = s.rdb.Set(context.Background(), cacheKey(shortCode), url.OriginalURL, 24*time.Hour)
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
