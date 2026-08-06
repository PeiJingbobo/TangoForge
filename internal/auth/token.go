package auth

import (
	"crypto/subtle"
	"net/http"
)

// SecureEqual 恒定时间字符串比较（防时序侧信道；Token 比较必须使用）。
// 供 auth 内部与 api（MCP HTTP 鉴权，TF-016）复用。
func SecureEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// BearerToken 提取 Authorization: Bearer <token> 中的 token；格式不符返回空串。
func BearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) > len(prefix) && h[:len(prefix)] == prefix {
		return h[len(prefix):]
	}
	return ""
}
