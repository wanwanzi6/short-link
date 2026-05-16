package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// HTTPRequestsTotal counts total HTTP requests
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"path", "method", "status"},
	)

	// RateLimitExceededTotal counts requests rejected by rate limiter
	RateLimitExceededTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rate_limit_exceeded_total",
			Help: "Total number of requests rejected by rate limiter",
		},
		[]string{"limit_type"},
	)

	// CacheHitTotal counts cache hits
	CacheHitTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_hit_total",
			Help: "Total number of cache hits",
		},
		[]string{"cache_type"},
	)

	// CacheMissTotal counts cache misses
	CacheMissTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "cache_miss_total",
			Help: "Total number of cache misses",
		},
	)
)

// RateLimitExceeded increments the rate limit exceeded counter
func RateLimitExceeded(limitType string) {
	RateLimitExceededTotal.WithLabelValues(limitType).Inc()
}

// CacheHit increments the cache hit counter
func CacheHit(cacheType string) {
	CacheHitTotal.WithLabelValues(cacheType).Inc()
}

// CacheMiss increments the cache miss counter
func CacheMiss() {
	CacheMissTotal.Inc()
}

// RecordRequest records an HTTP request
func RecordRequest(path, method, status string) {
	HTTPRequestsTotal.WithLabelValues(path, method, status).Inc()
}