// Package router は gin.Engine の組み立て (ミドルウェア・ルート登録) を担う
package router

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"LotteryPredictions_API/internal/config"
	"LotteryPredictions_API/internal/handler"
	"LotteryPredictions_API/internal/middleware"
	"LotteryPredictions_API/internal/repository"
	"LotteryPredictions_API/internal/response"
)

// readinessTimeout は /readyz, /health での DB Ping 上限
const readinessTimeout = time.Second

// Deps は router が必要とする依存
type Deps struct {
	Config      *config.Config
	Logger      *slog.Logger
	Predictions repository.PredictionRepository
	Status      repository.StatusRepository
	Pinger      handler.Pinger
}

// New は設定と依存からルーティング済みの gin.Engine を返す。
// gin.SetMode は呼び出し側で事前に行うこと。
func New(d Deps) (*gin.Engine, error) {
	cfg := d.Config
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}

	r := gin.New()

	// 信頼するプロキシを明示する。
	// TRUSTED_PROXIES が未設定なら nil = どのプロキシも信頼せず、
	// X-Forwarded-For などの偽装可能なヘッダーを無視する。
	if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		return nil, err
	}

	r.Use(
		middleware.RequestID(),
		middleware.Logger(logger, cfg.BasePath+"/healthz"),
		middleware.Recovery(logger, response.Error),
		middleware.CORS(cfg, logger),
	)

	r.NoRoute(func(c *gin.Context) {
		response.Error(c, http.StatusNotFound, "not_found", "route not found")
	})

	health := handler.NewHealthHandler(d.Pinger, readinessTimeout, logger)
	lottery := handler.NewLotteryHandler(d.Predictions, logger)
	status := handler.NewStatusHandler(d.Status, logger)

	// BASE_PATH ("/lottery" など) を全ルートの先頭に付ける。
	// 未設定ならルート直下 (/health, /api/...) になる。
	root := r.Group(cfg.BasePath)

	root.GET("/healthz", health.Liveness)
	root.GET("/readyz", health.Readiness)
	root.GET("/health", health.Readiness) // 互換: 旧来のヘルスチェック (readiness と同じ挙動)

	api := root.Group("/api", middleware.Timeout(cfg.DBQueryTimeout))
	{
		// 互換: 無印パス。将来的に廃止予定。v1 と同じ handler を登録する
		api.GET("/predictions", lottery.List)
		api.GET("/predictions/:id", lottery.Get)

		v1 := api.Group("/v1")
		v1.GET("/predictions", lottery.List)
		v1.GET("/predictions/:id", lottery.Get)
		v1.GET("/status", status.Status)
	}

	return r, nil
}
