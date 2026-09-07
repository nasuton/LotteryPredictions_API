package middleware

import (
	"log"
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"LotteryPredictions_API/internal/config"
)

// CORS は設定に基づく CORS ミドルウェアを返す。
//
// ALLOWED_ORIGINS が未設定の場合はどのオリジンも許可しない。
// GitHub Pages から利用する場合は https://<ユーザー名>.github.io を指定する。
func CORS(cfg *config.Config) gin.HandlerFunc {
	if len(cfg.AllowedOrigins) == 0 {
		log.Println("cors: ALLOWED_ORIGINS が未設定のため、クロスオリジン要求は許可されません")
	} else {
		log.Printf("cors: allowed origins %v", cfg.AllowedOrigins)
	}

	return cors.New(cors.Config{
		// ワイルドカードではなく明示的なオリジンのみを許可する
		AllowOrigins: cfg.AllowedOrigins,
		AllowMethods: []string{
			http.MethodGet,
			http.MethodOptions,
		},
		AllowHeaders: []string{
			"Origin",
			"Content-Type",
			"Accept",
		},
		ExposeHeaders: []string{"Content-Length"},
		// Cookie / 認証情報は使用しないため false
		AllowCredentials: false,
		MaxAge:           cfg.CORSMaxAge,
	})
}

