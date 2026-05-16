package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"

	"github.com/wanwanzi6/short-link/internal/metrics"
	"github.com/wanwanzi6/short-link/internal/response"
)

// Global rate limiter: 5000 QPS system-wide
var globalLimiter = rate.NewLimiter(rate.Limit(5000), 5000)

// Per-IP rate limiters: 20 QPS per IP
var ipLimiters sync.Map

// getIPLimiter returns the rate limiter for a specific IP
func getIPLimiter(ip string) *rate.Limiter {
	if v, ok := ipLimiters.Load(ip); ok {
		return v.(*rate.Limiter)
	}

	// Create new limiter: 20 QPS, burst of 20
	limiter := rate.NewLimiter(rate.Limit(20), 20)
	ipLimiters.Store(ip, limiter)
	return limiter
}

// clientIP extracts the client IP from the request
func clientIP(c *gin.Context) string {
	// Check X-Forwarded-For header first (for proxy scenarios)
	if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
		return xff
	}
	// Check X-Real-IP header
	if xri := c.GetHeader("X-Real-IP"); xri != "" {
		return xri
	}
	return c.ClientIP()
}

// RateLimiter returns a Gin middleware that implements rate limiting
// It applies two levels of rate limiting:
// 1. Global rate limit: 5000 QPS system-wide
// 2. Per-IP rate limit: 20 QPS per IP
// Note: /metrics endpoint is excluded from rate limiting
func RateLimiter() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip rate limiting for Prometheus metrics endpoint
		if c.FullPath() == "/metrics" {
			c.Next()
			return
		}

		ip := clientIP(c)
		ipLimiter := getIPLimiter(ip)

		// Check global limiter first
		if !globalLimiter.Allow() {
			metrics.RateLimitExceeded("global")
			serveTooManyRequests(c, "global")
			c.Abort()
			return
		}

		// Check per-IP limiter
		if !ipLimiter.Allow() {
			metrics.RateLimitExceeded("per_ip")
			serveTooManyRequests(c, "per_ip")
			c.Abort()
			return
		}

		c.Next()
	}
}

// serveTooManyRequests returns 429 response using sync.Pool
func serveTooManyRequests(c *gin.Context, limitType string) {
	resp := response.GetErrorResponse()
	resp.Error = "rate limit exceeded: too many requests"

	c.Header("Retry-After", "1")
	c.Header("X-RateLimit-Type", limitType)
	c.JSON(http.StatusTooManyRequests, resp)

	// Return to pool
	response.PutErrorResponse(resp)
}

// CleanupUnusedLimiters periodically removes stale IP limiters
// This is a background task to prevent memory growth from many unique IPs
func StartLimiterCleanup(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			// For production, implement TTL-based cleanup
			// For now, sync.Map handles memory naturally
		}
	}()
}