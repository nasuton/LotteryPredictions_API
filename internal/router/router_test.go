package router

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"LotteryPredictions_API/internal/config"
	"LotteryPredictions_API/internal/middleware"
	"LotteryPredictions_API/internal/model"
	"LotteryPredictions_API/internal/repository"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type stubRepo struct{}

func (stubRepo) FindAll(context.Context, string, int, int) ([]model.LotteryPrediction, error) {
	return []model.LotteryPrediction{}, nil
}
func (stubRepo) FindByID(context.Context, int64) (*model.LotteryPrediction, error) {
	return &model.LotteryPrediction{ID: 1, LotteryType: "loto6", Pattern: "p", Numbers: "1,2,3"}, nil
}
func (stubRepo) Snapshot(context.Context, string) (repository.Snapshot, error) {
	return repository.Snapshot{}, nil
}
func (stubRepo) CountByType(context.Context) ([]model.TypeSummary, error) { return nil, nil }
func (stubRepo) LastBatchRuns(context.Context) ([]model.BatchRun, error)  { return nil, nil }

func (stubRepo) FindHitRates(context.Context, string, int, int) ([]model.LotteryHitRate, error) {
	return []model.LotteryHitRate{}, nil
}

func (stubRepo) CountHitRates(context.Context, string) (int64, error) { return 0, nil }

type stubPinger struct{}

func (stubPinger) PingContext(context.Context) error { return nil }

func newTestRouter(t *testing.T, basePath string, origins []string) *gin.Engine {
	t.Helper()
	cfg := &config.Config{
		BasePath:       basePath,
		AllowedOrigins: origins,
		CORSMaxAge:     time.Hour,
		DBQueryTimeout: time.Second,
	}
	r, err := New(Deps{
		Config:      cfg,
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Predictions: stubRepo{},
		Status:      stubRepo{},
		HitRates:    stubRepo{},
		Pinger:      stubPinger{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func get(r *gin.Engine, target string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
	return w
}

func TestRoutes(t *testing.T) {
	for _, base := range []string{"", "/lottery"} {
		r := newTestRouter(t, base, nil)
		paths := []string{
			"/health", "/healthz", "/readyz",
			"/api/predictions", "/api/predictions/1",
			"/api/v1/predictions", "/api/v1/predictions/1", "/api/v1/status",
			"/api/lottery_hit_rates", "/api/v1/lottery_hit_rates",
		}
		for _, p := range paths {
			w := get(r, base+p)
			if w.Code != http.StatusOK {
				t.Errorf("GET %s%s = %d body=%s", base, p, w.Code, w.Body.String())
			}
			if w.Header().Get(middleware.HeaderRequestID) == "" {
				t.Errorf("GET %s%s missing %s", base, p, middleware.HeaderRequestID)
			}
		}
		// 無印には /status を生やさない
		if w := get(r, base+"/api/status"); w.Code != http.StatusNotFound {
			t.Errorf("/api/status should be 404, got %d", w.Code)
		}
	}
}

func TestNoRoute_UnifiedError(t *testing.T) {
	r := newTestRouter(t, "", nil)
	w := get(r, "/nope")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status %d", w.Code)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("404 should be no-store")
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "not_found" || body.Error.RequestID != w.Header().Get(middleware.HeaderRequestID) {
		t.Errorf("unexpected 404 body %s", w.Body.String())
	}
}

func TestBasePathIsolation(t *testing.T) {
	r := newTestRouter(t, "/lottery", nil)
	if w := get(r, "/health"); w.Code != http.StatusNotFound {
		t.Errorf("routes outside BASE_PATH should 404, got %d", w.Code)
	}
}

func TestCORSWiring(t *testing.T) {
	r := newTestRouter(t, "", []string{"https://nasuton.github.io"})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/predictions", nil)
	req.Header.Set("Origin", "https://nasuton.github.io")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Header().Get("Access-Control-Allow-Origin") != "https://nasuton.github.io" {
		t.Errorf("CORS header missing: %v", w.Header())
	}
}

func TestInvalidTrustedProxies(t *testing.T) {
	cfg := &config.Config{TrustedProxies: []string{"not-an-ip"}}
	if _, err := New(Deps{Config: cfg, Predictions: stubRepo{}, Status: stubRepo{}, Pinger: stubPinger{}}); err == nil {
		t.Error("invalid TRUSTED_PROXIES should be rejected")
	}
}
