package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"LotteryPredictions_API/internal/middleware"
	"LotteryPredictions_API/internal/model"
	"LotteryPredictions_API/internal/repository"
)

func init() {
	gin.SetMode(gin.TestMode)
}

var discard = slog.New(slog.NewTextHandler(io.Discard, nil))

// mockRepo は PredictionRepository / StatusRepository のテストダブル
type mockRepo struct {
	findAll       func(ctx context.Context, lotteryType string, limit, offset int) ([]model.LotteryPrediction, error)
	findByID      func(ctx context.Context, id int64) (*model.LotteryPrediction, error)
	snapshot      func(ctx context.Context, lotteryType string) (repository.Snapshot, error)
	countByType   func(ctx context.Context) ([]model.TypeSummary, error)
	lastBatchRuns func(ctx context.Context) ([]model.BatchRun, error)

	findAllCalls int
}

func (m *mockRepo) FindAll(ctx context.Context, lotteryType string, limit, offset int) ([]model.LotteryPrediction, error) {
	m.findAllCalls++
	return m.findAll(ctx, lotteryType, limit, offset)
}
func (m *mockRepo) FindByID(ctx context.Context, id int64) (*model.LotteryPrediction, error) {
	return m.findByID(ctx, id)
}
func (m *mockRepo) Snapshot(ctx context.Context, lotteryType string) (repository.Snapshot, error) {
	return m.snapshot(ctx, lotteryType)
}
func (m *mockRepo) CountByType(ctx context.Context) ([]model.TypeSummary, error) {
	return m.countByType(ctx)
}
func (m *mockRepo) LastBatchRuns(ctx context.Context) ([]model.BatchRun, error) {
	return m.lastBatchRuns(ctx)
}

func date(s string) model.Date {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return model.Date(t)
}

func makePredictions(n int) []model.LotteryPrediction {
	list := make([]model.LotteryPrediction, 0, n)
	for i := 0; i < n; i++ {
		list = append(list, model.LotteryPrediction{
			ID: int64(1000 - i), LotteryType: "loto6", Pattern: fmt.Sprintf("p%d", i),
			PredictedAt: date("2026-09-28"), Numbers: "1,2,3,4,5,6",
		})
	}
	return list
}

// healthyRepo は total 件のデータを持つ正常系の mock を返す
func healthyRepo(total int) *mockRepo {
	return &mockRepo{
		snapshot: func(_ context.Context, _ string) (repository.Snapshot, error) {
			return repository.Snapshot{Count: int64(total), MaxID: 1000, MaxPredictedAt: date("2026-09-28")}, nil
		},
		findAll: func(_ context.Context, _ string, limit, offset int) ([]model.LotteryPrediction, error) {
			remain := total - offset
			if remain <= 0 {
				return []model.LotteryPrediction{}, nil
			}
			if remain > limit {
				remain = limit
			}
			return makePredictions(remain), nil
		},
		findByID: func(_ context.Context, id int64) (*model.LotteryPrediction, error) {
			if id != 1 {
				return nil, sql.ErrNoRows
			}
			return &model.LotteryPrediction{ID: 1, LotteryType: "loto7", Pattern: "x", PredictedAt: date("2026-09-27"), Numbers: "1,2,3,4,5,6,7"}, nil
		},
	}
}

// newRouter は v1 と無印の両方に handler を登録した engine を返す
func newRouter(repo *mockRepo) *gin.Engine {
	r := gin.New()
	r.Use(middleware.RequestID())
	h := NewLotteryHandler(repo, discard)
	s := NewStatusHandler(repo, discard)
	s.now = func() time.Time { return time.Date(2026, 9, 28, 12, 0, 0, 0, time.FixedZone("JST", 9*3600)) }
	for _, prefix := range []string{"/api", "/api/v1", "/lottery/api/v1"} {
		g := r.Group(prefix)
		g.GET("/predictions", h.List)
		g.GET("/predictions/:id", h.Get)
		g.GET("/status", s.Status)
	}
	return r
}

func do(r *gin.Engine, target string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decode(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, w.Body.String())
	}
	return body
}

func assertError(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("status = %d want %d body=%s", w.Code, status, w.Body.String())
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("error Cache-Control = %q want no-store", cc)
	}
	if w.Header().Get("ETag") != "" {
		t.Errorf("error response must not carry ETag")
	}
	body := decode(t, w)
	e, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("error should be an object: %s", w.Body.String())
	}
	if e["code"] != code {
		t.Errorf("error.code = %v want %s", e["code"], code)
	}
	if msg, _ := e["message"].(string); msg == "" {
		t.Error("error.message should not be empty")
	}
	if rid, _ := e["request_id"].(string); rid == "" || rid != w.Header().Get(middleware.HeaderRequestID) {
		t.Errorf("error.request_id = %v should match header %q", e["request_id"], w.Header().Get(middleware.HeaderRequestID))
	}
}

func TestList_DefaultsAndLimitClamp(t *testing.T) {
	repo := healthyRepo(250)
	r := newRouter(repo)

	tests := []struct {
		query     string
		wantLimit float64
		wantOff   float64
	}{
		{"", 20, 0},
		{"?limit=0", 20, 0},
		{"?limit=-3", 20, 0},
		{"?limit=abc", 20, 0},
		{"?limit=500", 100, 0},
		{"?limit=50&offset=10", 50, 10},
		{"?offset=-1", 20, 0},
		{"?offset=x", 20, 0},
	}
	for _, tt := range tests {
		w := do(r, "/api/v1/predictions"+tt.query, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: status %d", tt.query, w.Code)
		}
		body := decode(t, w)
		if body["limit"] != tt.wantLimit || body["offset"] != tt.wantOff {
			t.Errorf("%s: limit=%v offset=%v want %v/%v", tt.query, body["limit"], body["offset"], tt.wantLimit, tt.wantOff)
		}
		if body["total"] != float64(250) {
			t.Errorf("%s: total=%v", tt.query, body["total"])
		}
		if data, ok := body["data"].([]any); !ok || len(data) != int(tt.wantLimit) {
			t.Errorf("%s: data length=%d want %v", tt.query, len(data), tt.wantLimit)
		}
	}
}

func TestList_NextURLFollowsCalledPath(t *testing.T) {
	r := newRouter(healthyRepo(112))

	tests := []struct {
		target string
		want   any
	}{
		{"/api/predictions?limit=112", "/api/predictions?limit=100&offset=100"},
		{"/api/v1/predictions?limit=112", "/api/v1/predictions?limit=100&offset=100"},
		{"/lottery/api/v1/predictions?lottery_type=loto6&limit=50", "/lottery/api/v1/predictions?limit=50&lottery_type=loto6&offset=50"},
		{"/api/v1/predictions?limit=100&offset=100", nil},
		{"/api/v1/predictions?limit=100&offset=12", nil},
		{"/api/v1/predictions?offset=500", nil},
	}
	for _, tt := range tests {
		w := do(r, tt.target, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: status %d", tt.target, w.Code)
		}
		body := decode(t, w)
		if body["next_url"] != tt.want {
			t.Errorf("%s: next_url=%v want %v", tt.target, body["next_url"], tt.want)
		}
	}
}

func TestList_ResponseShapeAndHeaders(t *testing.T) {
	r := newRouter(healthyRepo(3))
	w := do(r, "/api/predictions", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=300, stale-while-revalidate=600" {
		t.Errorf("Cache-Control = %q", cc)
	}
	etag := w.Header().Get("ETag")
	if !strings.HasPrefix(etag, `W/"`) || !strings.HasSuffix(etag, `"`) {
		t.Errorf("ETag should be weak, got %q", etag)
	}
	body := decode(t, w)
	for _, key := range []string{"data", "total", "limit", "offset", "next_url"} {
		if _, ok := body[key]; !ok {
			t.Errorf("response missing key %q", key)
		}
	}
	first := body["data"].([]any)[0].(map[string]any)
	for _, key := range []string{"id", "lottery_type", "pattern", "predicted_at", "numbers", "numbers_raw"} {
		if _, ok := first[key]; !ok {
			t.Errorf("prediction missing key %q", key)
		}
	}
	if nums, ok := first["numbers"].([]any); !ok || len(nums) != 6 {
		t.Errorf("numbers should be an array of 6, got %v", first["numbers"])
	}
}

func TestList_ETag304(t *testing.T) {
	repo := healthyRepo(30)
	r := newRouter(repo)

	w1 := do(r, "/api/v1/predictions?lottery_type=loto6", nil)
	etag := w1.Header().Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag")
	}

	w2 := do(r, "/api/v1/predictions?lottery_type=loto6", map[string]string{"If-None-Match": etag})
	if w2.Code != http.StatusNotModified {
		t.Fatalf("status = %d want 304", w2.Code)
	}
	if w2.Body.Len() != 0 {
		t.Errorf("304 body should be empty")
	}
	if w2.Header().Get("ETag") != etag || w2.Header().Get("Cache-Control") == "" {
		t.Errorf("304 should carry ETag and Cache-Control")
	}
	if repo.findAllCalls != 1 {
		t.Errorf("FindAll should be skipped on 304, calls=%d", repo.findAllCalls)
	}

	// 弱い比較: W/ 無しでも一致
	w3 := do(r, "/api/v1/predictions?lottery_type=loto6", map[string]string{"If-None-Match": strings.TrimPrefix(etag, "W/")})
	if w3.Code != http.StatusNotModified {
		t.Errorf("weak comparison should match, got %d", w3.Code)
	}

	// 別の条件では別 ETag
	w4 := do(r, "/api/v1/predictions?lottery_type=loto7", map[string]string{"If-None-Match": etag})
	if w4.Code != http.StatusOK {
		t.Errorf("different query should not match, got %d", w4.Code)
	}

	// DB の状態が変われば ETag が変わる
	repo.snapshot = func(_ context.Context, _ string) (repository.Snapshot, error) {
		return repository.Snapshot{Count: 31, MaxID: 1001, MaxPredictedAt: date("2026-09-29")}, nil
	}
	w5 := do(r, "/api/v1/predictions?lottery_type=loto6", map[string]string{"If-None-Match": etag})
	if w5.Code != http.StatusOK || w5.Header().Get("ETag") == etag {
		t.Errorf("changed snapshot should invalidate ETag: %d %s", w5.Code, w5.Header().Get("ETag"))
	}
}

func TestList_InvalidLotteryType(t *testing.T) {
	r := newRouter(healthyRepo(1))
	for _, bad := range []string{"loto5", "LOTO6", "loto6x", "bingo5", "%27%20OR%201%3D1"} {
		w := do(r, "/api/v1/predictions?lottery_type="+bad, nil)
		assertError(t, w, http.StatusBadRequest, "invalid_lottery_type")
	}
	for _, good := range model.LotteryTypes {
		w := do(r, "/api/v1/predictions?lottery_type="+good, nil)
		if w.Code != http.StatusOK {
			t.Errorf("%s should be accepted, got %d", good, w.Code)
		}
	}
}

func TestList_RepositoryError(t *testing.T) {
	repo := healthyRepo(1)
	repo.snapshot = func(context.Context, string) (repository.Snapshot, error) {
		return repository.Snapshot{}, errors.New("secret internal detail")
	}
	w := do(newRouter(repo), "/api/v1/predictions", nil)
	assertError(t, w, http.StatusInternalServerError, "internal_error")
	if strings.Contains(w.Body.String(), "secret internal detail") {
		t.Error("internal error detail leaked to client")
	}

	repo = healthyRepo(1)
	repo.findAll = func(context.Context, string, int, int) ([]model.LotteryPrediction, error) {
		return nil, errors.New("boom")
	}
	w = do(newRouter(repo), "/api/v1/predictions", nil)
	assertError(t, w, http.StatusInternalServerError, "internal_error")
}

func TestGet(t *testing.T) {
	r := newRouter(healthyRepo(1))

	w := do(r, "/api/v1/predictions/1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=300, stale-while-revalidate=600" {
		t.Errorf("Cache-Control = %q", cc)
	}
	etag := w.Header().Get("ETag")
	if !strings.HasPrefix(etag, `W/"`) {
		t.Errorf("ETag = %q", etag)
	}
	data := decode(t, w)["data"].(map[string]any)
	if data["id"] != float64(1) || data["lottery_type"] != "loto7" {
		t.Errorf("unexpected data %v", data)
	}

	w304 := do(r, "/api/v1/predictions/1", map[string]string{"If-None-Match": etag})
	if w304.Code != http.StatusNotModified {
		t.Errorf("304 expected, got %d", w304.Code)
	}

	assertError(t, do(r, "/api/v1/predictions/0", nil), http.StatusBadRequest, "invalid_id")
	assertError(t, do(r, "/api/v1/predictions/abc", nil), http.StatusBadRequest, "invalid_id")
	assertError(t, do(r, "/api/v1/predictions/-5", nil), http.StatusBadRequest, "invalid_id")
	assertError(t, do(r, "/api/v1/predictions/999", nil), http.StatusNotFound, "not_found")
	assertError(t, do(r, "/api/predictions/999", nil), http.StatusNotFound, "not_found")

	repo := healthyRepo(1)
	repo.findByID = func(context.Context, int64) (*model.LotteryPrediction, error) { return nil, errors.New("db down") }
	assertError(t, do(newRouter(repo), "/api/v1/predictions/1", nil), http.StatusInternalServerError, "internal_error")
}

func TestStatus(t *testing.T) {
	jst := time.FixedZone("JST", 9*3600)
	repo := healthyRepo(0)
	repo.countByType = func(context.Context) ([]model.TypeSummary, error) {
		return []model.TypeSummary{
			{LotteryType: "loto6", Count: 43, LatestPredictedAt: date("2026-09-28")},
			{LotteryType: "loto7", Count: 12, LatestPredictedAt: date("2026-09-26")},
		}, nil
	}
	repo.lastBatchRuns = func(context.Context) ([]model.BatchRun, error) {
		return []model.BatchRun{{
			BatchName: "registration", LotteryType: "loto6", Status: "success",
			StartedAt:    time.Date(2026, 9, 28, 3, 0, 0, 0, jst),
			FinishedAt:   time.Date(2026, 9, 28, 3, 5, 12, 0, jst),
			RowsAffected: 43,
		}}, nil
	}

	w := do(newRouter(repo), "/api/v1/status", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=300" {
		t.Errorf("Cache-Control = %q", cc)
	}
	data := decode(t, w)["data"].(map[string]any)
	preds := data["predictions"].(map[string]any)
	if preds["total"] != float64(55) {
		t.Errorf("total = %v want 55", preds["total"])
	}
	byType := preds["by_type"].([]any)
	if len(byType) != 2 || byType[0].(map[string]any)["latest_predicted_at"] != "2026-09-28" {
		t.Errorf("by_type = %v", byType)
	}
	runs := data["last_batch_runs"].([]any)
	run := runs[0].(map[string]any)
	if run["batch_name"] != "registration" || run["status"] != "success" || run["rows_affected"] != float64(43) ||
		run["finished_at"] != "2026-09-28T03:05:12+09:00" {
		t.Errorf("last_batch_runs[0] = %v", run)
	}
	if data["generated_at"] != "2026-09-28T12:00:00+09:00" {
		t.Errorf("generated_at = %v", data["generated_at"])
	}
}

func TestStatus_BatchRunsTableMissing(t *testing.T) {
	repo := healthyRepo(0)
	repo.countByType = func(context.Context) ([]model.TypeSummary, error) { return nil, nil }
	repo.lastBatchRuns = func(context.Context) ([]model.BatchRun, error) {
		return nil, fmt.Errorf("%w: Error 1146: Table 'lottery.batch_runs' doesn't exist", repository.ErrTableNotFound)
	}

	w := do(newRouter(repo), "/api/v1/status", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	data := decode(t, w)["data"].(map[string]any)
	if v, ok := data["last_batch_runs"]; !ok || v != nil {
		t.Errorf("last_batch_runs should be null, got %v", v)
	}
	preds := data["predictions"].(map[string]any)
	if preds["total"] != float64(0) {
		t.Errorf("total = %v", preds["total"])
	}
	if bt, ok := preds["by_type"].([]any); !ok || len(bt) != 0 {
		t.Errorf("by_type should be an empty array, got %v", preds["by_type"])
	}
}

func TestStatus_Errors(t *testing.T) {
	repo := healthyRepo(0)
	repo.countByType = func(context.Context) ([]model.TypeSummary, error) { return nil, errors.New("db down") }
	repo.lastBatchRuns = func(context.Context) ([]model.BatchRun, error) { return nil, nil }
	assertError(t, do(newRouter(repo), "/api/v1/status", nil), http.StatusInternalServerError, "internal_error")

	repo.countByType = func(context.Context) ([]model.TypeSummary, error) { return nil, nil }
	repo.lastBatchRuns = func(context.Context) ([]model.BatchRun, error) { return nil, errors.New("other error") }
	assertError(t, do(newRouter(repo), "/api/v1/status", nil), http.StatusInternalServerError, "internal_error")
}

// pinger は Pinger のテストダブル
type pinger struct{ err error }

func (p pinger) PingContext(context.Context) error { return p.err }

func TestHealth(t *testing.T) {
	r := gin.New()
	ok := NewHealthHandler(pinger{}, time.Second, discard)
	ng := NewHealthHandler(pinger{err: errors.New("down")}, time.Second, discard)
	r.GET("/healthz", ng.Liveness) // DB が死んでいても liveness は 200
	r.GET("/readyz", ok.Readiness)
	r.GET("/health", ok.Readiness)
	r.GET("/ng/readyz", ng.Readiness)

	for _, p := range []string{"/healthz", "/readyz", "/health"} {
		w := do(r, p, nil)
		if w.Code != http.StatusOK || decode(t, w)["status"] != "ok" {
			t.Errorf("%s: %d %s", p, w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s: health endpoints must be no-store", p)
		}
	}
	w := do(r, "/ng/readyz", nil)
	if w.Code != http.StatusServiceUnavailable || decode(t, w)["status"] != "ng" {
		t.Errorf("readyz with failing DB: %d %s", w.Code, w.Body.String())
	}
}
