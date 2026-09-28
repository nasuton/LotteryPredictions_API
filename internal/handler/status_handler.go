package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"LotteryPredictions_API/internal/middleware"
	"LotteryPredictions_API/internal/model"
	"LotteryPredictions_API/internal/repository"
	"LotteryPredictions_API/internal/response"
)

// StatusHandler は GET /api/v1/status (更新状況の見える化) を提供する
type StatusHandler struct {
	repo   repository.StatusRepository
	logger *slog.Logger
	now    func() time.Time
}

func NewStatusHandler(repo repository.StatusRepository, logger *slog.Logger) *StatusHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &StatusHandler{repo: repo, logger: logger, now: time.Now}
}

// Status GET /api/v1/status
//
// batch_runs テーブルが存在しない (バッチ側の移行前) 場合は last_batch_runs を null にして 200 を返す。
func (h *StatusHandler) Status(c *gin.Context) {
	ctx := c.Request.Context()
	requestID := middleware.GetRequestID(c)

	byType, err := h.repo.CountByType(ctx)
	if err != nil {
		h.logger.Error("status handler: count by type", "error", err, "request_id", requestID)
		response.Error(c, http.StatusInternalServerError, "internal_error", "failed to fetch status")
		return
	}
	if byType == nil {
		byType = []model.TypeSummary{}
	}
	var total int64
	for _, t := range byType {
		total += t.Count
	}

	var lastRuns any
	runs, err := h.repo.LastBatchRuns(ctx)
	switch {
	case errors.Is(err, repository.ErrTableNotFound):
		h.logger.Warn("status handler: batch_runs テーブルが存在しないため last_batch_runs は null を返します",
			"error", err, "request_id", requestID)
		lastRuns = nil
	case err != nil:
		h.logger.Error("status handler: last batch runs", "error", err, "request_id", requestID)
		response.Error(c, http.StatusInternalServerError, "internal_error", "failed to fetch status")
		return
	default:
		if runs == nil {
			runs = []model.BatchRun{}
		}
		lastRuns = runs
	}

	response.SetCache(c, response.CacheControlStatus)
	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"predictions": gin.H{
				"total":   total,
				"by_type": byType,
			},
			"last_batch_runs": lastRuns,
			"generated_at":    h.now().Format(time.RFC3339),
		},
	})
}
