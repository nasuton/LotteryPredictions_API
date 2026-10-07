package repository

import (
	"context"

	"LotteryPredictions_API/internal/model"
)

// FindHitRates は lottery_hit_rates の全カラムを取得する。
func (r *LotteryRepository) FindHitRates(ctx context.Context, lotteryType string, limit, offset int) ([]model.LotteryHitRate, error) {
	query := `SELECT lottery_type, pattern, prediction_count, mean_hits, hit_rate,
match_3_rate, match_4_rate, match_5_rate, match_6_rate, match_7_rate, created_at, updated_at
FROM lottery_hit_rates`
	where, args := buildWhere(lotteryType)
	query += where + ` ORDER BY lottery_type ASC, pattern ASC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, wrapErr(err)
	}
	defer rows.Close()

	hitRates := make([]model.LotteryHitRate, 0, limit)
	for rows.Next() {
		var rate model.LotteryHitRate
		if err := rows.Scan(
			&rate.LotteryType, &rate.Pattern, &rate.PredictionCount, &rate.MeanHits, &rate.HitRate,
			&rate.Match3Rate, &rate.Match4Rate, &rate.Match5Rate, &rate.Match6Rate, &rate.Match7Rate,
			&rate.CreatedAt, &rate.UpdatedAt,
		); err != nil {
			return nil, err
		}
		hitRates = append(hitRates, rate)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return hitRates, nil
}

func (r *LotteryRepository) CountHitRates(ctx context.Context, lotteryType string) (int64, error) {
	query := `SELECT COUNT(*) FROM lottery_hit_rates`
	where, args := buildWhere(lotteryType)
	var total int64
	if err := r.db.QueryRowContext(ctx, query+where, args...).Scan(&total); err != nil {
		return 0, wrapErr(err)
	}
	return total, nil
}
