package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const requestIDKey = "request_id"

const maxRequestIDLength = 128

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := normalizeRequestID(c.GetHeader("X-Request-ID"))
		if id == "" {
			id = newRequestID()
		}
		c.Set(requestIDKey, id)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}

func normalizeRequestID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maxRequestIDLength {
		return ""
	}
	for i := 0; i < len(value); i++ {
		char := value[i]
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '-' || char == '_' || char == '.' || char == ':' {
			continue
		}
		return ""
	}
	return value
}

func RequestIDFromContext(c *gin.Context) string {
	value, ok := c.Get(requestIDKey)
	if !ok {
		return ""
	}
	id, ok := value.(string)
	if !ok {
		return ""
	}
	return id
}

func newRequestID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return hex.EncodeToString(raw[:])
	}
	return strconv.FormatInt(time.Now().UnixNano(), 16)
}
