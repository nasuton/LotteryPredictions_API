package middleware

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/gin-gonic/gin"
)

const (
	// HeaderRequestID はリクエスト ID を受け渡すヘッダ名
	HeaderRequestID = "X-Request-ID"
	// ContextKeyRequestID は gin.Context に格納するキー
	ContextKeyRequestID = "request_id"
	// maxRequestIDLength は外部から受け取る ID の最大長
	maxRequestIDLength = 128
)

// RequestID は X-Request-ID を受け取れば流用し、無ければ生成してコンテキストとレスポンスヘッダに設定する
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(HeaderRequestID)
		if !isValidRequestID(id) {
			id = newRequestID()
		}
		c.Set(ContextKeyRequestID, id)
		c.Header(HeaderRequestID, id)
		c.Next()
	}
}

// GetRequestID はコンテキストからリクエスト ID を取り出す。未設定なら空文字
func GetRequestID(c *gin.Context) string {
	return c.GetString(ContextKeyRequestID)
}

// newRequestID は crypto/rand 由来の 16 byte を hex 化した 32 文字の ID を返す
func newRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// isValidRequestID はログ汚染を避けるため、印字可能 ASCII のみ・長さ上限内の ID だけを流用対象とする
func isValidRequestID(id string) bool {
	if id == "" || len(id) > maxRequestIDLength {
		return false
	}
	for i := 0; i < len(id); i++ {
		if id[i] < 0x21 || id[i] > 0x7e {
			return false
		}
	}
	return true
}
