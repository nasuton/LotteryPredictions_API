package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config はアプリケーション全体の設定
type Config struct {
	// DB 接続設定
	DBUser     string
	DBPassword string
	DBHost     string
	DBPort     string
	DBName     string

	// コネクションプール設定
	DBMaxOpenConns    int
	DBMaxIdleConns    int
	DBConnMaxLifetime time.Duration

	// サーバ設定
	Port    string
	GinMode string

	// BasePath は全ルートの先頭に付けるパス (例: "/lottery")。
	// 空文字ならルート直下。末尾スラッシュは自動で除去される。
	BasePath string

	// TrustedProxies は ClientIP() 判定時に信頼するプロキシの IP / CIDR。
	// nil の場合はどのプロキシも信頼せず、TCP 接続元 IP をそのまま使用する。
	TrustedProxies []string

	// AllowedOrigins は CORS で許可するオリジン (例: https://user.github.io)
	AllowedOrigins []string
	// CORSMaxAge はプリフライト結果をブラウザがキャッシュする時間
	CORSMaxAge time.Duration
}

// Load は .env を読み込んだうえで環境変数から設定を組み立てる。
// .env が存在しない場合はエラーにせず、OS の環境変数のみを使用する。
func Load() *Config {
	// 明示的にファイルを指定したい場合は ENV_FILE を利用する
	envFile := os.Getenv("ENV_FILE")
	if envFile == "" {
		envFile = ".env"
	}

	if err := godotenv.Load(envFile); err != nil {
		abs, _ := filepath.Abs(envFile)
		if os.IsNotExist(err) {
			log.Printf("config: %s (絶対パス: %s) が見つからないため環境変数のみを使用します", envFile, abs)
		} else {
			log.Printf("config: %s (絶対パス: %s) の読み込みに失敗しました: %v", envFile, abs, err)
		}
	} else {
		log.Printf("config: %s を読み込みました", envFile)
	}

	return &Config{
		DBUser:     getEnv("DB_USER", "root"),
		DBPassword: getEnv("DB_PASSWORD", "password"),
		DBHost:     getEnv("DB_HOST", "127.0.0.1"),
		DBPort:     getEnv("DB_PORT", "3306"),
		DBName:     getEnv("DB_NAME", "lottery"),

		DBMaxOpenConns:    getEnvInt("DB_MAX_OPEN_CONNS", 25),
		DBMaxIdleConns:    getEnvInt("DB_MAX_IDLE_CONNS", 25),
		DBConnMaxLifetime: getEnvDuration("DB_CONN_MAX_LIFETIME", 5*time.Minute),

		Port:    getEnv("PORT", "8080"),
		GinMode: getEnv("GIN_MODE", "debug"),

		BasePath: normalizeBasePath(getEnv("BASE_PATH", "")),

		TrustedProxies: getEnvList("TRUSTED_PROXIES"),

		AllowedOrigins: getEnvList("ALLOWED_ORIGINS"),
		CORSMaxAge:     getEnvDuration("CORS_MAX_AGE", 12*time.Hour),
	}
}

// DSN は go-sql-driver/mysql 用の接続文字列を返す
func (c *Config) DSN() string {
	return fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&loc=Local",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName,
	)
}

// SafeDSN はパスワードを伏せた接続情報を返す (ログ出力用)
func (c *Config) SafeDSN() string {
	pw := "(未設定)"
	if c.DBPassword != "" {
		pw = "****"
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", c.DBUser, pw, c.DBHost, c.DBPort, c.DBName)
}

// LogSummary は起動時に読み込まれた設定の概要をログ出力する
func (c *Config) LogSummary() {
	log.Printf("config: DB接続先 = %s", c.SafeDSN())
	log.Printf("config: PORT=%s GIN_MODE=%s", c.Port, c.GinMode)
	if c.BasePath == "" {
		log.Println("config: BASE_PATH=(なし) 例) /health, /api/predictions")
	} else {
		log.Printf("config: BASE_PATH=%s 例) %s/health, %s/api/predictions",
			c.BasePath, c.BasePath, c.BasePath)
	}

	// DB_USER / DB_PASSWORD が未設定のままだとデフォルト値で接続を試み、
	// Access denied (Error 1045) になりやすいので警告する
	if os.Getenv("DB_USER") == "" || os.Getenv("DB_PASSWORD") == "" {
		log.Println("config: 警告 DB_USER / DB_PASSWORD が環境変数にありません。" +
			".env の場所 (カレントディレクトリ or ENV_FILE) と systemd の EnvironmentFile を確認してください")
	}
}

// normalizeBasePath は BASE_PATH を "/xxx" 形式に整える。
// 空文字や "/" の場合は空文字 (ルート直下) を返す。
func normalizeBasePath(v string) string {
	v = strings.TrimSpace(v)
	v = strings.Trim(v, "/")
	if v == "" {
		return ""
	}
	return "/" + v
}

// getEnv は環境変数を取得し、未設定ならデフォルト値を返す
func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// getEnvInt は環境変数を int として取得する
func getEnvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		log.Printf("config: %s の値 %q は整数ではないためデフォルト値 %d を使用します", key, v, def)
		return def
	}
	return n
}

// getEnvList はカンマ区切りの環境変数を文字列スライスとして取得する。
// 未設定または空の場合は nil を返す。
func getEnvList(key string) []string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	list := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			list = append(list, p)
		}
	}
	if len(list) == 0 {
		return nil
	}
	return list
}

// getEnvDuration は環境変数を time.Duration として取得する (例: "5m", "30s")
func getEnvDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		log.Printf("config: %s の値 %q は期間として解釈できないためデフォルト値 %s を使用します", key, v, def)
		return def
	}
	return d
}
