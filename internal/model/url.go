package model

import (
	"time"
)

// URL 短链接模型
// 用于存储原始长链接及其对应的短码
type URL struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	OriginalURL string    `gorm:"type:varchar(512);not null;uniqueIndex" json:"original_url"`
	ShortCode   string    `gorm:"type:varchar(10);uniqueIndex" json:"short_code"`
	Clicks      int64     `gorm:"default:0" json:"clicks"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名为 short_links
func (URL) TableName() string {
	return "short_links"
}
