package database

import (
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"

	"LotteryPredictions_API/internal/config"
)

// DB はアプリ全体で共有するコネクションプール
var DB *sql.DB

// Connect は設定に従い MySQL へ接続し、コネクションプールを初期化する
func Connect(cfg *config.Config) error {
	db, err := sql.Open("mysql", cfg.DSN())
	if err != nil {
		return fmt.Errorf("failed to open mysql: %w", err)
	}

	db.SetMaxOpenConns(cfg.DBMaxOpenConns)
	db.SetMaxIdleConns(cfg.DBMaxIdleConns)
	db.SetConnMaxLifetime(cfg.DBConnMaxLifetime)

	if err := db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("failed to ping mysql: %w", err)
	}

	DB = db
	return nil
}

// Close は接続を閉じる
func Close() error {
	if DB == nil {
		return nil
	}
	return DB.Close()
}
