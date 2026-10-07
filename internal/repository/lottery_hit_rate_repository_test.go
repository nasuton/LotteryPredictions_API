package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

// hitRateConnector exercises database/sql's real DECIMAL and NULL scanning
// without requiring a live MySQL instance or an additional test dependency.
type hitRateConnector struct {
	query func(context.Context, string, []driver.NamedValue) (driver.Rows, error)
}

func (c *hitRateConnector) Connect(context.Context) (driver.Conn, error) { return c, nil }
func (c *hitRateConnector) Driver() driver.Driver                        { return c }
func (c *hitRateConnector) Open(string) (driver.Conn, error)             { return c, nil }
func (c *hitRateConnector) Prepare(string) (driver.Stmt, error)          { return nil, driver.ErrSkip }
func (c *hitRateConnector) Close() error                                 { return nil }
func (c *hitRateConnector) Begin() (driver.Tx, error)                    { return nil, driver.ErrSkip }
func (c *hitRateConnector) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.query(ctx, query, args)
}

type hitRateRows struct {
	columns []string
	values  [][]driver.Value
	nextErr error
	closed  bool
}

func (r *hitRateRows) Columns() []string {
	if r.columns != nil {
		return r.columns
	}
	return []string{"lottery_type", "pattern", "prediction_count", "mean_hits", "hit_rate",
		"match_3_rate", "match_4_rate", "match_5_rate", "match_6_rate", "match_7_rate", "created_at", "updated_at"}
}
func (r *hitRateRows) Close() error { r.closed = true; return nil }
func (r *hitRateRows) Next(dest []driver.Value) error {
	if len(r.values) == 0 {
		if r.nextErr != nil {
			return r.nextErr
		}
		return io.EOF
	}
	copy(dest, r.values[0])
	r.values = r.values[1:]
	return nil
}

func hitRateTestRepository(t *testing.T, query func(context.Context, string, []driver.NamedValue) (driver.Rows, error)) *LotteryRepository {
	t.Helper()
	db := sql.OpenDB(&hitRateConnector{query: query})
	t.Cleanup(func() { db.Close() })
	return NewLotteryRepository(db)
}

func TestFindHitRates(t *testing.T) {
	stamp := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	for _, lotteryType := range []string{"", "loto6", "loto7", "miniloto", "loto6' OR 1=1 --"} {
		t.Run("type="+lotteryType, func(t *testing.T) {
			rows := &hitRateRows{values: [][]driver.Value{
				{"loto6", "頻度", int64(4294967295), []byte("1.25"), []byte("12.50"),
					[]byte("10.00"), []byte("2.50"), []byte("0.00"), []byte("0.00"), nil, stamp, stamp},
				{"loto7", "random", int64(20), []byte("1.25"), []byte("12.50"),
					[]byte("10.00"), []byte("2.50"), []byte("0.00"), []byte("1.50"), []byte("0.00"), stamp, stamp},
				{"miniloto", "random", int64(20), []byte("1.25"), []byte("12.50"),
					[]byte("10.00"), []byte("2.50"), []byte("0.00"), nil, nil, stamp, stamp},
			}}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			repo := hitRateTestRepository(t, func(gotCtx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
				if gotCtx != ctx {
					t.Fatal("request context was not used")
				}
				if !strings.Contains(query, "FROM lottery_hit_rates") || !strings.HasSuffix(query, " ORDER BY lottery_type ASC, pattern ASC LIMIT ? OFFSET ?") {
					t.Fatalf("unexpected query %s", query)
				}
				wantArgs := []driver.NamedValue{{Ordinal: 1, Value: int64(20)}, {Ordinal: 2, Value: int64(5)}}
				if lotteryType == "" {
					if strings.Contains(query, "WHERE") {
						t.Fatalf("unfiltered query %s args=%v", query, args)
					}
				} else {
					if !strings.Contains(query, " WHERE lottery_type = ?") {
						t.Fatalf("filter must use bound parameter: %s", query)
					}
					wantArgs = []driver.NamedValue{{Ordinal: 1, Value: lotteryType}, {Ordinal: 2, Value: int64(20)}, {Ordinal: 3, Value: int64(5)}}
				}
				if !reflect.DeepEqual(args, wantArgs) {
					t.Fatalf("args=%v want=%v", args, wantArgs)
				}
				return rows, nil
			})
			rates, err := repo.FindHitRates(ctx, lotteryType, 20, 5)
			if err != nil {
				t.Fatal(err)
			}
			if len(rates) != 3 || rates[0].PredictionCount != 4294967295 || rates[0].MeanHits != 1.25 || rates[0].HitRate != 12.5 || rates[0].Match3Rate != 10 || rates[0].Match4Rate != 2.5 || rates[0].Match5Rate != 0 {
				t.Fatalf("incorrect decimal/count scan: %+v", rates)
			}
			if rates[0].Match6Rate == nil || *rates[0].Match6Rate != 0 || rates[0].Match7Rate != nil || rates[1].Match6Rate == nil || *rates[1].Match6Rate != 1.5 || rates[1].Match7Rate == nil || *rates[1].Match7Rate != 0 || rates[2].Match6Rate != nil || rates[2].Match7Rate != nil {
				t.Fatalf("incorrect nullable rate scan: %+v", rates)
			}
			if rates[0].Pattern != "頻度" || rates[0].LotteryType != "loto6" || !rates[0].CreatedAt.Equal(stamp) || !rates[0].UpdatedAt.Equal(stamp) || !rows.closed {
				t.Fatalf("incorrect text/time scan or rows not closed: %+v", rates[0])
			}
		})
	}
}

func TestFindHitRatesEmpty(t *testing.T) {
	repo := hitRateTestRepository(t, func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
		return &hitRateRows{}, nil
	})
	rates, err := repo.FindHitRates(context.Background(), "", 20, 0)
	if err != nil || rates == nil || len(rates) != 0 {
		t.Fatalf("rates=%v err=%v", rates, err)
	}
}

func TestFindHitRatesErrors(t *testing.T) {
	rowErr := errors.New("row iteration failed")
	for _, tc := range []struct {
		name     string
		queryErr error
		rows     *hitRateRows
		wantErr  error
	}{
		{name: "missing table", queryErr: &mysql.MySQLError{Number: 1146, Message: "missing"}, wantErr: ErrTableNotFound},
		{name: "query deadline", queryErr: context.DeadlineExceeded, wantErr: context.DeadlineExceeded},
		{name: "rows", rows: &hitRateRows{nextErr: rowErr}, wantErr: rowErr},
		{name: "scan", rows: &hitRateRows{values: [][]driver.Value{{"loto6", "bad", int64(1), []byte("invalid decimal"), nil, nil, nil, nil, nil, nil, nil, nil}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := hitRateTestRepository(t, func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
				if tc.queryErr != nil {
					return nil, tc.queryErr
				}
				return tc.rows, nil
			})
			rates, err := repo.FindHitRates(context.Background(), "", 20, 0)
			if err == nil || rates != nil || (tc.wantErr != nil && !errors.Is(err, tc.wantErr)) {
				t.Fatalf("rates=%v err=%v want=%v", rates, err, tc.wantErr)
			}
			if tc.rows != nil && !tc.rows.closed {
				t.Error("rows were not closed on error")
			}
		})
	}
}

func TestCountHitRates(t *testing.T) {
	for _, lotteryType := range []string{"", "loto6", "loto7", "miniloto", "loto6' OR 1=1 --"} {
		for _, wantTotal := range []int64{0, 112} {
			ctx := context.Background()
			repo := hitRateTestRepository(t, func(gotCtx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
				wantQuery := "SELECT COUNT(*) FROM lottery_hit_rates"
				var wantArgs []driver.NamedValue
				if lotteryType != "" {
					wantQuery += " WHERE lottery_type = ?"
					wantArgs = []driver.NamedValue{{Ordinal: 1, Value: lotteryType}}
				}
				if gotCtx != ctx || query != wantQuery || len(args) != len(wantArgs) || (len(args) > 0 && !reflect.DeepEqual(args, wantArgs)) {
					t.Fatalf("unexpected count query: %s args=%v", query, args)
				}
				return &hitRateRows{columns: []string{"count"}, values: [][]driver.Value{{wantTotal}}}, nil
			})
			total, err := repo.CountHitRates(ctx, lotteryType)
			if err != nil || total != wantTotal {
				t.Fatalf("total=%d err=%v want=%d", total, err, wantTotal)
			}
		}
	}
}

func TestCountHitRatesErrors(t *testing.T) {
	for _, tc := range []struct {
		queryErr error
		value    driver.Value
		wantErr  error
	}{
		{queryErr: &mysql.MySQLError{Number: 1146}, wantErr: ErrTableNotFound},
		{queryErr: context.DeadlineExceeded, wantErr: context.DeadlineExceeded},
		{value: "invalid count"},
	} {
		repo := hitRateTestRepository(t, func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
			if tc.queryErr != nil {
				return nil, tc.queryErr
			}
			return &hitRateRows{columns: []string{"count"}, values: [][]driver.Value{{tc.value}}}, nil
		})
		total, err := repo.CountHitRates(context.Background(), "loto6")
		if total != 0 || err == nil || (tc.wantErr != nil && !errors.Is(err, tc.wantErr)) {
			t.Fatalf("total=%d err=%v want=%v", total, err, tc.wantErr)
		}
	}
}
