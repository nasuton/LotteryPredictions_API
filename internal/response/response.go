// Package response は API のレスポンスヘッダとエラー形式を統一する
package response

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"

	"LotteryPredictions_API/internal/middleware"
)

const (
	// CacheControlPredictions は一覧・単一取得のキャッシュ指示
	CacheControlPredictions = "public, max-age=300, stale-while-revalidate=600"
	// CacheControlStatus は /status のキャッシュ指示
	CacheControlStatus = "public, max-age=300"
	// CacheControlNoStore はエラー・ヘルスチェック用
	CacheControlNoStore = "no-store"
)

// ErrorBody はエラーレスポンスの形式
//
//	{"error": {"code": "invalid_id", "message": "...", "request_id": "..."}}
type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail はエラーの詳細
type ErrorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

// Error は統一形式のエラーレスポンスを書き出す。Cache-Control は常に no-store
func Error(c *gin.Context, status int, code, message string) {
	h := c.Writer.Header()
	h.Del("ETag")
	h.Set("Cache-Control", CacheControlNoStore)
	c.JSON(status, ErrorBody{Error: ErrorDetail{
		Code:      code,
		Message:   message,
		RequestID: middleware.GetRequestID(c),
	}})
}

// SetCache は Cache-Control ヘッダを設定する
func SetCache(c *gin.Context, value string) {
	c.Header("Cache-Control", value)
}

// WeakETag は与えられた値を "|" 区切りで連結し SHA-1 した弱い ETag を返す
func WeakETag(parts ...any) string {
	h := sha1.New()
	for i, p := range parts {
		if i > 0 {
			h.Write([]byte("|"))
		}
		fmt.Fprint(h, p)
	}
	return `W/"` + hex.EncodeToString(h.Sum(nil)) + `"`
}

// ETagMatches は If-None-Match の値に etag が含まれるかを弱い比較で判定する
func ETagMatches(ifNoneMatch, etag string) bool {
	ifNoneMatch = strings.TrimSpace(ifNoneMatch)
	if ifNoneMatch == "" {
		return false
	}
	if ifNoneMatch == "*" {
		return true
	}
	want := strings.TrimPrefix(etag, "W/")
	for _, candidate := range strings.Split(ifNoneMatch, ",") {
		candidate = strings.TrimPrefix(strings.TrimSpace(candidate), "W/")
		if candidate != "" && candidate == want {
			return true
		}
	}
	return false
}
