package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// dateLayout は predicted_at (DATE型) の入出力フォーマット
const dateLayout = "2006-01-02"

// Date は DATE 型カラム用。JSON では "2006-01-02" 形式で入出力する
type Date time.Time

func (d Date) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Time(d).Format(dateLayout))
}

func (d *Date) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return err
	}
	*d = Date(t)
	return nil
}

// Scan は database/sql から値を読み取る (DSN の parseTime=true 有無どちらにも対応)
func (d *Date) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*d = Date(time.Time{})
	case time.Time:
		*d = Date(v)
	case []byte:
		return d.parse(string(v))
	case string:
		return d.parse(v)
	default:
		return fmt.Errorf("model.Date: unsupported scan type %T", src)
	}
	return nil
}

func (d *Date) parse(s string) error {
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return err
	}
	*d = Date(t)
	return nil
}

// Value は database/sql へ値を渡す
func (d Date) Value() (driver.Value, error) {
	return time.Time(d).Format(dateLayout), nil
}

func (d Date) String() string {
	return time.Time(d).Format(dateLayout)
}

// LotteryPrediction は lottery_predictions テーブルの1レコードを表す
type LotteryPrediction struct {
	// ID は主キー (auto_increment)
	ID int64 `json:"id"`
	// LotteryType は loto6 / loto7 などの対象名
	LotteryType string `json:"lottery_type"`
	// Pattern は数字の選択に使用したパターン名
	Pattern string `json:"pattern"`
	// PredictedAt はレコードの作成日
	PredictedAt Date `json:"predicted_at"`
	// Numbers は選択された数字。DB 上はカンマ区切りの文字列
	Numbers string `json:"-"`
}

// NumberList は Numbers をカンマで分割したスライスを返す
func (p LotteryPrediction) NumberList() []string {
	parts := strings.Split(p.Numbers, ",")
	list := make([]string, 0, len(parts))
	for _, v := range parts {
		if v = strings.TrimSpace(v); v != "" {
			list = append(list, v)
		}
	}
	return list
}

// MarshalJSON は numbers を配列、numbers_raw を元の文字列として出力する
func (p LotteryPrediction) MarshalJSON() ([]byte, error) {
	type alias LotteryPrediction
	return json.Marshal(struct {
		alias
		Numbers    []string `json:"numbers"`
		NumbersRaw string   `json:"numbers_raw"`
	}{
		alias:      alias(p),
		Numbers:    p.NumberList(),
		NumbersRaw: p.Numbers,
	})
}
