package handler

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"LotteryPredictions_API/internal/middleware"
	"LotteryPredictions_API/internal/model"
	"LotteryPredictions_API/internal/repository"
	"LotteryPredictions_API/internal/response"
)

type HitRateHandler struct {
	repo   repository.HitRateRepository
	logger *slog.Logger
}

func NewHitRateHandler(repo repository.HitRateRepository, logger *slog.Logger) *HitRateHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &HitRateHandler{repo: repo, logger: logger}
}

// List GET /api/v1/lottery_hit_rates?lottery_type=loto6&limit=20&offset=0
func (h *HitRateHandler) List(c *gin.Context) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", strconv.Itoa(defaultLimit)))
	if err != nil || limit <= 0 {
		limit = defaultLimit
	} else if limit > maxLimit {
		limit = maxLimit
	}
	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 {
		offset = 0
	}
	lotteryType := strings.TrimSpace(c.Query("lottery_type"))
	if lotteryType != "" && !model.IsValidHitRateLotteryType(lotteryType) {
		response.Error(c, http.StatusBadRequest, "invalid_lottery_type",
			"lottery_type must be one of "+strings.Join(model.HitRateLotteryTypes, ", "))
		return
	}

	ctx := c.Request.Context()
	total, err := h.repo.CountHitRates(ctx, lotteryType)
	if err != nil {
		h.logError(c, "count hit rates", err)
		response.Error(c, http.StatusInternalServerError, "internal_error", "failed to fetch lottery hit rates")
		return
	}
	hitRates, err := h.repo.FindHitRates(ctx, lotteryType, limit, offset)
	if err != nil {
		h.logError(c, "fetch hit rates", err)
		response.Error(c, http.StatusInternalServerError, "internal_error", "failed to fetch lottery hit rates")
		return
	}
	if hitRates == nil {
		hitRates = []model.LotteryHitRate{}
	}
	var nextURL *string
	if len(hitRates) > 0 && int64(limit) < total-int64(offset) {
		next := *c.Request.URL
		query := next.Query()
		query.Set("limit", strconv.Itoa(limit))
		query.Set("offset", strconv.FormatInt(int64(offset)+int64(limit), 10))
		next.RawQuery = query.Encode()
		url := next.RequestURI()
		nextURL = &url
	}

	response.SetCache(c, response.CacheControlStatus)
	c.JSON(http.StatusOK, gin.H{
		"data":     hitRates,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
		"next_url": nextURL,
	})
}

func (h *HitRateHandler) logError(c *gin.Context, op string, err error) {
	h.logger.Error("hit rate handler: "+op,
		"error", err,
		"request_id", middleware.GetRequestID(c),
		"path", c.Request.URL.Path,
	)
}
