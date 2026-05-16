package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/wanwanzi6/short-link/internal/config"
	"github.com/wanwanzi6/short-link/internal/db"
	"github.com/wanwanzi6/short-link/internal/handler"
	"github.com/wanwanzi6/short-link/internal/metrics"
	"github.com/wanwanzi6/short-link/internal/middleware"
	"github.com/wanwanzi6/short-link/internal/service"
)

func main() {
	// 0. 初始化配置（最早执行，读取 config.yaml 和环境变量）
	if err := config.InitConfig(); err != nil {
		log.Fatalf("Failed to initialize config: %v", err)
	}

	// 1. 初始化基础设施：连接数据库
	if err := db.InitDB(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	log.Println("Database initialized successfully")

	// 2. 初始化 Redis
	if err := db.InitRedis(); err != nil {
		log.Fatalf("Failed to initialize Redis: %v", err)
	}

	// 3. 初始化本地缓存（BigCache L1）
	if err := db.InitCache(); err != nil {
		log.Fatalf("Failed to initialize local cache: %v", err)
	}

	// 4. 初始化布隆过滤器（防止缓存穿透）
	if err := db.InitBloomFilter(); err != nil {
		log.Fatalf("Failed to initialize bloom filter: %v", err)
	}
	if err := db.WarmupBloomFilter(); err != nil {
		log.Fatalf("Failed to warm up bloom filter: %v", err)
	}

	// 5. 建立依赖链路：实例化 service 和 handler
	urlService := service.NewURLService(db.DB, db.RDB, db.Filter, db.Cache)
	urlHandler := handler.NewURLHandler(urlService)

	// 6. 配置 Gin 路由
	// gin.Default() 创建一个默认的 Engine，包含 Logger 和 Recovery 中间件
	r := gin.Default()

	// Prometheus metrics endpoint
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Register middlewares
	r.Use(middleware.RateLimiter())
	r.Use(metrics.Middleware())

	// 注册路由
	// POST /api/shorten - 生成短链接
	r.POST("/api/shorten", urlHandler.ShortenURL)

	// GET /:short_code - 根据短码重定向到原始 URL
	r.GET("/:short_code", urlHandler.Redirect)

	// 7. 优雅启动：在 8080 端口启动服务
	addr := ":8080"
	log.Printf("Server starting on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}