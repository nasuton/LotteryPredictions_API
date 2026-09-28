package handler

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"LotteryPredictions_API/internal/middleware"
	"LotteryPredictions_API/internal/response"
)

// Pinger は DB 疎通確認の抽象 (*sql.DB が満たす)
type Pinger interface {
	PingContext(ctx context.Context) error
}

// HealthHandler は liveness / readiness を提供する
type HealthHandler struct {
	pinger  Pinger
	timeout time.Duration
	logger  *slog.Logger
}

func NewHealthHandler(pinger Pinger, timeout time.Duration, logger *slog.Logger) *HealthHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &HealthHandler{pinger: pinger, timeout: timeout, logger: logger}
}

// Liveness GET /healthz: プロセスが応答できるかのみを返す (DB には触れない)
func (h *HealthHandler) Liveness(c *gin.Context) {
	response.SetCache(c, response.CacheControlNoStore)
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// Readiness GET /readyz, /health: DB への PingContext が timeout 内に成功すれば 200、失敗なら 503
func (h *HealthHandler) Readiness(c *gin.Context) {
	response.SetCache(c, response.CacheControlNoStore)

	ctx, cancel := context.WithTimeout(c.Request.Context(), h.timeout)
	defer cancel()

	if err := h.pinger.PingContext(ctx); err != nil {
		h.logger.Warn("readiness check failed", "error", err, "request_id", middleware.GetRequestID(c))
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "ng"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
