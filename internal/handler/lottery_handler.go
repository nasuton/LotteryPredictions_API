package handler

import (
	"database/sql"
	"errors"
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

const (
	defaultLimit = 20
	maxLimit     = 100
)

// LotteryHandler は予測データの取得 API を提供する
type LotteryHandler struct {
	repo   repository.PredictionRepository
	logger *slog.Logger
}

func NewLotteryHandler(repo repository.PredictionRepository, logger *slog.Logger) *LotteryHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &LotteryHandler{repo: repo, logger: logger}
}

// List GET /api/v1/predictions?lottery_type=loto6&limit=20&offset=0
//
// next_url は呼ばれたパスをそのまま引き継ぐため、v1 で呼べば v1、無印で呼べば無印のパスになる。
func (h *LotteryHandler) List(c *gin.Context) {
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
	if lotteryType != "" && !model.IsValidLotteryType(lotteryType) {
		response.Error(c, http.StatusBadRequest, "invalid_lottery_type",
			"lottery_type must be one of "+strings.Join(model.LotteryTypes, ", "))
		return
	}

	ctx := c.Request.Context()

	snap, err := h.repo.Snapshot(ctx, lotteryType)
	if err != nil {
		h.logError(c, "snapshot predictions", err)
		response.Error(c, http.StatusInternalServerError, "internal_error", "failed to fetch predictions")
		return
	}
	total := snap.Count

	// updated_at 列に依存せず、DB の状態 (件数・最大 ID・最新予測日) と検索条件から弱い ETag を作る
	etag := response.WeakETag(lotteryType, limit, offset, snap.MaxPredictedAt.String(), total, snap.MaxID)
	if response.ETagMatches(c.GetHeader("If-None-Match"), etag) {
		response.SetCache(c, response.CacheControlPredictions)
		c.Header("ETag", etag)
		c.Status(http.StatusNotModified)
		return
	}

	predictions, err := h.repo.FindAll(ctx, lotteryType, limit, offset)
	if err != nil {
		h.logError(c, "fetch predictions", err)
		response.Error(c, http.StatusInternalServerError, "internal_error", "failed to fetch predictions")
		return
	}

	// 同じ API オリジンに対する相対 URL。検索条件と BASE_PATH、呼ばれたパス (v1 / 無印) を引き継ぐ。
	var nextURL *string
	if len(predictions) > 0 && int64(limit) < total-int64(offset) {
		next := *c.Request.URL
		query := next.Query()
		query.Set("limit", strconv.Itoa(limit))
		query.Set("offset", strconv.FormatInt(int64(offset)+int64(limit), 10))
		next.RawQuery = query.Encode()
		url := next.RequestURI()
		nextURL = &url
	}

	response.SetCache(c, response.CacheControlPredictions)
	c.Header("ETag", etag)
	c.JSON(http.StatusOK, gin.H{
		"data":     predictions,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
		"next_url": nextURL,
	})
}

// Get GET /api/v1/predictions/:id
func (h *LotteryHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, http.StatusBadRequest, "invalid_id", "id must be a positive integer")
		return
	}

	prediction, err := h.repo.FindByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.Error(c, http.StatusNotFound, "not_found", "prediction not found")
			return
		}
		h.logError(c, "fetch prediction", err)
		response.Error(c, http.StatusInternalServerError, "internal_error", "failed to fetch prediction")
		return
	}

	etag := response.WeakETag(prediction.ID, prediction.LotteryType, prediction.Pattern,
		prediction.PredictedAt.String(), prediction.Numbers)
	response.SetCache(c, response.CacheControlPredictions)
	c.Header("ETag", etag)
	if response.ETagMatches(c.GetHeader("If-None-Match"), etag) {
		c.Status(http.StatusNotModified)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": prediction})
}

// logError は内部エラーの詳細をクライアントに返さず slog にのみ記録する
func (h *LotteryHandler) logError(c *gin.Context, op string, err error) {
	h.logger.Error("lottery handler: "+op,
		"error", err,
		"request_id", middleware.GetRequestID(c),
		"path", c.Request.URL.Path,
	)
}
