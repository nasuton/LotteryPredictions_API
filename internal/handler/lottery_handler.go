package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"LotteryPredictions_API/internal/repository"
)

type LotteryHandler struct {
	repo *repository.LotteryRepository
}

func NewLotteryHandler(repo *repository.LotteryRepository) *LotteryHandler {
	return &LotteryHandler{repo: repo}
}

// List GET /api/predictions?lottery_type=loto6&limit=20&offset=0
func (h *LotteryHandler) List(c *gin.Context) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil || limit <= 0 {
		limit = 20
	} else if limit > 100 {
		limit = 100
	}
	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 {
		offset = 0
	}
	lotteryType := strings.TrimSpace(c.Query("lottery_type"))

	ctx := c.Request.Context()

	predictions, err := h.repo.FindAll(ctx, lotteryType, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch predictions"})
		return
	}

	total, err := h.repo.Count(ctx, lotteryType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to count predictions"})
		return
	}

	// 同じ API オリジンに対する相対 URL。検索条件と BASE_PATH を引き継ぐ。
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

	c.JSON(http.StatusOK, gin.H{
		"data":     predictions,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
		"next_url": nextURL,
	})
}

// Get GET /api/predictions/:id
func (h *LotteryHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	prediction, err := h.repo.FindByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "prediction not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch prediction"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": prediction})
}
