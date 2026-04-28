package main

import (
	"log"

	"github.com/wanwanzi6/short-link/internal/db"
)

func main() {
	// 初始化数据库连接
	if err := db.InitDB(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	log.Println("Server started successfully")
}
