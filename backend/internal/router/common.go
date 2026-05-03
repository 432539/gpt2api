// Package router 提供通用 gin Engine 构造（含基础中间件 + healthz）。
package router

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kleinai/backend/internal/bootstrap"
	"github.com/kleinai/backend/internal/middleware"
	"github.com/kleinai/backend/pkg/version"
)

// Options 路由构造选项。
type Options struct {
	ServiceName string
	Deps        *bootstrap.Deps
}

// New 返回带基础中间件 + 健康检查 + ready 检查的 gin.Engine。
func New(opt Options) *gin.Engine {
	if opt.Deps.Cfg.IsProd() {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(
		middleware.Recovery(),
		middleware.RequestID(),
		middleware.AccessLog(),
		middleware.SecurityHeaders(),
		middleware.CORS(opt.Deps.Cfg.CORS.Origins),
	)

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"service": opt.ServiceName,
			"env":     opt.Deps.Cfg.App.Env,
			"version": version.Build,
			"time":    time.Now().UTC().Format(time.RFC3339),
		})
	})

	r.GET("/readyz", func(c *gin.Context) {
		ready := true
		details := gin.H{}
		if opt.Deps.DB != nil {
			if sqlDB, err := opt.Deps.DB.DB(); err == nil {
				if err := sqlDB.PingContext(c.Request.Context()); err != nil {
					ready, details["mysql"] = false, err.Error()
				} else {
					details["mysql"] = "ok"
				}
			} else {
				ready, details["mysql"] = false, err.Error()
			}
		} else {
			ready, details["mysql"] = false, "not initialized"
		}
		if opt.Deps.Redis != nil {
			if err := opt.Deps.Redis.Ping(c.Request.Context()).Err(); err != nil {
				ready, details["redis"] = false, err.Error()
			} else {
				details["redis"] = "ok"
			}
		} else {
			ready, details["redis"] = false, "not initialized"
		}
		status := http.StatusOK
		if !ready {
			status = http.StatusServiceUnavailable
		}
		c.JSON(status, gin.H{"ready": ready, "details": details})
	})

	return r
}
