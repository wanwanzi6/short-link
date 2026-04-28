package db

import (
	"fmt"
	"log"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/wanwanzi6/short-link/internal/model"
)

// DB 是全局数据库连接实例，供其他包调用
var DB *gorm.DB

// InitDB 初始化 MySQL 数据库连接
//
// 连接参数说明：
//   - 用户名: root (由 MYSQL_ROOT_PASSWORD 环境变量设置)
//   - 密码: root123456
//   - 主机: shortlink-db (docker-compose 服务名，Docker 网络内可解析)
//   - 端口: 3306
//   - 数据库名: short_link (由 MYSQL_DATABASE 环境变量创建)
//
// 连接池配置：
//   - MaxIdleConns: 10  最大空闲连接数
//   - MaxOpenConns: 100 最大打开连接数
//   - ConnMaxLifetime: 1小时 连接最大生命周期，避免长时间连接导致的问题
//
// 错误处理：
//   - 连接失败会记录致命日志并终止程序
//   - 使用 gormLogger 记录 SQL 执行情况（开发环境建议开启）
func InitDB() error {
	// DSN (Data Source Name) 格式：
	// username:password@tcp(host:port)/dbname?charset=utf8mb4&parseTime=True&loc=Local
	//
	// 参数说明：
	//   - charset=utf8mb4 支持完整 Unicode 字符（包括 emoji）
	//   - parseTime=True 自动将时间类型解析为 time.Time
	//   - loc=Local 使用本地时区
	dsn := "root:root123456@tcp(127.0.0.1:3306)/short_link?charset=utf8mb4&parseTime=True&loc=Local"

	// 配置 GORM 日志级别
	// Silent 模式不输出任何日志
	// Info 模式输出 SQL 语句（生产环境建议用 Warning）
	gormLogger := logger.New(
		log.New(log.Writer(), "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             time.Second, // 慢查询阈值，超过 1 秒记录
			LogLevel:                  logger.Info, // 记录 Info 级别及以上的日志
			IgnoreRecordNotFoundError: true,        // 忽略 "record not found" 错误
			Colorful:                  true,        // 彩色输出
		},
	)

	// 打开数据库连接
	// gorm.Config 中可以传入 Logger、SkipDefaultTransaction 等配置
	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: gormLogger,
	})
	if err != nil {
		// 连接失败是致命错误，无法继续运行
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// 获取底层的 sql.DB 实例，用于配置连接池
	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	// 配置连接池参数
	// 合理的连接池设置可以提高数据库性能和稳定性
	sqlDB.SetMaxIdleConns(10)           // 最大空闲连接数
	sqlDB.SetMaxOpenConns(100)          // 最大打开连接数
	sqlDB.SetConnMaxLifetime(time.Hour) // 连接最大生命周期

	// 测试连接是否成功
	// Ping 会发起一次实际的数据库通信
	if err = sqlDB.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	log.Println("Database connection established successfully")

	// 自动迁移表结构
	// AutoMigrate 会根据模型的 GORM 标签自动创建或更新表
	// 如果表已存在，只会调整结构（添加新列等），不会丢失数据
	if err = DB.AutoMigrate(&model.URL{}); err != nil {
		return fmt.Errorf("failed to auto migrate: %w", err)
	}

	log.Println("Database migration completed: short_links table ready")
	return nil
}

// CloseDB 关闭数据库连接
// 应在程序退出时调用，确保连接池资源被正确释放
func CloseDB() error {
	if DB == nil {
		return nil
	}
	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
