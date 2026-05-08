package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/wanwanzi6/short-link/internal/response"
	"github.com/wanwanzi6/short-link/internal/service"
)

// URLHandler HTTP 处理器
// 负责处理 HTTP 请求和响应
type URLHandler struct {
	svc *service.URLService
}

// NewURLHandler 创建一个新的 URLHandler 实例
func NewURLHandler(svc *service.URLService) *URLHandler {
	return &URLHandler{
		svc: svc,
	}
}

// ShortenRequest 短链接生成请求结构
type ShortenRequest struct {
	LongURL string `json:"long_url"`
}

// ShortenURL 处理短链接生成请求
//
// 请求方法：POST
// 请求体：{"long_url": "https://example.com/very/long/url"}
//
// 流程：
//  1. 解析 JSON 请求体，获取 long_url 字段
//  2. 校验 URL 是否合法（不能为空）
//  3. 调用 service 层生成短码
//  4. 返回生成的短码
func (h *URLHandler) ShortenURL(c *gin.Context) {
	var req ShortenRequest

	// 解析 JSON 请求体
	// 如果解析失败或格式不对，返回 400 错误
	if err := c.ShouldBindJSON(&req); err != nil {
		resp := response.GetBadRequestResponse()
		resp.Error = "invalid request: " + err.Error()
		c.JSON(resp.Code(), resp)
		response.PutBadRequestResponse(resp)
		return
	}

	// 校验 URL 不能为空
	if req.LongURL == "" {
		resp := response.GetBadRequestResponse()
		resp.Error = "long_url cannot be empty"
		c.JSON(resp.Code(), resp)
		response.PutBadRequestResponse(resp)
		return
	}

	// 调用 service 层生成短码
	code, err := h.svc.ShortenURL(req.LongURL)
	if err != nil {
		resp := response.GetErrorResponse()
		resp.Error = "failed to shorten URL: " + err.Error()
		c.JSON(resp.Code(), resp)
		response.PutErrorResponse(resp)
		return
	}

	// 返回成功响应
	resp := response.GetShortenResponse()
	resp.ShortCode = code
	c.JSON(resp.Code(), resp)
	response.PutShortenResponse(resp)
}

// Redirect 处理短链接跳转请求
//
// 请求方法：GET
// 路由参数：short_code
//
// 流程：
//  1. 从 URL 路径参数获取 short_code
//  2. 调用 service 层查询原始长链接
//  3. 如果找不到，返回 404
//  4. 如果找到，使用 302 重定向到原始 URL
func (h *URLHandler) Redirect(c *gin.Context) {
	// 从路由参数获取 short_code
	shortCode := c.Param("short_code")
	if shortCode == "" {
		resp := response.GetBadRequestResponse()
		resp.Error = "short_code is required"
		c.JSON(resp.Code(), resp)
		response.PutBadRequestResponse(resp)
		return
	}

	// 调用 service 层获取原始 URL
	longURL, err := h.svc.GetOriginalURL(shortCode)
	if err != nil {
		// 返回 404 Not Found
		resp := response.GetNotFoundResponse()
		resp.Error = "short code not found"
		c.JSON(resp.Code(), resp)
		response.PutNotFoundResponse(resp)
		return
	}

	// 使用 302 重定向到原始 URL
	// StatusFound (302) 是临时重定向，浏览器会跳转到新地址
	// 同时增加点击数（异步更新，不阻塞重定向）
	_ = h.svc.UpdateClickCount(shortCode)
	c.Redirect(http.StatusFound, longURL)
}