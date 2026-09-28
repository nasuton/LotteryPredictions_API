package middleware

import (
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
)

// ErrorWriter は統一形式のエラーレスポンスを書き出す関数 (response.Error を渡す)
type ErrorWriter func(c *gin.Context, status int, code, message string)

// Recovery は panic を回復して slog に記録し、統一形式の 500 を返す
func Recovery(logger *slog.Logger, write ErrorWriter) gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, recovered any) {
		logger.Error("panic recovered",
			"error", recovered,
			"request_id", GetRequestID(c),
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"stack", string(debug.Stack()),
		)
		write(c, http.StatusInternalServerError, "internal_error", "internal server error")
		c.Abort()
	})
}
