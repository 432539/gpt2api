package middleware

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kleinai/backend/pkg/errcode"
	"github.com/kleinai/backend/pkg/jwtx"
	"github.com/kleinai/backend/pkg/response"
)

type ctxKey string

const (
	CtxUID     ctxKey = "kc:uid"
	CtxClaims  ctxKey = "kc:claims"
	CtxSubject ctxKey = "kc:sub"
)

// AuthJWT 校验 Bearer token。expectSub 用于区分用户端 / 管理后台。
func AuthJWT(mgr *jwtx.Manager, expectSub jwtx.Subject) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			response.Fail(c, errcode.Unauthorized)
			return
		}
		tok := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		claims, err := mgr.ParseAccess(tok)
		if err != nil {
			response.Fail(c, errcode.TokenExpired.Wrap(err))
			return
		}
		if claims.Subject != expectSub {
			response.Fail(c, errcode.TokenInvalid)
			return
		}

		ctx := context.WithValue(c.Request.Context(), CtxUID, claims.UID)
		ctx = context.WithValue(ctx, CtxClaims, claims)
		ctx = context.WithValue(ctx, CtxSubject, claims.Subject)
		c.Request = c.Request.WithContext(ctx)
		c.Set(string(CtxUID), claims.UID)
		c.Set(string(CtxClaims), claims)
		c.Next()
	}
}

// MustUID 从 ctx / gin.Context 取 UID（若不存在则 panic，确保中间件已生效）。
func MustUID(c *gin.Context) uint64 {
	if v, ok := c.Get(string(CtxUID)); ok {
		if uid, ok2 := v.(uint64); ok2 {
			return uid
		}
	}
	if v := c.Request.Context().Value(CtxUID); v != nil {
		if uid, ok := v.(uint64); ok {
			return uid
		}
	}
	panic("uid missing in context; AuthJWT middleware not applied?")
}

// UID 安全版（不存在时返回 0）。
func UID(c *gin.Context) uint64 {
	if v, ok := c.Get(string(CtxUID)); ok {
		if uid, ok2 := v.(uint64); ok2 {
			return uid
		}
	}
	return 0
}

// MustClaims 取 claims。
func MustClaims(c *gin.Context) *jwtx.Claims {
	if v, ok := c.Get(string(CtxClaims)); ok {
		if cl, ok2 := v.(*jwtx.Claims); ok2 {
			return cl
		}
	}
	panic("claims missing in context")
}
