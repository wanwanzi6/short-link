# ShortLink - 高性能短链接服务

> 🔗 基于 Go + MySQL + Redis 打造的企业级短链接系统，支持高并发、缓存穿透防护与点击追踪

---

## 📖 项目简介

`ShortLink` 是一款高性能短链接生成服务，采用 Go 语言开发，支持 Redis 旁路缓存、Base62 短码生成、布隆过滤器防缓存穿透等企业级特性。

### 核心能力

| 能力 | 说明 |
|------|------|
| 🔢 短码生成 | 基于 MySQL 自增 ID + Base62 编码，生成 6~8 位短码 |
| ⚡ 高性能缓存 | Redis 旁路缓存 + 24h TTL，异步回写降低延迟 |
| 🛡️ 缓存穿透防护 | 布隆过滤器拦截不存在请求，保护数据库 |
| 📊 点击追踪 | 原子自增更新点击数，无锁并发 |
| 🐳 容器化部署 | Docker Compose 一键启动 MySQL + Redis |

---

## 🏗️ 系统架构

```
┌─────────────────────────────────────────────────────────────────────────┐
│                              Client Request                             │
└─────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                           Gin HTTP Server :8080                         │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │  POST /api/shorten              GET /:short_code                │    │
│  └─────────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────────┘
                                    │
                    ┌───────────────┼───────────────┐
                    │               │               │
                    ▼               ▼               ▼
            ┌───────────┐   ┌───────────┐   ┌───────────┐
            │  Handler  │   │  Handler  │   │  Handler  │
            │ ShortenURL│   │ Redirect  │   │   ...     │
            └─────┬─────┘   └─────┬─────┘   └───────────┘
                  │               │
                  └───────┬───────┘
                          ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                          Service Layer (URLService)                     │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │  ShortenURL(longURL)    │    GetOriginalURL(shortCode)          │    │
│  │  UpdateClickCount()     │                                       │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                    │                                    │
│  ┌────────────┐  ┌────────────┐  ┌────────────┐  ┌────────────┐         │
│  │   MySQL    │  │   Redis    │  │   Bloom    │  │   Base62   │         │
│  │  (持久化)   │  │  (缓存层)  │  │  (防护层)   │  │  (编码层)  │         │
│  └────────────┘  └────────────┘  └────────────┘  └────────────┘         │  
└─────────────────────────────────────────────────────────────────────────┘
```

### 数据流说明

```
┌────────────────────────────────────────────────────────────────┐
│                     ShortenURL (生成短链接)                     │
├────────────────────────────────────────────────────────────────┤
│  1. 查询 DB 是否存在该 long_url                                 │
│  2. 不存在 → 插入获取自增 ID                                    │
│  3. Base62(ID) → 生成 short_code                               │
│  4. 同步写入 Redis (TTL 24h)                                   │
│  5. 添加 short_code 到布隆过滤器                                │
└────────────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────────────┐
│                     GetOriginalURL (重定向)                     │
├────────────────────────────────────────────────────────────────┤
│  1. Bloom Filter.Test() → 快速判断是否存在                       │
│     └─ 不存在 → 直接返回 404 (拦截恶意请求)                       │
│  2. Redis.Get() → 命中则直接返回                                 │
│  3. 未命中 → 查询 MySQL                                         │
│  4. 异步回写 Redis (goroutine，不阻塞响应)                       │
└────────────────────────────────────────────────────────────────┘
```

---

## ✨ 核心亮点

### 1. Base62 短码算法

采用自定义 Base62 编码算法，将数据库自增 ID 转换为短字符串：

```
字符集: 0-9 a-z A-Z (共62个字符)

算法原理:
- 数据库自增 ID (uint64) → 除以 62 取余 → 映射到字符集
- 例如: ID=12345 → "3d7" (12345 = 3×62² + 13×62 + 7)

优势:
- 单调递增，ID 越大短码越长，最长约 10 字符
- 无需第三方库，纯位运算实现 O(k) 时间复杂度
- 天然支持去重，相同 URL 映射到同一 ID
```

### 2. Redis 旁路缓存

采用经典的 Cache-Aside 模式：

```
读操作 (GetOriginalURL):
  ┌──────────┐      ┌──────────┐     ┌──────────┐
  │  Bloom   │────▶│  Redis   │────▶│   MySQL  │
  │  Check   │      │   GET    │     │   QUERY  │
  └──────────┘      └──────────┘     └──────────┘
      │                  │                  │
      ▼                  ▼                  ▼
   "不存在"          "返回缓存"        "写入Redis"

写操作 (ShortenURL):
  同步写入 Redis，不阻塞响应
  goroutine 异步回写，避免惊群效应
```

**内存优化**: 使用 `redis.Options{PoolSize: 10, MinIdleConns: 5}` 连接池，合理控制内存占用。

### 3. 布隆过滤器防缓存穿透

防止恶意请求穿透到数据库：

```
布隆过滤器配置:
  - 容量: 100万 (capacity = 1000000)
  - 误报率: 0.01% (fpRate = 0.0001)
  - 内存占用: ~1.44 MB

工作原理:
  - Test(shortCode) → false: 短码一定不存在，直接返回 404
  - Test(shortCode) → true:  短码可能存在，继续查询 Redis/MySQL

启动预热:
  - WarmupBloomFilter() 从 MySQL 加载所有已存在的 short_code
  - 避免冷启动时大量误判
```

### 4. 点击数原子更新

使用 GORM 的 `UpdateColumn` + `Expr` 实现原子自增：

```go
// 无锁并发，SQL 层保证原子性
db.Model(&URL{}).Where("short_code = ?", code).
    UpdateColumn("clicks", gorm.Expr("clicks + ?", 1))
```

---

## 🛠️ 技术栈

| 分类 | 技术 | 版本 | 用途 |
|------|------|------|------|
| **语言** | Go | 1.25.6 | 服务端开发 |
| **Web框架** | Gin | v1.12.0 | HTTP 路由与中间件 |
| **ORM** | GORM | v1.31.1 | MySQL 操作 |
| **缓存** | go-redis | v9.19.0 | Redis 客户端 |
| **布隆过滤** | bits-and-blooms | v3.7.1 | 缓存穿透防护 |
| **数据库** | MySQL | 8.0 | 持久化存储 |
| **缓存** | Redis | 7-alpine | 旁路缓存 |
| **容器** | Docker Compose | - | 服务编排 |
| **测试** | testify | v1.11.1 | 单元测试 |
| **模拟Redis** | miniredis | v2.37.0 | 测试隔离 |

---

## 🚀 快速启动

### 前置条件

- Go 1.25+
- Docker & Docker Compose
- Git

### 1. 克隆项目

```bash
git clone https://github.com/wanwanzi6/short-link.git
cd short-link
```

### 2. 启动基础设施 (MySQL + Redis)

```bash
docker-compose up -d db redis
```

等待服务就绪（约 30 秒）：
```bash
docker-compose ps
# shortlink-db    healthy   ...
# shortlink-redis healthy   ...
```

### 3. 启动服务

```bash
go run cmd/server/main.go
```

输出示例：
```
Database initialized successfully
Redis connection established successfully
Bloom filter initialized with capacity=1000000, fpRate=0.01%
Server starting on :8080
```

### 4. 测试 API

**生成短链接:**
```bash
curl -X POST http://localhost:8080/api/shorten \
  -H "Content-Type: application/json" \
  -d '{"long_url": "https://github.com/wanwanzi6"}'
```

响应:
```json
{"short_code": "1"}
```

**访问短链接 (重定向):**
```bash
curl -I http://localhost:8080/1
```

响应头:
```
HTTP/1.1 302 Found
Location: https://github.com/wanwanzi6
```

---

## 📡 API 文档

### 接口一览

| 方法 | 路径 | 描述 | 请求体 | 响应 |
|------|------|------|--------|------|
| `POST` | `/api/shorten` | 生成短链接 | `{"long_url": "..."}` | `{"short_code": "..."}` |
| `GET` | `/:short_code` | 重定向跳转 | - | 302 重定向到原始 URL |

### 详细说明

#### POST /api/shorten

生成短链接或返回已存在的短码。

**请求:**
```json
{
  "long_url": "https://example.com/very/long/path"
}
```

**成功响应 (200):**
```json
{
  "short_code": "3d7"
}
```

**错误响应 (400):**
```json
{
  "error": "long_url cannot be empty"
}
```

---

#### GET /:short_code

根据短码重定向到原始 URL。

**成功响应 (302):**
```
HTTP/1.1 302 Found
Location: https://example.com/very/long/path
```

**错误响应 (404):**
```json
{
  "error": "short code not found"
}
```

---

## 📊 测试覆盖率

```
包名                      覆盖率
─────────────────────────────────────────
cmd/server               0.0%
internal/db              0.0%
internal/handler        82.6%  ✅
internal/model          0.0%
internal/service        89.3%  ✅
pkg/utils               100.0% ✅
─────────────────────────────────────────
总体覆盖率                >80%
```

### 测试说明

- `internal/service`: 覆盖布隆过滤器拦截、Redis 缓存命中、数据库回写、去重等场景
- `internal/handler`: 覆盖输入校验、响应格式、状态码、点击数更新等场景
- `pkg/utils`: Base62 编解码 100% 覆盖

### 运行测试

```bash
go test ./... -cover
```

---

## 📁 项目结构

```
short-link/
├── cmd/
│   └── server/
│       └── main.go              # 程序入口
├── internal/
│   ├── db/
│   │   ├── mysql.go             # MySQL 初始化
│   │   ├── redis.go             # Redis 初始化
│   │   └── bloom.go             # 布隆过滤器
│   ├── handler/
│   │   └── url_handler.go       # HTTP 处理器
│   ├── model/
│   │   └── url.go               # 数据模型
│   └── service/
│       └── url_service.go       # 业务逻辑
├── pkg/
│   └── utils/
│       └── base62.go            # Base62 编码算法
├── tests/
│   └── api/
│       └── test.http            # HTTP 测试文件
├── docker-compose.yaml          # 基础设施编排
└── README.md
```

---

## 🔮 未来规划

- [ ] 支持自定义短码
- [ ] 批量生成短链接
- [ ] 访问统计分析面板
- [ ] 短码过期与删除机制
- [ ] 分布式集群部署支持

---

## 📜 License

MIT License - 详见 [LICENSE](LICENSE)