package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/wanwanzi6/short-link/internal/db"
	"github.com/wanwanzi6/short-link/internal/handler"
	"github.com/wanwanzi6/short-link/internal/service"
)

func main() {
	// 1. 初始化基础设施：连接数据库
	if err := db.InitDB(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	log.Println("Database initialized successfully")

	// 2. 初始化 Redis
	if err := db.InitRedis(); err != nil {
		log.Fatalf("Failed to initialize Redis: %v", err)
	}

	// 3. 建立依赖链路：实例化 service 和 handler
	urlService := service.NewURLService(db.DB, db.RDB)
	urlHandler := handler.NewURLHandler(urlService)

	// 4. 配置 Gin 路由
	// gin.Default() 创建一个默认的 Engine，包含 Logger 和 Recovery 中间件
	r := gin.Default()

	// 注册路由
	// POST /api/shorten - 生成短链接
	r.POST("/api/shorten", urlHandler.ShortenURL)

	// GET /:short_code - 根据短码重定向到原始 URL
	r.GET("/:short_code", urlHandler.Redirect)

	// 5. 优雅启动：在 8080 端口启动服务
	addr := ":8080"
	log.Printf("Server starting on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
