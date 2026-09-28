package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"

	"LotteryPredictions_API/internal/model"
)

// mysqlErrNoSuchTable は MySQL の ER_NO_SUCH_TABLE
const mysqlErrNoSuchTable = 1146

// selectColumns は lottery_predictions の取得対象カラム
const selectColumns = `id, lottery_type, pattern, predicted_at, numbers`

// buildWhere は lottery_type の絞り込み条件を組み立てる。日付では制限しない。
func buildWhere(lotteryType string) (string, []any) {
	if lotteryType == "" {
		return "", nil
	}

	return ` WHERE lottery_type = ?`, []any{lotteryType}
}

// wrapErr は MySQL 固有エラーを判別可能なセンチネルに包む
func wrapErr(err error) error {
	var me *mysql.MySQLError
	if errors.As(err, &me) && me.Number == mysqlErrNoSuchTable {
		return fmt.Errorf("%w: %v", ErrTableNotFound, err)
	}
	return err
}

// LotteryRepository は lottery_predictions / batch_runs テーブルへのアクセスを担当する。
// PredictionRepository と StatusRepository の両方を実装する。
type LotteryRepository struct {
	db *sql.DB
}

var (
	_ PredictionRepository = (*LotteryRepository)(nil)
	_ StatusRepository     = (*LotteryRepository)(nil)
)

func NewLotteryRepository(db *sql.DB) *LotteryRepository {
	return &LotteryRepository{db: db}
}

// FindAll は複数レコードを取得する。lotteryType が空文字でない場合は絞り込む
func (r *LotteryRepository) FindAll(ctx context.Context, lotteryType string, limit, offset int) ([]model.LotteryPrediction, error) {
	query := `SELECT ` + selectColumns + ` FROM lottery_predictions`
	where, args := buildWhere(lotteryType)
	query += where
	query += ` ORDER BY predicted_at DESC, id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, wrapErr(err)
	}
	defer rows.Close()

	predictions := make([]model.LotteryPrediction, 0, limit)
	for rows.Next() {
		var p model.LotteryPrediction
		if err := rows.Scan(&p.ID, &p.LotteryType, &p.Pattern, &p.PredictedAt, &p.Numbers); err != nil {
			return nil, err
		}
		predictions = append(predictions, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return predictions, nil
}

// FindByID は1件のレコードを取得する。存在しない場合は sql.ErrNoRows を返す
func (r *LotteryRepository) FindByID(ctx context.Context, id int64) (*model.LotteryPrediction, error) {
	const query = `SELECT ` + selectColumns + ` FROM lottery_predictions WHERE id = ?`

	var p model.LotteryPrediction
	err := r.db.QueryRowContext(ctx, query, id).
		Scan(&p.ID, &p.LotteryType, &p.Pattern, &p.PredictedAt, &p.Numbers)
	if err != nil {
		return nil, wrapErr(err)
	}
	return &p, nil
}

// Snapshot は条件に一致するレコードの件数・最大 ID・最新予測日を 1 クエリで返す
func (r *LotteryRepository) Snapshot(ctx context.Context, lotteryType string) (Snapshot, error) {
	query := `SELECT COUNT(*), COALESCE(MAX(id), 0), MAX(predicted_at) FROM lottery_predictions`
	where, args := buildWhere(lotteryType)
	query += where

	var s Snapshot
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&s.Count, &s.MaxID, &s.MaxPredictedAt); err != nil {
		return Snapshot{}, wrapErr(err)
	}
	return s, nil
}

// CountByType は lottery_type ごとの件数と最新予測日を返す
func (r *LotteryRepository) CountByType(ctx context.Context) ([]model.TypeSummary, error) {
	const query = `SELECT lottery_type, COUNT(*), MAX(predicted_at)
FROM lottery_predictions
GROUP BY lottery_type
ORDER BY lottery_type`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, wrapErr(err)
	}
	defer rows.Close()

	list := make([]model.TypeSummary, 0, len(model.LotteryTypes))
	for rows.Next() {
		var t model.TypeSummary
		if err := rows.Scan(&t.LotteryType, &t.Count, &t.LatestPredictedAt); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

// LastBatchRuns は batch_name, lottery_type ごとに最新 (id 最大) の実行記録を返す
func (r *LotteryRepository) LastBatchRuns(ctx context.Context) ([]model.BatchRun, error) {
	const query = `SELECT b.batch_name, b.lottery_type, b.status, b.started_at, b.finished_at, b.rows_affected, b.message
FROM batch_runs b
INNER JOIN (
	SELECT batch_name, lottery_type, MAX(id) AS id
	FROM batch_runs
	GROUP BY batch_name, lottery_type
) latest ON latest.id = b.id
ORDER BY b.batch_name, b.lottery_type`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, wrapErr(err)
	}
	defer rows.Close()

	runs := make([]model.BatchRun, 0, len(model.LotteryTypes))
	for rows.Next() {
		var b model.BatchRun
		if err := rows.Scan(&b.BatchName, &b.LotteryType, &b.Status, &b.StartedAt, &b.FinishedAt, &b.RowsAffected, &b.Message); err != nil {
			return nil, err
		}
		runs = append(runs, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return runs, nil
}
