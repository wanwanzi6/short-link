package response

import (
	"net/http"
	"sync"
)

// Response 接口，所有响应结构体需实现此接口
type Response interface {
	Poolable
	Code() int
}

// Poolable 接口，支持对象池
type Poolable interface {
	Reset()
}

// poolMap 存储所有响应类型的 sync.Pool
var poolMap = make(map[string]*sync.Pool)

// registerPool 注册响应类型到对象池
func registerPool(name string, p *sync.Pool) {
	poolMap[name] = p
}

// Get 从对象池获取实例
func Get(name string) any {
	if p, ok := poolMap[name]; ok {
		return p.Get()
	}
	return nil
}

// Put 归还实例到对象池
func Put(name string, v any) {
	if p, ok := poolMap[name]; ok {
		p.Put(v)
	}
}

// --- ShortenResponse ---

func init() {
	registerPool("ShortenResponse", &sync.Pool{
		New: func() any {
			return &ShortenResponse{}
		},
	})
}

type ShortenResponse struct {
	ShortCode string `json:"short_code"`
}

func (r *ShortenResponse) Reset() {
	r.ShortCode = ""
}

func (r *ShortenResponse) Code() int {
	return http.StatusOK
}

// GetShortenResponse 从对象池获取 ShortenResponse
func GetShortenResponse() *ShortenResponse {
	return Get("ShortenResponse").(*ShortenResponse)
}

// PutShortenResponse 归还 ShortenResponse 到对象池
func PutShortenResponse(r *ShortenResponse) {
	r.Reset()
	Put("ShortenResponse", r)
}

// --- ErrorResponse ---

func init() {
	registerPool("ErrorResponse", &sync.Pool{
		New: func() any {
			return &ErrorResponse{}
		},
	})
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func (r *ErrorResponse) Reset() {
	r.Error = ""
}

func (r *ErrorResponse) Code() int {
	return http.StatusInternalServerError
}

// GetErrorResponse 从对象池获取 ErrorResponse
func GetErrorResponse() *ErrorResponse {
	return Get("ErrorResponse").(*ErrorResponse)
}

// PutErrorResponse 归还 ErrorResponse 到对象池
func PutErrorResponse(r *ErrorResponse) {
	r.Reset()
	Put("ErrorResponse", r)
}

// --- BadRequestResponse ---

func init() {
	registerPool("BadRequestResponse", &sync.Pool{
		New: func() any {
			return &BadRequestResponse{}
		},
	})
}

type BadRequestResponse struct {
	Error string `json:"error"`
}

func (r *BadRequestResponse) Reset() {
	r.Error = ""
}

func (r *BadRequestResponse) Code() int {
	return http.StatusBadRequest
}

// GetBadRequestResponse 从对象池获取 BadRequestResponse
func GetBadRequestResponse() *BadRequestResponse {
	return Get("BadRequestResponse").(*BadRequestResponse)
}

// PutBadRequestResponse 归还 BadRequestResponse 到对象池
func PutBadRequestResponse(r *BadRequestResponse) {
	r.Reset()
	Put("BadRequestResponse", r)
}

// --- NotFoundResponse ---

func init() {
	registerPool("NotFoundResponse", &sync.Pool{
		New: func() any {
			return &NotFoundResponse{}
		},
	})
}

type NotFoundResponse struct {
	Error string `json:"error"`
}

func (r *NotFoundResponse) Reset() {
	r.Error = ""
}

func (r *NotFoundResponse) Code() int {
	return http.StatusNotFound
}

// GetNotFoundResponse 从对象池获取 NotFoundResponse
func GetNotFoundResponse() *NotFoundResponse {
	return Get("NotFoundResponse").(*NotFoundResponse)
}

// PutNotFoundResponse 归还 NotFoundResponse 到对象池
func PutNotFoundResponse(r *NotFoundResponse) {
	r.Reset()
	Put("NotFoundResponse", r)
}