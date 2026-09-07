package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"LotteryPredictions_API/internal/config"
	"LotteryPredictions_API/internal/database"
	"LotteryPredictions_API/internal/handler"
	"LotteryPredictions_API/internal/middleware"
	"LotteryPredictions_API/internal/repository"
)

func main() {
	// .env / 環境変数から設定を読み込む
	cfg := config.Load()
	cfg.LogSummary()
	gin.SetMode(cfg.GinMode)

	// MySQL 接続
	if err := database.Connect(cfg); err != nil {
		log.Fatalf("database connection error: %v", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			log.Printf("database close error: %v", err)
		}
	}()

	lotteryRepo := repository.NewLotteryRepository(database.DB)
	lotteryHandler := handler.NewLotteryHandler(lotteryRepo)

	r := gin.Default()

	// 信頼するプロキシを明示する。
	// TRUSTED_PROXIES が未設定なら nil = どのプロキシも信頼せず、
	// X-Forwarded-For などの偽装可能なヘッダーを無視する。
	if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		log.Fatalf("invalid TRUSTED_PROXIES: %v", err)
	}
	if len(cfg.TrustedProxies) == 0 {
		log.Println("trusted proxies: なし (接続元IPをそのまま使用します)")
	} else {
		log.Printf("trusted proxies: %v", cfg.TrustedProxies)
	}

	// CORS (GitHub Pages などのフロントエンドからのアクセスを許可する)
	r.Use(middleware.CORS(cfg))

	// BASE_PATH ("/lottery" など) を全ルートの先頭に付ける。
	// 未設定ならルート直下 (/health, /api/...) になる。
	root := r.Group(cfg.BasePath)

	root.GET("/health", func(c *gin.Context) {
		if err := database.DB.Ping(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "ng"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	api := root.Group("/api")
	{
		api.GET("/predictions", lotteryHandler.List)
		api.GET("/predictions/:id", lotteryHandler.Get)
	}

	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
