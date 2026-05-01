package config

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/viper"
)

// AppConfig 是全局配置实例，供其他包调用
var AppConfig *Config

// Config 应用配置结构体
// 包含 MySQL、Redis 等所有可配置项
type Config struct {
	Database DatabaseConfig `mapstructure:"database"`
	Redis    RedisConfig    `mapstructure:"redis"`
}

// DatabaseConfig MySQL 数据库配置
type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"db_name"`
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
}

// InitConfig 初始化配置
//
// 优先级：环境变量 > config.yaml
//
// 支持的环境变量：
//   - DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME
//   - REDIS_HOST, REDIS_PORT, REDIS_PASSWORD
//
// 如果设置了环境变量，则覆盖 config.yaml 中的对应值
func InitConfig() error {
	// 设置配置文件名和路径
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")

	// 尝试读取 config.yaml
	if err := viper.ReadInConfig(); err != nil {
		if err.Error() == "config file not found" {
			return fmt.Errorf("config.yaml not found. Please copy config.yaml.example to config.yaml and modify it accordingly")
		}
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// 反序列化到 Config 结构体
	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// 从环境变量覆盖（优先级最高）
	overrideFromEnv(&cfg)

	AppConfig = &cfg

	log.Printf("Config loaded: DB=%s:%d/%s, Redis=%s:%d",
		cfg.Database.Host, cfg.Database.Port, cfg.Database.DBName,
		cfg.Redis.Host, cfg.Redis.Port)

	return nil
}

// overrideFromEnv 从环境变量覆盖配置
//
// 环境变量优先级高于 config.yaml
func overrideFromEnv(cfg *Config) {
	if v := os.Getenv("DB_HOST"); v != "" {
		cfg.Database.Host = v
	}
	if v := os.Getenv("DB_PORT"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.Database.Port)
	}
	if v := os.Getenv("DB_USER"); v != "" {
		cfg.Database.User = v
	}
	if v := os.Getenv("DB_PASSWORD"); v != "" {
		cfg.Database.Password = v
	}
	if v := os.Getenv("DB_NAME"); v != "" {
		cfg.Database.DBName = v
	}

	if v := os.Getenv("REDIS_HOST"); v != "" {
		cfg.Redis.Host = v
	}
	if v := os.Getenv("REDIS_PORT"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.Redis.Port)
	}
	if v := os.Getenv("REDIS_PASSWORD"); v != "" {
		cfg.Redis.Password = v
	}
}