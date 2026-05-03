package middleware

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kleinai/backend/internal/model"
	"github.com/kleinai/backend/internal/service"
	"github.com/kleinai/backend/pkg/errcode"
	"github.com/kleinai/backend/pkg/response"
)

const (
	CtxAPIKey  ctxKey = "kc:apikey"
	CtxKeyUID  ctxKey = "kc:apikey_uid"
	CtxKeyScope ctxKey = "kc:apikey_scope"
)

// AuthAPIKey OpenAI 兼容服务鉴权：Authorization: Bearer sk-klein-xxxx。
func AuthAPIKey(svc *service.APIKeyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			response.Fail(c, errcode.APIKeyInvalid)
			return
		}
		plain := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		k, err := svc.Verify(c.Request.Context(), plain)
		if err != nil {
			response.Fail(c, err)
			return
		}
		ctx := context.WithValue(c.Request.Context(), CtxAPIKey, k)
		ctx = context.WithValue(ctx, CtxKeyUID, k.UserID)
		ctx = context.WithValue(ctx, CtxKeyScope, k.Scope)
		c.Request = c.Request.WithContext(ctx)
		c.Set(string(CtxAPIKey), k)
		c.Set(string(CtxKeyUID), k.UserID)
		c.Set(string(CtxKeyScope), k.Scope)
		c.Next()
	}
}

// APIKeyFromCtx 从 ctx / gin.Context 中取出 *model.APIKey；若无返回 nil。
func APIKeyFromCtx(c *gin.Context) *model.APIKey {
	if v, ok := c.Get(string(CtxAPIKey)); ok {
		if k, ok2 := v.(*model.APIKey); ok2 {
			return k
		}
	}
	if v := c.Request.Context().Value(CtxAPIKey); v != nil {
		if k, ok := v.(*model.APIKey); ok {
			return k
		}
	}
	return nil
}

// APIKeyScopeAllow 检查 scope 是否允许给定 action（image / video / chat）。
func APIKeyScopeAllow(c *gin.Context, action string) bool {
	scope := ""
	if v, ok := c.Get(string(CtxKeyScope)); ok {
		scope, _ = v.(string)
	}
	if scope == "" || scope == "*" {
		return true
	}
	for _, s := range strings.Split(scope, ",") {
		if strings.TrimSpace(s) == action {
			return true
		}
	}
	return false
}
