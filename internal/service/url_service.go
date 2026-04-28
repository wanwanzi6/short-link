package service

import (
	"github.com/wanwanzi6/short-link/internal/db"
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
func NewURLService() *URLService {
	return &URLService{
		db: db.DB,
	}
}

// ShortenURL 将长链接转换为短链接
//
// 流程：
//  1. 先插入一条记录，此时 ShortCode 为空
//  2. 从插入结果中获取自动生成的 ID
//  3. 使用 Base62 算法将 ID 编码为短码
//  4. 更新记录，将短码写入数据库
//  5. 返回生成的短码
//
// 为什么分两步而不是直接生成短码后插入？
// - 因为我们需要等待数据库返回自增 ID
// - 如果直接插入再查询，会多一次数据库往返
// - CurrentCare: 使用 ID 生成短码，再 Update，这是常见做法
func (s *URLService) ShortenURL(longURL string) (string, error) {
	// 创建 URL 记录，ShortCode 初始为空
	urlRecord := &model.URL{
		OriginalURL: longURL,
	}

	// 插入记录到数据库，获取自动生成的 ID
	if err := s.db.Create(urlRecord).Error; err != nil {
		return "", err
	}

	// 使用 Base62 算法将 ID 编码为短码
	// 例如：ID = 12345 -> ShortCode = "3d7"
	code := utils.Encode(urlRecord.ID)

	// 将生成的短码更新到数据库记录中
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
