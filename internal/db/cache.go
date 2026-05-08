package db

import (
	"context"
	"log"
	"time"

	"github.com/allegro/bigcache/v3"

	"github.com/wanwanzi6/short-link/internal/config"
)

// Cache 是全局本地缓存实例（BigCache L1 缓存）
var Cache *bigcache.BigCache

// InitCache 初始化本地缓存
//
// BigCache 配置说明：
//   - Shards: 256 缓存分片数，并发读写性能更好
//   - LifeWindow: 10分钟 记录最大存活时间
//   - MaxEntriesInWindow: 100000 单个时间窗口内最大条目数
//   - MaxEntrySize: 500 单条记录最大字节数
//   - HardMaxCacheSize: 100 MB 硬上限，超出后不再写入
//
// L1 缓存作用：
//   - 热点数据放在进程内存中，避免网络开销直接访问 Redis
//   - 比 Redis 延迟更低（纳秒级 vs 微秒级）
//   - 适合高热点的重定向场景
func InitCache() error {
	cfg := config.AppConfig.Redis

	// 计算缓存大小配置
	// 100万短码，每个约200字节，约200MB理论需求
	// 这里设置100MB硬上限，配合10分钟TTL，实际占用会小很多
	hardMaxSize := 100 // MB

	conf := bigcache.DefaultConfig(10 * time.Minute) // LifeWindow: 10分钟
	conf.HardMaxCacheSize = hardMaxSize

	cache, err := bigcache.New(context.Background(), conf)
	if err != nil {
		return err
	}

	Cache = cache
	log.Printf("Local cache initialized (LifeWindow=10min, HardMaxCacheSize=%dMB)", hardMaxSize)
	log.Printf("Redis config: %s:%d", cfg.Host, cfg.Port)
	return nil
}

// CloseCache 关闭本地缓存
func CloseCache() error {
	if Cache != nil {
		return Cache.Close()
	}
	return nil
}
