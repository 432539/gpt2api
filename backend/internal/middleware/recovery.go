// Package middleware 提供 HTTP 中间件集合。
package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kleinai/backend/pkg/errcode"
	"github.com/kleinai/backend/pkg/logger"
	"github.com/kleinai/backend/pkg/response"
)

// Recovery 捕获 panic 并返回 500，同时记录堆栈。
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.FromCtx(c.Request.Context()).Error("panic recovered",
					zap.Any("panic", rec),
					zap.ByteString("stack", debug.Stack()),
					zap.String("path", c.Request.URL.Path),
				)
				if !c.Writer.Written() {
					response.Fail(c, errcode.Internal)
					c.AbortWithStatus(http.StatusInternalServerError)
				}
			}
		}()
		c.Next()
	}
}
