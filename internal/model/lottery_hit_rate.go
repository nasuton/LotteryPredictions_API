package model

import "time"

// HitRateLotteryTypes はヒット率テーブルの対象種別。
var HitRateLotteryTypes = []string{"loto6", "loto7", "miniloto"}

func IsValidHitRateLotteryType(v string) bool {
	for _, t := range HitRateLotteryTypes {
		if v == t {
			return true
		}
	}
	return false
}

// LotteryHitRate は lottery_hit_rates の最新パターン別集計。
// 対象外の一致率は JSON で null を返す。
type LotteryHitRate struct {
	LotteryType     string    `json:"lottery_type"`
	Pattern         string    `json:"pattern"`
	PredictionCount int64     `json:"prediction_count"`
	MeanHits        float64   `json:"mean_hits"`
	HitRate         float64   `json:"hit_rate"`
	Match3Rate      float64   `json:"match_3_rate"`
	Match4Rate      float64   `json:"match_4_rate"`
	Match5Rate      float64   `json:"match_5_rate"`
	Match6Rate      *float64  `json:"match_6_rate"`
	Match7Rate      *float64  `json:"match_7_rate"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
