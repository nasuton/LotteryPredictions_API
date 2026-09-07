package repository

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"LotteryPredictions_API/internal/model"
)

// selectColumns は lottery_predictions の取得対象カラム
const selectColumns = `id, lottery_type, pattern, predicted_at, numbers`

// dateLayout は predicted_at (DATE型) の比較用フォーマット
const dateLayout = "2006-01-02"

// buildWhere は lottery_type と「サーバーの現在日付 = predicted_at」の条件を組み立てる
func buildWhere(lotteryType string) (string, []any) {
	conds := make([]string, 0, 2)
	args := make([]any, 0, 2)

	if lotteryType != "" {
		conds = append(conds, `lottery_type = ?`)
		args = append(args, lotteryType)
	}

	// サーバー(アプリ)の現在日付と predicted_at が同じものだけに絞り込む
	conds = append(conds, `predicted_at = ?`)
	args = append(args, time.Now().Format(dateLayout))

	return ` WHERE ` + strings.Join(conds, ` AND `), args
}

// LotteryRepository は lottery_predictions テーブルへのアクセスを担当する
type LotteryRepository struct {
	db *sql.DB
}

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
		return nil, err
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
		return nil, err
	}
	return &p, nil
}

// Count は条件に一致する総件数を返す
func (r *LotteryRepository) Count(ctx context.Context, lotteryType string) (int64, error) {
	query := `SELECT COUNT(*) FROM lottery_predictions`
	where, args := buildWhere(lotteryType)
	query += where

	var total int64
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}
