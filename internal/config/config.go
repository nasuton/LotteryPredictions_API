package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Getenv は環境変数を返す関数。テストでは map ベースの実装に差し替える
type Getenv func(key string) string

// MapGetenv は map から環境変数を引く Getenv を返す (テスト用)
func MapGetenv(m map[string]string) Getenv {
	return func(key string) string { return m[key] }
}

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
	// DBQueryTimeout はリクエスト単位で DB 操作に与えるタイムアウト
	DBQueryTimeout time.Duration

	// サーバ設定
	Port    string
	GinMode string

	// HTTP サーバのタイムアウト類
	HTTPReadHeaderTimeout time.Duration
	HTTPReadTimeout       time.Duration
	HTTPWriteTimeout      time.Duration
	HTTPIdleTimeout       time.Duration
	HTTPMaxHeaderBytes    int
	// ShutdownTimeout は graceful shutdown で処理中リクエストの完了を待つ上限
	ShutdownTimeout time.Duration

	// BasePath は全ルートの先頭に付けるパス (例: "/lottery")。
	// 空文字ならルート直下。末尾スラッシュは自動で除去される。
	BasePath string

	// TrustedProxies は ClientIP() 判定時に信頼するプロキシの IP / CIDR。
	// nil の場合はどのプロキシも信頼せず、TCP 接続元 IP をそのまま使用する。
	TrustedProxies []string

	// AllowedOrigins は CORS で許可するオリジン (例: https://user.github.io)。
	// 空の場合はクロスオリジン要求を一切許可しない。
	AllowedOrigins []string
	// CORSMaxAge はプリフライト結果をブラウザがキャッシュする時間
	CORSMaxAge time.Duration

	// ログ設定
	LogFormat string     // "json" | "text"
	LogLevel  slog.Level // debug | info | warn | error
}

// LoadDotEnv は ENV_FILE (既定 ".env") を読み込み OS 環境変数に反映する。
// 既に設定済みの環境変数は上書きしない。ファイルが無い場合は loaded=false, err=nil。
func LoadDotEnv() (path string, loaded bool, err error) {
	path = os.Getenv("ENV_FILE")
	if path == "" {
		path = ".env"
	}
	if abs, e := filepath.Abs(path); e == nil {
		path = abs
	}
	if err := godotenv.Load(path); err != nil {
		if os.IsNotExist(err) {
			return path, false, nil
		}
		return path, false, err
	}
	return path, true, nil
}

// Load は .env を読み込んだうえで OS 環境変数から設定を組み立てる
func Load() (*Config, error) {
	if _, _, err := LoadDotEnv(); err != nil {
		return nil, err
	}
	return Parse(os.Getenv)
}

// Parse は getenv から設定を組み立てる純粋関数。
// 必須項目の欠落や値の形式不正はまとめてエラーとして返す。
func Parse(getenv Getenv) (*Config, error) {
	p := &parser{getenv: getenv}

	cfg := &Config{
		DBUser:     p.str("DB_USER", "root"),
		DBPassword: p.required("DB_PASSWORD"),
		DBHost:     p.str("DB_HOST", "127.0.0.1"),
		DBPort:     p.str("DB_PORT", "3306"),
		DBName:     p.required("DB_NAME"),

		DBMaxOpenConns:    p.integer("DB_MAX_OPEN_CONNS", 25),
		DBMaxIdleConns:    p.integer("DB_MAX_IDLE_CONNS", 25),
		DBConnMaxLifetime: p.duration("DB_CONN_MAX_LIFETIME", 5*time.Minute),
		DBQueryTimeout:    p.duration("DB_QUERY_TIMEOUT", 3*time.Second),

		Port:    p.str("PORT", "8080"),
		GinMode: p.oneOf("GIN_MODE", "release", "debug", "release", "test"),

		HTTPReadHeaderTimeout: p.duration("HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
		HTTPReadTimeout:       p.duration("HTTP_READ_TIMEOUT", 10*time.Second),
		HTTPWriteTimeout:      p.duration("HTTP_WRITE_TIMEOUT", 15*time.Second),
		HTTPIdleTimeout:       p.duration("HTTP_IDLE_TIMEOUT", 60*time.Second),
		HTTPMaxHeaderBytes:    p.integer("HTTP_MAX_HEADER_BYTES", 1<<20),
		ShutdownTimeout:       p.duration("SHUTDOWN_TIMEOUT", 10*time.Second),

		BasePath: normalizeBasePath(p.str("BASE_PATH", "")),

		TrustedProxies: p.list("TRUSTED_PROXIES"),

		AllowedOrigins: p.list("ALLOWED_ORIGINS"),
		CORSMaxAge:     p.duration("CORS_MAX_AGE", 12*time.Hour),

		LogFormat: p.oneOf("LOG_FORMAT", "json", "json", "text"),
		LogLevel:  p.level("LOG_LEVEL", slog.LevelInfo),
	}

	if _, err := strconv.Atoi(cfg.Port); err != nil {
		p.errorf("PORT の値 %q はポート番号ではありません", cfg.Port)
	}

	if err := p.err(); err != nil {
		return nil, err
	}
	return cfg, nil
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
func (c *Config) LogSummary(logger *slog.Logger) {
	logger.Info("設定を読み込みました",
		"db", c.SafeDSN(),
		"port", c.Port,
		"gin_mode", c.GinMode,
		"base_path", c.BasePath,
		"trusted_proxies", c.TrustedProxies,
		"allowed_origins", c.AllowedOrigins,
		"log_format", c.LogFormat,
		"log_level", c.LogLevel.String(),
		"http_read_header_timeout", c.HTTPReadHeaderTimeout.String(),
		"http_read_timeout", c.HTTPReadTimeout.String(),
		"http_write_timeout", c.HTTPWriteTimeout.String(),
		"http_idle_timeout", c.HTTPIdleTimeout.String(),
		"db_query_timeout", c.DBQueryTimeout.String(),
		"shutdown_timeout", c.ShutdownTimeout.String(),
	)
	if len(c.TrustedProxies) == 0 {
		logger.Info("trusted proxies なし: 接続元 IP をそのまま使用します")
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

// parser は環境変数の取得と検証エラーの蓄積を担う
type parser struct {
	getenv Getenv
	errs   []error
}

func (p *parser) errorf(format string, args ...any) {
	p.errs = append(p.errs, fmt.Errorf(format, args...))
}

func (p *parser) err() error {
	if len(p.errs) == 0 {
		return nil
	}
	return fmt.Errorf("config: %w", errors.Join(p.errs...))
}

func (p *parser) str(key, def string) string {
	if v := p.getenv(key); v != "" {
		return v
	}
	return def
}

// required は必須項目を取得する。未設定ならエラーを記録する
func (p *parser) required(key string) string {
	v := p.getenv(key)
	if v == "" {
		p.errorf("%s は必須です (環境変数または .env で設定してください)", key)
	}
	return v
}

func (p *parser) integer(key string, def int) int {
	v := p.getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		p.errorf("%s の値 %q は整数ではありません", key, v)
		return def
	}
	return n
}

// duration は Go の期間文字列 (例: "5m", "30s") として取得する
func (p *parser) duration(key string, def time.Duration) time.Duration {
	v := p.getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		p.errorf("%s の値 %q は期間として解釈できません (例: 5s, 1m)", key, v)
		return def
	}
	return d
}

// list はカンマ区切りの環境変数を文字列スライスとして取得する。
// 未設定または空要素のみの場合は nil を返す。
func (p *parser) list(key string) []string {
	v := strings.TrimSpace(p.getenv(key))
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	list := make([]string, 0, len(parts))
	for _, s := range parts {
		if s = strings.TrimSpace(s); s != "" {
			list = append(list, s)
		}
	}
	if len(list) == 0 {
		return nil
	}
	return list
}

// oneOf は許可値のいずれかであることを検証して取得する
func (p *parser) oneOf(key, def string, allowed ...string) string {
	v := strings.ToLower(strings.TrimSpace(p.getenv(key)))
	if v == "" {
		return def
	}
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	p.errorf("%s の値 %q は %s のいずれかにしてください", key, v, strings.Join(allowed, "|"))
	return def
}

func (p *parser) level(key string, def slog.Level) slog.Level {
	v := strings.TrimSpace(p.getenv(key))
	if v == "" {
		return def
	}
	var l slog.Level
	if err := l.UnmarshalText([]byte(v)); err != nil {
		p.errorf("%s の値 %q は debug|info|warn|error のいずれかにしてください", key, v)
		return def
	}
	return l
}
