package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"LotteryPredictions_API/internal/config"
	"LotteryPredictions_API/internal/database"
	"LotteryPredictions_API/internal/logging"
	"LotteryPredictions_API/internal/repository"
	"LotteryPredictions_API/internal/router"
)

// dbConnectTimeout は起動時の DB 接続 (初回 Ping) の上限
const dbConnectTimeout = 10 * time.Second

func main() {
	os.Exit(run())
}

func run() int {
	// 設定が読めるまでは text 形式の暫定ロガーで stderr に出す
	bootLogger := logging.New(os.Stderr, "text", slog.LevelInfo)

	envFile, envLoaded, envErr := config.LoadDotEnv()
	if envErr != nil {
		bootLogger.Error("環境変数ファイルの読み込みに失敗しました", "file", envFile, "error", envErr)
		return 1
	}

	cfg, err := config.Parse(os.Getenv)
	if err != nil {
		bootLogger.Error("設定エラーのため起動を中止します", "error", err)
		return 1
	}

	logger := logging.New(os.Stdout, cfg.LogFormat, cfg.LogLevel)
	slog.SetDefault(logger)

	if envLoaded {
		logger.Info("環境変数ファイルを読み込みました", "file", envFile)
	} else {
		logger.Info("環境変数ファイルが無いため OS の環境変数のみを使用します", "file", envFile)
	}
	cfg.LogSummary(logger)
	gin.SetMode(cfg.GinMode)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// MySQL 接続
	connCtx, cancelConn := context.WithTimeout(ctx, dbConnectTimeout)
	db, err := database.Connect(connCtx, cfg)
	cancelConn()
	if err != nil {
		logger.Error("データベース接続に失敗しました", "error", err, "db", cfg.SafeDSN())
		return 1
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Error("データベース切断に失敗しました", "error", err)
		} else {
			logger.Info("データベース接続を閉じました")
		}
	}()
	logger.Info("データベースに接続しました", "db", cfg.SafeDSN())

	repo := repository.NewLotteryRepository(db)
	engine, err := router.New(router.Deps{
		Config:      cfg,
		Logger:      logger,
		Predictions: repo,
		Status:      repo,
		Pinger:      db,
	})
	if err != nil {
		logger.Error("ルーターの初期化に失敗しました (TRUSTED_PROXIES を確認してください)", "error", err)
		return 1
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           engine,
		ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
		ReadTimeout:       cfg.HTTPReadTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout,
		IdleTimeout:       cfg.HTTPIdleTimeout,
		MaxHeaderBytes:    cfg.HTTPMaxHeaderBytes,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("HTTP サーバーを起動します", "addr", srv.Addr, "base_path", cfg.BasePath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
		close(serveErr)
	}()

	select {
	case <-ctx.Done():
		logger.Info("終了シグナルを受信しました。graceful shutdown を開始します", "timeout", cfg.ShutdownTimeout.String())
	case err := <-serveErr:
		if err != nil {
			logger.Error("HTTP サーバーが異常終了しました", "error", err)
			return 1
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown に失敗したため強制終了します", "error", err)
		_ = srv.Close()
		return 1
	}
	logger.Info("HTTP サーバーを停止しました")
	return 0
}
