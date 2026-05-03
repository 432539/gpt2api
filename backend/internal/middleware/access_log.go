package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kleinai/backend/pkg/logger"
)

// AccessLog 记录 HTTP 访问日志（结构化 JSON）。
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		latency := time.Since(start)
		l := logger.FromCtx(c.Request.Context())

		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.String("query", c.Request.URL.RawQuery),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", latency),
			zap.Int("size", c.Writer.Size()),
			zap.String("ip", c.ClientIP()),
			zap.String("ua", c.Request.UserAgent()),
		}

		if len(c.Errors) > 0 {
			fields = append(fields, zap.String("errors", c.Errors.String()))
			l.Error("http", fields...)
			return
		}

		switch {
		case c.Writer.Status() >= 500:
			l.Error("http", fields...)
		case c.Writer.Status() >= 400:
			l.Warn("http", fields...)
		default:
			l.Info("http", fields...)
		}
	}
}
