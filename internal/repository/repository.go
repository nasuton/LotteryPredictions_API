package repository

import (
	"context"
	"errors"

	"LotteryPredictions_API/internal/model"
)

// ErrTableNotFound は参照先テーブルが存在しない (MySQL error 1146) 場合に返す。
// バッチ側のスキーマ移行 (batch_runs 追加) 前でも API を落とさないために使う。
var ErrTableNotFound = errors.New("table not found")

// Snapshot は一覧の状態を要約した値。ETag の算出に使う。
// updated_at 列に依存せず、件数・最大 ID・最新予測日から構成する。
type Snapshot struct {
	Count          int64
	MaxID          int64
	MaxPredictedAt model.Date
}

// PredictionRepository は予測データの読み取りを抽象化する。handler はこれに依存する。
type PredictionRepository interface {
	// FindAll は predicted_at, id の降順で複数レコードを返す。lotteryType が空なら絞り込まない
	FindAll(ctx context.Context, lotteryType string, limit, offset int) ([]model.LotteryPrediction, error)
	// FindByID は1件を返す。存在しなければ sql.ErrNoRows を返す
	FindByID(ctx context.Context, id int64) (*model.LotteryPrediction, error)
	// Snapshot は条件に一致するレコード群の件数・最大 ID・最新予測日を返す
	Snapshot(ctx context.Context, lotteryType string) (Snapshot, error)
}

// StatusRepository は /api/v1/status 用の集計を抽象化する
type StatusRepository interface {
	// CountByType は lottery_type ごとの件数と最新予測日を返す
	CountByType(ctx context.Context) ([]model.TypeSummary, error)
	// LastBatchRuns は batch_name, lottery_type ごとの最新実行を返す。
	// batch_runs テーブルが存在しない場合は ErrTableNotFound を返す
	LastBatchRuns(ctx context.Context) ([]model.BatchRun, error)
}
