package auth

import (
	"crypto/subtle"
	"net/http"
)

// secureEqual 恒定时间字符串比较（防时序侧信道；Token 比较必须使用）。
func secureEqual(a, b string) bool {
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
