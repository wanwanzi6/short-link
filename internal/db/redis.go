package db

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/wanwanzi6/short-link/internal/config"
)

// RDB 是全局 Redis 客户端实例
var RDB *redis.Client

// InitRedis 初始化 Redis 连接
//
// 连接参数从 config.AppConfig 中读取：
//   - Host: 配置中的 redis.host
//   - Port: 配置中的 redis.port
//   - Password: 配置中的 redis.password
//   - DB: 0 (默认数据库)
//
// 连接池配置：
//   - PoolSize: 10 最大连接数
//   - MinIdleConns: 5 最小空闲连接数
//   - ConnectTimeout: 5 秒连接超时
//   - ReadTimeout: 3 秒读取超时
//   - WriteTimeout: 3 秒写入超时
func InitRedis() error {
	cfg := config.AppConfig.Redis

	RDB = redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password:     cfg.Password,
		DB:           0,                     // 默认数据库
		PoolSize:     10,                    // 连接池大小
		MinIdleConns: 5,                     // 最小空闲连接
		DialTimeout:  5 * time.Second,       // 连接超时
		ReadTimeout:  3 * time.Second,       // 读取超时
		WriteTimeout: 3 * time.Second,       // 写入超时
	})

	// Ping 测试连接是否成功
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := RDB.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}

	log.Println("Redis connection established successfully")
	return nil
}

// CloseRedis 关闭 Redis 连接
func CloseRedis() error {
	if RDB != nil {
		return RDB.Close()
	}
	return nil
}