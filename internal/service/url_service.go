package service

import (
	"github.com/wanwanzi6/short-link/internal/model"
	"github.com/wanwanzi6/short-link/pkg/utils"
	"gorm.io/gorm"
)

// Service URL 服务层
// 负责处理 URL 相关的业务逻辑
type URLService struct {
	db *gorm.DB
}

// NewURLService 创建一个新的 URLService 实例
func NewURLService(db *gorm.DB) *URLService {
	return &URLService{
		db: db,
	}
}

// ShortenURL 将长链接转换为短链接
//
// 核心逻辑（查询优先）：
//  1. 先查询该长链接是否已存在
//  2. 存在则直接返回已有的短码
//  3. 不存在则创建新记录，生成短码后返回
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

	return code, nil
}

// GetOriginalURL 根据短码查询原始长链接
//
// 流程：
//  1. 使用短码在数据库中查询对应记录
//  2. 返回记录的原始 URL
//
// 注意：
//   - 如果找不到对应短码，会返回空字符串
//   - 实际使用中可能需要区分"不存在"和"查询失败"的情况
func (s *URLService) GetOriginalURL(shortCode string) (string, error) {
	var url model.URL

	// 查询 short_code 等于指定值的记录
	if err := s.db.Where("short_code = ?", shortCode).First(&url).Error; err != nil {
		return "", err
	}

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
