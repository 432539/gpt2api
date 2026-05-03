package service

import (
	"strings"
)

func normalizeGrokSSOToken(token string) string {
	token = strings.TrimSpace(token)
	token = strings.TrimPrefix(token, "Bearer ")
	token = strings.TrimPrefix(token, "bearer ")
	token = strings.TrimPrefix(token, "sso=")
	token = strings.ReplaceAll(token, "\u200b", "")
	token = strings.ReplaceAll(token, "\ufeff", "")
	return strings.Join(strings.Fields(token), "")
}

func shortTokenName(token string) string {
	token = normalizeGrokSSOToken(token)
	if len(token) <= 10 {
		return token
	}
	return token[:10]
}
