package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"LotteryPredictions_API/internal/middleware"
	"LotteryPredictions_API/internal/model"
	"LotteryPredictions_API/internal/repository"
)

type hitRateRepo struct {
	find     func(context.Context, string, int, int) ([]model.LotteryHitRate, error)
	total    int64
	countErr error
}

type hitRateContextKey struct{}

func hitRateRequest(r *gin.Engine, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req = req.WithContext(context.WithValue(req.Context(), hitRateContextKey{}, "request-context"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func (r hitRateRepo) FindHitRates(ctx context.Context, lotteryType string, limit, offset int) ([]model.LotteryHitRate, error) {
	return r.find(ctx, lotteryType, limit, offset)
}

func (r hitRateRepo) CountHitRates(context.Context, string) (int64, error) {
	return r.total, r.countErr
}

func hitRateRouter(repo repository.HitRateRepository) *gin.Engine {
	r := gin.New()
	r.Use(middleware.RequestID())
	h := NewHitRateHandler(repo, discard)
	for _, prefix := range []string{"/api", "/api/v1", "/lottery/api", "/lottery/api/v1"} {
		r.GET(prefix+"/lottery_hit_rates", h.List)
	}
	return r
}

func TestHitRatesList(t *testing.T) {
	zero := 0.0
	stamp := time.Date(2026, 10, 7, 12, 0, 0, 0, time.FixedZone("JST", 9*3600))
	for _, lotteryType := range []string{"", "loto6", "loto7", "miniloto"} {
		t.Run("type="+lotteryType, func(t *testing.T) {
			r := hitRateRouter(hitRateRepo{total: 1, find: func(ctx context.Context, gotType string, limit, offset int) ([]model.LotteryHitRate, error) {
				if gotType != lotteryType {
					t.Fatalf("lottery_type = %q want %q", gotType, lotteryType)
				}
				if ctx.Value(hitRateContextKey{}) != "request-context" {
					t.Fatal("request context was not passed to repository")
				}
				if limit != 20 || offset != 0 {
					t.Fatalf("limit=%d offset=%d", limit, offset)
				}
				return []model.LotteryHitRate{{
					LotteryType: "loto6", Pattern: "頻度", PredictionCount: 120,
					MeanHits: 1.25, HitRate: 12.5, Match3Rate: 10, Match4Rate: 2.5,
					Match5Rate: 0, Match6Rate: &zero, Match7Rate: nil,
					CreatedAt: stamp, UpdatedAt: stamp,
				}}, nil
			}})
			for _, prefix := range []string{"/api", "/api/v1", "/lottery/api/v1"} {
				target := prefix + "/lottery_hit_rates"
				if lotteryType != "" {
					// Leading/trailing whitespace is normalized like predictions.
					target += "?lottery_type=%20" + lotteryType + "%20"
				}
				w := hitRateRequest(r, target)
				if w.Code != http.StatusOK {
					t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
				}
				body := decode(t, w)
				data := body["data"].([]any)
				if body["total"] != float64(1) || len(data) != 1 {
					t.Fatalf("unexpected response %s", w.Body.String())
				}
				if body["limit"] != float64(20) || body["offset"] != float64(0) || body["next_url"] != nil {
					t.Fatalf("unexpected pagination %s", w.Body.String())
				}
				rate := data[0].(map[string]any)
				want := map[string]any{
					"lottery_type": "loto6", "pattern": "頻度", "prediction_count": float64(120),
					"mean_hits": 1.25, "hit_rate": 12.5, "match_3_rate": float64(10),
					"match_4_rate": 2.5, "match_5_rate": float64(0), "match_6_rate": float64(0),
					"match_7_rate": nil, "created_at": stamp.Format(time.RFC3339), "updated_at": stamp.Format(time.RFC3339),
				}
				if len(rate) != len(want) {
					t.Fatalf("unexpected fields %v", rate)
				}
				for key, value := range want {
					if got, ok := rate[key]; !ok || got != value {
						t.Errorf("%s = %v want %v", key, got, value)
					}
				}
				if w.Header().Get("Cache-Control") != "public, max-age=300" {
					t.Errorf("unexpected Cache-Control %q", w.Header().Get("Cache-Control"))
				}
			}
		})
	}
}

func TestHitRatesEmpty(t *testing.T) {
	r := hitRateRouter(hitRateRepo{find: func(context.Context, string, int, int) ([]model.LotteryHitRate, error) {
		return nil, nil
	}})
	w := do(r, "/api/v1/lottery_hit_rates", nil)
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"data":[],"limit":20,"next_url":null,"offset":0,"total":0}` {
		t.Fatalf("empty response status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestHitRatesInvalidType(t *testing.T) {
	r := hitRateRouter(hitRateRepo{find: func(context.Context, string, int, int) ([]model.LotteryHitRate, error) {
		t.Fatal("invalid type must not query the database")
		return nil, nil
	}})
	for _, lotteryType := range []string{"numbers3", "numbers4", "unknown", "LOTO6"} {
		w := do(r, "/api/v1/lottery_hit_rates?lottery_type="+lotteryType, nil)
		assertError(t, w, http.StatusBadRequest, "invalid_lottery_type")
	}
}

func TestHitRatesDBError(t *testing.T) {
	for _, err := range []error{errors.New("private DB error"), repository.ErrTableNotFound, context.DeadlineExceeded} {
		for _, op := range []string{"count", "find"} {
			repo := hitRateRepo{find: func(context.Context, string, int, int) ([]model.LotteryHitRate, error) {
				return nil, err
			}}
			if op == "count" {
				repo.countErr = err
				repo.find = func(context.Context, string, int, int) ([]model.LotteryHitRate, error) {
					t.Fatal("must not fetch a page after count error")
					return nil, nil
				}
			}
			w := do(hitRateRouter(repo), "/api/v1/lottery_hit_rates", nil)
			assertError(t, w, http.StatusInternalServerError, "internal_error")
			if strings.Contains(w.Body.String(), err.Error()) {
				t.Errorf("response exposes internal error: %s", w.Body.String())
			}
		}
	}
}

func pagedHitRateRepo(total int64) hitRateRepo {
	return hitRateRepo{total: total, find: func(_ context.Context, _ string, limit, offset int) ([]model.LotteryHitRate, error) {
		rates := []model.LotteryHitRate{}
		for i := int64(offset); i < total && len(rates) < limit; i++ {
			rates = append(rates, model.LotteryHitRate{LotteryType: "loto6", Pattern: fmt.Sprintf("p%03d", i)})
		}
		return rates, nil
	}}
}

func TestHitRatesPaginationParameters(t *testing.T) {
	for _, tc := range []struct {
		query         string
		limit, offset int
	}{
		{"", 20, 0},
		{"?limit=112", 100, 0},
		{"?limit=100&offset=100", 100, 100},
		{"?limit=5&offset=10", 5, 10},
		{"?limit=0&offset=-1", 20, 0},
		{"?limit=-1&offset=invalid", 20, 0},
		{"?limit=invalid", 20, 0},
		{"?limit=999999999999999999999&offset=999999999999999999999", 20, 0},
	} {
		t.Run(tc.query, func(t *testing.T) {
			repo := pagedHitRateRepo(250)
			find := repo.find
			repo.find = func(ctx context.Context, lt string, limit, offset int) ([]model.LotteryHitRate, error) {
				if limit != tc.limit || offset != tc.offset {
					t.Fatalf("repository limit=%d offset=%d want=%d,%d", limit, offset, tc.limit, tc.offset)
				}
				return find(ctx, lt, limit, offset)
			}
			w := do(hitRateRouter(repo), "/api/v1/lottery_hit_rates"+tc.query, nil)
			if w.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			body := decode(t, w)
			if body["total"] != float64(250) || body["limit"] != float64(tc.limit) || body["offset"] != float64(tc.offset) || len(body["data"].([]any)) != tc.limit {
				t.Fatalf("unexpected page: %s", w.Body.String())
			}
		})
	}
}

func TestHitRatesFollowNextURL(t *testing.T) {
	for _, prefix := range []string{"/api", "/api/v1", "/lottery/api", "/lottery/api/v1"} {
		t.Run(prefix, func(t *testing.T) {
			r := hitRateRouter(pagedHitRateRepo(112))
			target := prefix + "/lottery_hit_rates?lottery_type=loto6&limit=112&extra=keep"
			seen := map[string]bool{}
			for page := 0; page < 2; page++ {
				w := do(r, target, nil)
				if w.Code != http.StatusOK {
					t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
				}
				body := decode(t, w)
				data := body["data"].([]any)
				wantSize := 100
				if page == 1 {
					wantSize = 12
				}
				if len(data) != wantSize || body["total"] != float64(112) || body["limit"] != float64(100) || body["offset"] != float64(page*100) {
					t.Fatalf("unexpected page: %s", w.Body.String())
				}
				for _, row := range data {
					pattern := row.(map[string]any)["pattern"].(string)
					if seen[pattern] {
						t.Fatalf("duplicate pattern %s", pattern)
					}
					seen[pattern] = true
				}
				if page == 1 {
					if body["next_url"] != nil {
						t.Fatalf("last page must have null next_url")
					}
					break
				}
				target = body["next_url"].(string)
				next, err := url.Parse(target)
				if err != nil {
					t.Fatal(err)
				}
				q := next.Query()
				if next.IsAbs() || next.Path != prefix+"/lottery_hit_rates" || q.Get("lottery_type") != "loto6" || q.Get("limit") != "100" || q.Get("offset") != "100" || q.Get("extra") != "keep" {
					t.Fatalf("next URL lost path/filter/normalized pagination: %s", target)
				}
			}
			if len(seen) != 112 {
				t.Fatalf("retrieved %d patterns want 112", len(seen))
			}
		})
	}
}

func TestHitRatesLastAndOutOfRangePages(t *testing.T) {
	for _, tc := range []struct {
		total            int64
		offset, wantSize int
	}{
		{0, 0, 0}, {100, 0, 100}, {112, 100, 12}, {112, 112, 0}, {112, 200, 0},
	} {
		w := do(hitRateRouter(pagedHitRateRepo(tc.total)), fmt.Sprintf("/api/v1/lottery_hit_rates?limit=100&offset=%d", tc.offset), nil)
		if w.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		body := decode(t, w)
		if body["total"] != float64(tc.total) || len(body["data"].([]any)) != tc.wantSize || body["next_url"] != nil {
			t.Fatalf("unexpected last/out-of-range page: %s", w.Body.String())
		}
	}
}
