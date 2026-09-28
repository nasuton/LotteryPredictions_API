package middleware

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
)

// Timeout はリクエストのコンテキストに期限を設定する。
// handler は c.Request.Context() を DB 操作に渡すことで期限を伝播させる。
func Timeout(d time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), d)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
