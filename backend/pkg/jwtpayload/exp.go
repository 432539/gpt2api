// Package jwtpayload 从 JWT 字符串安全解析非签名载荷（仅解码 payload，不校验签名）。
package jwtpayload

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

// ClaimsFromJWT returns the unsigned JWT payload as a generic map.
// It only decodes the payload and does not verify the signature.
func ClaimsFromJWT(token string) (map[string]any, bool) {
	token = strings.TrimSpace(token)
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		payload, err = base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, false
		}
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, false
	}
	return claims, true
}

// ExpUnixFromJWT 返回标准 JWT 的 exp 声明（unix 秒），无法解析时为 (0, false)。
func ExpUnixFromJWT(token string) (int64, bool) {
	claims, ok := ClaimsFromJWT(token)
	if !ok {
		return 0, false
	}
	switch exp := claims["exp"].(type) {
	case float64:
		if exp > 0 {
			return int64(exp), true
		}
	case json.Number:
		if n, err := exp.Int64(); err == nil && n > 0 {
			return n, true
		}
	}
	return 0, false
}

// StringClaimFromJWT extracts a top-level string claim from an unsigned JWT payload.
func StringClaimFromJWT(token, key string) (string, bool) {
	claims, ok := ClaimsFromJWT(token)
	if !ok {
		return "", false
	}
	v, ok := claims[key].(string)
	if !ok || strings.TrimSpace(v) == "" {
		return "", false
	}
	return strings.TrimSpace(v), true
}
