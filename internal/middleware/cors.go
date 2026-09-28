package middleware

import (
	"log/slog"
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"LotteryPredictions_API/internal/config"
)

// CORS は設定に基づく CORS ミドルウェアを返す。
//
// ALLOWED_ORIGINS が空の場合は CORS ヘッダを一切付けない (= どのオリジンも許可しない)。
// gin-contrib/cors は AllowOrigins が空だと起動時に panic するため、no-op に切り替える。
// GitHub Pages から利用する場合は https://<ユーザー名>.github.io を指定する。
func CORS(cfg *config.Config, logger *slog.Logger) gin.HandlerFunc {
	if len(cfg.AllowedOrigins) == 0 {
		logger.Warn("cors: ALLOWED_ORIGINS が未設定のため、クロスオリジン要求は許可されません")
		return func(c *gin.Context) { c.Next() }
	}
	logger.Info("cors: 許可オリジン", "allowed_origins", cfg.AllowedOrigins)

	return cors.New(cors.Config{
		// ワイルドカードではなく明示的なオリジンのみを許可する
		AllowOrigins: cfg.AllowedOrigins,
		AllowMethods: []string{
			http.MethodGet,
			http.MethodHead,
			http.MethodOptions,
		},
		AllowHeaders: []string{
			"Origin",
			"Content-Type",
			"Accept",
			"If-None-Match",
			HeaderRequestID,
		},
		ExposeHeaders: []string{
			"Content-Length",
			"ETag",
			"Cache-Control",
			HeaderRequestID,
		},
		// Cookie / 認証情報は使用しないため false
		AllowCredentials: false,
		MaxAge:           cfg.CORSMaxAge,
	})
}
