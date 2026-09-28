package middleware

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"LotteryPredictions_API/internal/config"
)

func init() {
	gin.SetMode(gin.TestMode)
}

var discard = slog.New(slog.NewTextHandler(io.Discard, nil))

func newEngine(mw ...gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	r.Use(mw...)
	r.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, GetRequestID(c))
	})
	return r
}

func TestRequestID_Generated(t *testing.T) {
	r := newEngine(RequestID())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ping", nil))

	id := w.Header().Get(HeaderRequestID)
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(id) {
		t.Fatalf("generated request id should be 32 hex chars, got %q", id)
	}
	if w.Body.String() != id {
		t.Errorf("context id %q != header id %q", w.Body.String(), id)
	}

	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/ping", nil))
	if w2.Header().Get(HeaderRequestID) == id {
		t.Error("request ids should be unique per request")
	}
}

func TestRequestID_Reused(t *testing.T) {
	r := newEngine(RequestID())
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set(HeaderRequestID, "client-supplied-123")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get(HeaderRequestID); got != "client-supplied-123" {
		t.Errorf("header should reuse incoming id, got %q", got)
	}
	if w.Body.String() != "client-supplied-123" {
		t.Errorf("context should reuse incoming id, got %q", w.Body.String())
	}
}

func TestRequestID_RejectsUnsafe(t *testing.T) {
	r := newEngine(RequestID())
	for _, bad := range []string{"has space", "new\nline", "日本語", string(make([]byte, 200))} {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.Header.Set(HeaderRequestID, bad)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if got := w.Header().Get(HeaderRequestID); got == bad || len(got) != 32 {
			t.Errorf("unsafe id %q should be replaced, got %q", bad, got)
		}
	}
}

func TestCORS_AllowedOrigin(t *testing.T) {
	cfg := &config.Config{AllowedOrigins: []string{"https://nasuton.github.io"}, CORSMaxAge: time.Hour}
	r := newEngine(CORS(cfg, discard))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://nasuton.github.io")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://nasuton.github.io" {
		t.Errorf("Access-Control-Allow-Origin = %q", got)
	}
	if got := strings.ToLower(w.Header().Get("Access-Control-Expose-Headers")); !strings.Contains(got, "etag") ||
		!strings.Contains(got, strings.ToLower(HeaderRequestID)) {
		t.Errorf("Expose-Headers should include ETag and %s, got %q", HeaderRequestID, got)
	}

	// プリフライト
	pre := httptest.NewRequest(http.MethodOptions, "/ping", nil)
	pre.Header.Set("Origin", "https://nasuton.github.io")
	pre.Header.Set("Access-Control-Request-Method", http.MethodGet)
	pw := httptest.NewRecorder()
	r.ServeHTTP(pw, pre)
	if pw.Code != http.StatusNoContent {
		t.Errorf("preflight status = %d", pw.Code)
	}
	if pw.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Error("preflight should include Access-Control-Allow-Origin")
	}
}

func TestCORS_DisallowedOrigin(t *testing.T) {
	cfg := &config.Config{AllowedOrigins: []string{"https://nasuton.github.io"}, CORSMaxAge: time.Hour}
	r := newEngine(CORS(cfg, discard))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("disallowed origin must not receive Access-Control-Allow-Origin, got %q", got)
	}
	if w.Code == http.StatusOK && w.Body.Len() > 0 {
		// gin-contrib/cors は不許可オリジンを 403 で拒否する
		t.Errorf("disallowed origin should be rejected, got %d", w.Code)
	}
}

func TestCORS_EmptyOriginsDeniesAll(t *testing.T) {
	cfg := &config.Config{AllowedOrigins: nil}
	r := newEngine(CORS(cfg, discard)) // panic しないこと

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://nasuton.github.io")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("same-origin style request should still be served, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("no origin should be allowed when ALLOWED_ORIGINS is empty, got %q", got)
	}
}

func TestTimeout_SetsDeadline(t *testing.T) {
	r := gin.New()
	r.Use(Timeout(50 * time.Millisecond))
	r.GET("/ping", func(c *gin.Context) {
		dl, ok := c.Request.Context().Deadline()
		if !ok || time.Until(dl) > 50*time.Millisecond {
			c.String(http.StatusInternalServerError, "no deadline")
			return
		}
		c.String(http.StatusOK, "ok")
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ping", nil))
	if w.Code != http.StatusOK {
		t.Errorf("deadline not propagated: %s", w.Body.String())
	}
}

func TestRecovery_ReturnsUnifiedError(t *testing.T) {
	called := false
	write := func(c *gin.Context, status int, code, message string) {
		called = true
		c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
	}
	r := gin.New()
	r.Use(RequestID(), Recovery(discard, write))
	r.GET("/boom", func(c *gin.Context) { panic("boom") })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/boom", nil))
	if !called || w.Code != http.StatusInternalServerError {
		t.Errorf("recovery should write 500 via ErrorWriter, called=%v code=%d", called, w.Code)
	}
}
