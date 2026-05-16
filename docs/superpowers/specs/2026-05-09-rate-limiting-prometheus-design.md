# Rate Limiting & Prometheus Metrics Design

**Date**: 2026-05-09
**Status**: Approved

## 1. Overview

引入令牌桶限流保护并集成 Prometheus 监控指标，完善系统的防御体系与可观测性。

## 2. Technical Stack

| Component | Library | Version |
|-----------|---------|---------|
| Rate Limiter | golang.org/x/time/rate | official |
| Metrics | github.com/prometheus/client_golang | v1.20.x |
| Gin Middleware | github.com/prometheus/client_golang/prometheus/promhttp | - |

## 3. Architecture

```
请求 → RateLimiterMiddleware → MetricsMiddleware → Handler
            ↓                      ↓
      全局/单IP限流              指标埋点
```

## 4. Components

### 4.1 Rate Limiter Middleware (`internal/middleware/ratelimit.go`)

**Global Rate Limiter**:
- Limit: 5000 QPS (system-wide)
- Implementation: Single `rate.Limiter` instance

**Per-IP Rate Limiter**:
- Limit: 20 QPS per IP
- Implementation: `sync.Map` storing per-IP `*rate.Limiter`
- Cleanup: No explicit cleanup needed (limiter instances are lightweight)

**Behavior on limit exceeded**:
- Return HTTP 429 Too Many Requests
- Use `response.GetErrorResponse()` from sync.Pool
- Increment `rate_limit_exceeded_total` counter

### 4.2 Metrics Middleware (`internal/metrics/middleware.go`)

**Endpoint**: `GET /metrics` (Prometheus scrape target)

**Metrics Definition** (`internal/metrics/metrics.go`):

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `http_requests_total` | Counter | path, method, status | HTTP request count |
| `rate_limit_exceeded_total` | Counter | limit_type (global/per_ip) | Rate limit rejections |
| `cache_hit_total` | Counter | cache_type (local/redis) | Cache hits |
| `cache_miss_total` | Counter | - | Cache misses |

### 4.3 Docker Compose

Add Prometheus service:
```yaml
prometheus:
  image: prom/prometheus:latest
  ports:
    - "9090:9090"
  volumes:
    - ./docker/prometheus/prometheus.yml:/etc/prometheus/prometheus.yml
  command:
    - '--config.file=/etc/prometheus/prometheus.yml'
    - '--scrape_interval=15s'
```

### 4.4 Prometheus Config (`docker/prometheus/prometheus.yml`)

```yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: 'short-link'
    static_configs:
      - targets: ['host.docker.internal:8080']
    metrics_path: '/metrics'
```

## 5. Files to Create/Modify

| File | Action |
|------|--------|
| `internal/middleware/ratelimit.go` | Create |
| `internal/metrics/metrics.go` | Create |
| `internal/metrics/middleware.go` | Create |
| `cmd/server/main.go` | Modify - register middlewares |
| `docker-compose.yaml` | Modify - add Prometheus |
| `docker/prometheus/prometheus.yml` | Create |

## 6. Middleware Registration

In `main.go`:
```go
// Register middlewares
r.Use(middleware.RateLimiter())
r.Use(metricsMiddleware())
```

## 7. Performance Considerations

- Rate limiter uses efficient atomic operations
- Per-IP limiters stored in sync.Map to avoid lock contention
- 429 responses use sync.Pool as requested