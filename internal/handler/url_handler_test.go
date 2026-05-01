package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/bits-and-blooms/bloom/v3"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/wanwanzi6/short-link/internal/model"
	"github.com/wanwanzi6/short-link/internal/service"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// setupHandlerTest 创建测试所需的 handler 和清理函数
func setupHandlerTest(t *testing.T) (*gin.Engine, *bloom.BloomFilter, *gorm.DB, func()) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	err = db.AutoMigrate(&model.URL{})
	require.NoError(t, err)

	mr, err := miniredis.Run()
	require.NoError(t, err)

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	filter := bloom.NewWithEstimates(1000, 0.01)
	svc := service.NewURLService(db, rdb, filter)
	handler := NewURLHandler(svc)

	// 创建 Gin 引擎，注册实际路由
	r := gin.New()
	r.POST("/api/shorten", handler.ShortenURL)
	r.GET("/:short_code", handler.Redirect)

	cleanup := func() {
		mr.Close()
		rdb.Close()
	}

	return r, filter, db, cleanup
}

// TestShortenURL_Success 测试 POST /api/shorten 成功场景
func TestShortenURL_Success(t *testing.T) {
	r, _, _, cleanup := setupHandlerTest(t)
	defer cleanup()

	body := bytes.NewBufferString(`{"long_url": "https://example.com/test"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/shorten", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp ShortenResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.NotEmpty(t, resp.ShortCode)
}

// TestShortenURL_EmptyBody 测试空请求体
func TestShortenURL_EmptyBody(t *testing.T) {
	r, _, _, cleanup := setupHandlerTest(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/api/shorten", bytes.NewBufferString(""))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp["error"], "invalid request")
}

// TestShortenURL_EmptyURL 测试空 URL 字段
func TestShortenURL_EmptyURL(t *testing.T) {
	r, _, _, cleanup := setupHandlerTest(t)
	defer cleanup()

	body := bytes.NewBufferString(`{"long_url": ""}`)
	req := httptest.NewRequest(http.MethodPost, "/api/shorten", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp["error"], "long_url cannot be empty")
}

// TestShortenURL_InvalidJSON 测试无效 JSON
func TestShortenURL_InvalidJSON(t *testing.T) {
	r, _, _, cleanup := setupHandlerTest(t)
	defer cleanup()

	body := bytes.NewBufferString(`{invalid json}`)
	req := httptest.NewRequest(http.MethodPost, "/api/shorten", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestRedirect_NotFound 测试短码不存在（布隆过滤器拦截）
func TestRedirect_NotFound(t *testing.T) {
	r, _, _, cleanup := setupHandlerTest(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp["error"], "short code not found")
}

// TestRedirect_Success 测试重定向成功
func TestRedirect_Success(t *testing.T) {
	r, filter, db, cleanup := setupHandlerTest(t)
	defer cleanup()

	// 先创建一条记录
	urlRecord := &model.URL{
		OriginalURL: "https://redirect-test.com",
		ShortCode:   "redir",
	}
	db.Create(urlRecord)
	// 需要将 short_code 加入布隆过滤器，否则会被拦截
	filter.Add([]byte("redir"))

	req := httptest.NewRequest(http.MethodGet, "/redir", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "https://redirect-test.com", w.Header().Get("Location"))
}

// TestRedirect_ClicksUpdated 测试访问后点击数增加
func TestRedirect_ClicksUpdated(t *testing.T) {
	r, filter, db, cleanup := setupHandlerTest(t)
	defer cleanup()

	// 创建记录，初始 clicks = 0
	urlRecord := &model.URL{
		OriginalURL: "https://click-test.com",
		ShortCode:   "click",
		Clicks:      0,
	}
	db.Create(urlRecord)
	// 需要将 short_code 加入布隆过滤器
	filter.Add([]byte("click"))

	// 访问一次
	req := httptest.NewRequest(http.MethodGet, "/click", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusFound, w.Code)

	// 验证 clicks 已更新
	var updated model.URL
	db.Where("short_code = ?", "click").First(&updated)
	assert.Greater(t, updated.Clicks, int64(0))
}

// TestRedirect_EmptyShortCode 测试空短码
func TestRedirect_EmptyShortCode(t *testing.T) {
	r, _, _, cleanup := setupHandlerTest(t)
	defer cleanup()

	// Gin 路由 /:short_code 不会匹配空路径 /
	// 所以会返回 404，而不是 400
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	// 期望 404，因为路由不匹配
	assert.Equal(t, http.StatusNotFound, w.Code)
}