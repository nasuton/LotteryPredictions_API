package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func minimalEnv(extra map[string]string) Getenv {
	m := map[string]string{
		"DB_PASSWORD": "secret",
		"DB_NAME":     "lottery",
	}
	for k, v := range extra {
		m[k] = v
	}
	return MapGetenv(m)
}

func TestParse_Defaults(t *testing.T) {
	cfg, err := Parse(minimalEnv(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DBUser != "root" || cfg.DBHost != "127.0.0.1" || cfg.DBPort != "3306" {
		t.Errorf("unexpected DB defaults: %+v", cfg)
	}
	if cfg.DBPassword != "secret" || cfg.DBName != "lottery" {
		t.Errorf("required values not propagated: %+v", cfg)
	}
	if cfg.Port != "8080" || cfg.GinMode != "release" {
		t.Errorf("unexpected server defaults: port=%s mode=%s", cfg.Port, cfg.GinMode)
	}
	if cfg.HTTPReadHeaderTimeout != 5*time.Second || cfg.HTTPReadTimeout != 10*time.Second ||
		cfg.HTTPWriteTimeout != 15*time.Second || cfg.HTTPIdleTimeout != 60*time.Second ||
		cfg.HTTPMaxHeaderBytes != 1<<20 || cfg.ShutdownTimeout != 10*time.Second ||
		cfg.DBQueryTimeout != 3*time.Second {
		t.Errorf("unexpected timeout defaults: %+v", cfg)
	}
	if cfg.AllowedOrigins != nil {
		t.Errorf("AllowedOrigins should be nil when unset, got %v", cfg.AllowedOrigins)
	}
	if cfg.TrustedProxies != nil {
		t.Errorf("TrustedProxies should be nil when unset, got %v", cfg.TrustedProxies)
	}
	if cfg.BasePath != "" {
		t.Errorf("BasePath should be empty, got %q", cfg.BasePath)
	}
	if cfg.LogFormat != "json" || cfg.LogLevel != slog.LevelInfo {
		t.Errorf("unexpected log defaults: %s %s", cfg.LogFormat, cfg.LogLevel)
	}
}

func TestParse_RequiredMissing(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want []string
	}{
		{"password missing", map[string]string{"DB_NAME": "lottery"}, []string{"DB_PASSWORD"}},
		{"name missing", map[string]string{"DB_PASSWORD": "x"}, []string{"DB_NAME"}},
		{"both missing", map[string]string{}, []string{"DB_PASSWORD", "DB_NAME"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Parse(MapGetenv(tt.env))
			if err == nil {
				t.Fatalf("expected error, got config %+v", cfg)
			}
			for _, key := range tt.want {
				if !strings.Contains(err.Error(), key) {
					t.Errorf("error should mention %s: %v", key, err)
				}
			}
		})
	}
}

func TestParse_Durations(t *testing.T) {
	cfg, err := Parse(minimalEnv(map[string]string{
		"HTTP_READ_HEADER_TIMEOUT": "2s",
		"HTTP_READ_TIMEOUT":        "30s",
		"HTTP_WRITE_TIMEOUT":       "1m",
		"HTTP_IDLE_TIMEOUT":        "90s",
		"SHUTDOWN_TIMEOUT":         "20s",
		"DB_QUERY_TIMEOUT":         "500ms",
		"DB_CONN_MAX_LIFETIME":     "10m",
		"CORS_MAX_AGE":             "1h",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTPReadHeaderTimeout != 2*time.Second || cfg.HTTPReadTimeout != 30*time.Second ||
		cfg.HTTPWriteTimeout != time.Minute || cfg.HTTPIdleTimeout != 90*time.Second ||
		cfg.ShutdownTimeout != 20*time.Second || cfg.DBQueryTimeout != 500*time.Millisecond ||
		cfg.DBConnMaxLifetime != 10*time.Minute || cfg.CORSMaxAge != time.Hour {
		t.Errorf("durations not parsed: %+v", cfg)
	}

	for _, bad := range []string{"10", "abc", "-5s"} {
		_, err := Parse(minimalEnv(map[string]string{"HTTP_READ_TIMEOUT": bad}))
		if err == nil || !strings.Contains(err.Error(), "HTTP_READ_TIMEOUT") {
			t.Errorf("HTTP_READ_TIMEOUT=%q should be rejected, got %v", bad, err)
		}
	}
}

func TestParse_Integers(t *testing.T) {
	cfg, err := Parse(minimalEnv(map[string]string{
		"DB_MAX_OPEN_CONNS":     "10",
		"DB_MAX_IDLE_CONNS":     "5",
		"HTTP_MAX_HEADER_BYTES": "4096",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DBMaxOpenConns != 10 || cfg.DBMaxIdleConns != 5 || cfg.HTTPMaxHeaderBytes != 4096 {
		t.Errorf("integers not parsed: %+v", cfg)
	}
	if _, err := Parse(minimalEnv(map[string]string{"DB_MAX_OPEN_CONNS": "many"})); err == nil {
		t.Error("non-integer DB_MAX_OPEN_CONNS should be rejected")
	}
	if _, err := Parse(minimalEnv(map[string]string{"PORT": "http"})); err == nil {
		t.Error("non-numeric PORT should be rejected")
	}
}

func TestParse_AllowedOrigins(t *testing.T) {
	tests := []struct {
		raw  string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{",, ,", nil},
		{"https://nasuton.github.io", []string{"https://nasuton.github.io"}},
		{" https://a.example , https://b.example ,", []string{"https://a.example", "https://b.example"}},
	}
	for _, tt := range tests {
		cfg, err := Parse(minimalEnv(map[string]string{"ALLOWED_ORIGINS": tt.raw}))
		if err != nil {
			t.Fatalf("ALLOWED_ORIGINS=%q: unexpected error %v", tt.raw, err)
		}
		if len(cfg.AllowedOrigins) != len(tt.want) {
			t.Fatalf("ALLOWED_ORIGINS=%q: got %v want %v", tt.raw, cfg.AllowedOrigins, tt.want)
		}
		for i := range tt.want {
			if cfg.AllowedOrigins[i] != tt.want[i] {
				t.Errorf("ALLOWED_ORIGINS=%q: got %v want %v", tt.raw, cfg.AllowedOrigins, tt.want)
			}
		}
	}
}

func TestParse_BasePath(t *testing.T) {
	tests := map[string]string{
		"":          "",
		"/":         "",
		"lottery":   "/lottery",
		"/lottery/": "/lottery",
		" /api/x/ ": "/api/x",
	}
	for raw, want := range tests {
		cfg, err := Parse(minimalEnv(map[string]string{"BASE_PATH": raw}))
		if err != nil {
			t.Fatalf("BASE_PATH=%q: %v", raw, err)
		}
		if cfg.BasePath != want {
			t.Errorf("BASE_PATH=%q: got %q want %q", raw, cfg.BasePath, want)
		}
	}
}

func TestParse_EnumValues(t *testing.T) {
	cfg, err := Parse(minimalEnv(map[string]string{"LOG_FORMAT": "text", "LOG_LEVEL": "debug", "GIN_MODE": "debug"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LogFormat != "text" || cfg.LogLevel != slog.LevelDebug || cfg.GinMode != "debug" {
		t.Errorf("enum values not parsed: %+v", cfg)
	}
	if _, err := Parse(minimalEnv(map[string]string{"LOG_FORMAT": "xml"})); err == nil {
		t.Error("LOG_FORMAT=xml should be rejected")
	}
	if _, err := Parse(minimalEnv(map[string]string{"LOG_LEVEL": "verbose"})); err == nil {
		t.Error("LOG_LEVEL=verbose should be rejected")
	}
	if _, err := Parse(minimalEnv(map[string]string{"GIN_MODE": "prod"})); err == nil {
		t.Error("GIN_MODE=prod should be rejected")
	}
}

func TestSafeDSN_MasksPassword(t *testing.T) {
	cfg, err := Parse(minimalEnv(nil))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(cfg.SafeDSN(), "secret") {
		t.Errorf("SafeDSN leaked password: %s", cfg.SafeDSN())
	}
	if !strings.Contains(cfg.DSN(), "secret") || !strings.Contains(cfg.DSN(), "parseTime=true") {
		t.Errorf("DSN malformed: %s", cfg.DSN())
	}
}
