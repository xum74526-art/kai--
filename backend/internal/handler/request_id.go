package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	requestIDHeader     = "X-Request-ID"
	requestIDContextKey = "request-id"
)

var requestIDFallbackSequence atomic.Uint64

// RequestIDMiddleware 为每个请求生成服务端追踪编号。客户端传入值不会被反射，
// 避免把不可信控制字符或伪造的关联标识带入响应和服务端日志。
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ensureRequestID(c)
		c.Next()
	}
}

func ensureRequestID(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if value, exists := c.Get(requestIDContextKey); exists {
		if id, ok := value.(string); ok && validServerRequestID(id) {
			return id
		}
	}
	id := newRequestID()
	c.Set(requestIDContextKey, id)
	c.Header(requestIDHeader, id)
	return id
}

func requestID(c *gin.Context) string {
	if c == nil {
		return ""
	}
	value, exists := c.Get(requestIDContextKey)
	if !exists {
		return ""
	}
	id, _ := value.(string)
	if !validServerRequestID(id) {
		return ""
	}
	return id
}

func newRequestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err == nil {
		return hex.EncodeToString(value[:])
	}
	// 追踪编号不是凭证；熵源异常时仍保证固定、安全字符集和进程内差异性。
	fallback := strconv.FormatInt(time.Now().UnixNano(), 10) + ":" + strconv.FormatUint(requestIDFallbackSequence.Add(1), 10)
	digest := sha256.Sum256([]byte(fallback))
	return hex.EncodeToString(digest[:16])
}

func validServerRequestID(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
