# ShortLink - 高性能短链接服务

> 🔗 基于 Go + MySQL + Redis 打造的企业级短链接系统，支持高并发、缓存穿透防护与点击追踪

---

## 📖 项目简介

`ShortLink` 是一款高性能短链接生成服务，采用 Go 语言开发，支持 Redis 旁路缓存、Base62 短码生成、布隆过滤器防缓存穿透等企业级特性。

### 核心能力

| 能力 | 说明 |
|------|------|
| 🔢 短码生成 | 基于 MySQL 自增 ID + Base62 编码，生成 6~8 位短码 |
| ⚡ 高性能缓存 | L1 本地缓存(BigCache) + L2 Redis 旁路缓存 + 24h TTL |
| 🛡️ 缓存穿透防护 | 布隆过滤器拦截不存在请求，保护数据库 |
| 📊 点击追踪 | 原子自增更新点击数，无锁并发 |
| 🔄 响应对象池 | sync.Pool 复用响应结构体，减少 GC 压力 |
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
│  │  (持久化)   │  │  (L2缓存)  │  │  (防护层)   │  │  (编码层)  │         │
│  └────────────┘  └────────────┘  └────────────┘  └────────────┘         │
│                                    │                                    │
│  ┌─────────────────────────────────────────────────────────────┐        │
│  │              BigCache (L1 本地缓存)                            │        │
│  │  - 进程内内存缓存，纳秒级延迟                                  │        │
│  │  - TTL: 10分钟，防止 stale 数据                              │        │
│  │  - 热点数据预热，减少 Redis 网络往返                          │        │
│  └─────────────────────────────────────────────────────────────┘        │
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
│  5. 同步写入本地缓存 BigCache (TTL 10min)                       │
│  6. 添加 short_code 到布隆过滤器                                │
└────────────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────────────┐
│                     GetOriginalURL (重定向)                     │
├────────────────────────────────────────────────────────────────┤
│  1. Bloom Filter.Test() → 快速判断是否存在                       │
│     └─ 不存在 → 直接返回 404 (拦截恶意请求)                       │
│  2. BigCache.Get() → L1本地缓存命中则直接返回 (纳秒级)           │
│  3. Redis.Get() → L2缓存命中则返回 + 回填L1                     │
│  4. 未命中 → 查询 MySQL                                         │
│  5. 异步回写 Redis + 本地缓存 (goroutine，不阻塞响应)            │
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

### 2. L1 + L2 二级缓存架构

采用 BigCache(本地) + Redis(分布式) 二级缓存策略：

```
读操作 (GetOriginalURL):
  ┌──────────┐   ┌──────────┐   ┌──────────┐   ┌──────────┐
  │  Bloom   │──▶│ BigCache │──▶│  Redis   │──▶│   MySQL  │
  │  Check   │   │  (L1)    │   │   (L2)   │   │   QUERY  │
  └──────────┘   └──────────┘   └──────────┘   └──────────┘
      │               │               │               │
      ▼               ▼               ▼               ▼
   "不存在"        "L1命中"        "L2命中"        "回填L1+L2"

缓存层级对比:
┌─────────────────────────────────────────────────────────────────┐
│  层级   │  介质      │  延迟    │  容量    │  适用场景           │
├─────────┼────────────┼──────────┼──────────┼────────────────────┤
│  L1     │  本地内存   │  ~100ns  │  100MB   │  热点数据，高频访问  │
│  L2     │  Redis     │  ~100μs  │  内存/持久化 │  跨实例共享，宕机恢复 │
└─────────────────────────────────────────────────────────────────┘

L1 本地缓存 (BigCache) 配置:
  - TTL: 10 分钟
  - 硬上限: 100 MB
  - 分片数: 256 (高并发读写)

L2 分布式缓存 (Redis) 配置:
  - TTL: 24 小时
  - 连接池: PoolSize=10, MinIdleConns=5
```

**性能收益**:

| 场景 | L1+L2 (本地命中) | L2 Only (Redis) | 提升 |
|------|-----------------|-----------------|------|
| 内存分配 | 118 B/op | 135 B/op | **-13%** |
| 分配次数 | 118 allocs/op | 135 allocs/op | **-13%** |

> 压测环境: Intel i7-13650HX, miniredis 本地模拟
> 生产环境网络延迟约 0.5-2ms，ns/op 差距会更显著

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

### 4. 响应对象池 (sync.Pool)

使用 `sync.Pool` 复用 HTTP 响应结构体，减少内存分配开销：

```
响应类型池化:
  - ShortenResponse    → 生成短链接成功响应
  - ErrorResponse      → 通用错误响应 (500)
  - BadRequestResponse → 请求错误响应 (400)
  - NotFoundResponse   → 未找到响应 (404)

工作原理:
  1. 首次请求 → 从 pool.Get() 获取结构体
  2. 填充数据 → 序列化 JSON 返回
  3. 请求结束 → pool.Put() 归还到池中
  4. 下次请求 → 复用已存在的结构体，无需新建
```

**性能收益:**

| 场景 | 优化前 | 优化后 | 提升 |
|------|--------|--------|------|
| 错误响应 (404) | 27 allocs/op | 22 allocs/op | **-19%** |
| 错误响应内存 | 6,639 B/op | 6,228 B/op | **-6%** |
| 成功响应 (200) | 206 allocs/op | 198 allocs/op | **-4%** |

> 详细压测报告见 [docs/performance/response-pool-benchmark.md](docs/performance/response-pool-benchmark.md)

### 5. 点击数原子更新

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
| **本地缓存** | BigCache | v3.1.0 | L1 本地缓存 |
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
- Make (mingw32-make on Windows)

### 使用 Makefile 快速启动

```bash
# 查看所有可用命令
make help

# 启动基础设施 (MySQL + Redis)
make docker-up

# 运行测试
make test

# 运行压测
make bench

# 编译并启动服务
make build
./bin/server

# 或者直接运行 (无需编译)
go run cmd/server/main.go
```

### 手动启动

#### 1. 克隆项目

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

## 性能挑战与压测 (Performance Benchmark)

### 测试命令

```bash
go test -bench=. -benchmem -run=none ./internal/service/...
```

### L1 + L2 缓存压测对比

| 场景 | 缓存类型 | ns/op | 内存分配 | 分配次数 |
|------|---------|-------|----------|----------|
| L1 本地缓存命中 | BigCache (L1) | 57,307 | 13,824 B/op | **118 allocs/op** |
| L2 Redis缓存命中 | Redis Only (L2) | 57,037 | 14,222 B/op | 135 allocs/op |

**结论**: 本地缓存命中减少 **13%** 内存分配次数，绕过 Redis 网络往返。

> 测试环境: Intel i7-13650HX, miniredis 本地模拟
> 生产环境含真实网络延迟，L1 延迟优势更显著 (~0.5-2ms 节省)

### 布隆过滤器压测结果

| 场景 | 布隆过滤器 | QPS | 延迟 | ns/op | 内存分配 | 分配次数 |
|------|-----------|-----|------|-------|----------|----------|
| 正常请求(缓存命中) | ✅ | 21,258 | 56μs | 56,891 | 14,301 B/op | 135 allocs/op |
| 缓存穿透(有布隆) | ✅ | **354,048** | **3μs** | **2,899** | 6,947 B/op | 32 allocs/op |
| 缓存穿透(无布隆) | ❌ | 18,224 | 65μs | 65,225 | 11,883 B/op | 124 allocs/op |
| 混合(50%不存在) | ✅ | 39,853 | 31μs | 30,688 | 9,259 B/op | 75 allocs/op |

### 性能提升分析

| 指标 | WithoutBloom | WithBloom | 提升倍数 |
|------|-------------|-----------|---------|
| **延迟** | 65,225 ns/op | 2,899 ns/op | **22.5x** |
| **QPS** | 18,224 | 354,048 | **19.4x** |
| **内存分配** | 11,883 B/op | 6,947 B/op | **1.7x 节省** |
| **分配次数** | 124 allocs/op | 32 allocs/op | **3.9x 减少** |

### 架构价值

```
布隆过滤器拦截链 (WithBloom):
  请求 → filter.Test() → 2,899 ns → 返回 404
          ↑
       1 次内存操作，无需 I/O

无布隆过滤器穿透链 (WithoutBloom):
  请求 → Redis.Get() → nil → db.First() → 65,225 ns
          ↑              ↑           ↑
       内存操作        网络 I/O    数据库 I/O

L1+L2 二级缓存链 (热点数据):
  请求 → BigCache.Get() → 57,307 ns → 返回 URL
          ↑
       纯内存操作，无网络 I/O
```
       内存操作        网络 I/O    数据库 I/O
```

**核心结论：**

- 在 95% 不存在请求场景下，**布隆过滤器提升性能 22.5x**
- 单次布隆过滤判断仅需 ~10ns，内存占用 1.44MB 可支持 100 万短码
- 避免无效请求穿透到 Redis/数据库，节省 3.9x 的内存分配开销

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
│   ├── response/
│   │   └── response.go          # 响应结构体 + sync.Pool 对象池
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