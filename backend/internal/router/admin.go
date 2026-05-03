// Package router 管理后台路由。
package router

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kleinai/backend/internal/bootstrap"
	"github.com/kleinai/backend/internal/handler"
	"github.com/kleinai/backend/internal/middleware"
	"github.com/kleinai/backend/internal/repo"
	"github.com/kleinai/backend/internal/service"
	"github.com/kleinai/backend/pkg/jwtx"
)

// MountAdmin 在 root 上挂载 /admin/api/v1。
// 这里提供 AccountPool 实例，供后续 worker / openai 服务可能复用（暂存内部）。
func MountAdmin(r *gin.Engine, deps *bootstrap.Deps) *service.AccountPool {
	v1 := r.Group("/admin/api/v1")

	v1.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"pong": true, "scope": "admin"})
	})

	if deps.DB == nil {
		return nil
	}
	if deps.AES == nil {
		// 没有 AES 也禁止挂载账号池接口（凭证必须加密）
		return nil
	}

	// === repos ===
	adminRepo := repo.NewAdminRepo(deps.DB)
	accountRepo := repo.NewAccountRepo(deps.DB)
	walletRepo := repo.NewWalletRepo(deps.DB)

	// === pool ===
	pool := service.NewAccountPool(accountRepo, 30*time.Second)

	// === services ===
	adminAuth := service.NewAdminAuthService(adminRepo, deps.JWT)
	accountAdmin := service.NewAccountAdminService(accountRepo, pool, deps.AES)
	billingSvc := service.NewBillingService(deps.DB, walletRepo)
	cdkSvc := service.NewCDKService(deps.DB, billingSvc)

	// === handlers ===
	authH := handler.NewAdminAuthHandler(adminAuth, adminRepo)
	accountH := handler.NewAdminAccountHandler(accountAdmin, pool)
	cdkH := handler.NewAdminCDKHandler(cdkSvc)

	// auth 公开
	auth := v1.Group("/auth")
	if deps.Limiter != nil {
		auth.Use(middleware.RateLimitIP(deps.Limiter, 30))
	}
	auth.POST("/login", authH.Login)

	// 登录后接口
	authed := v1.Group("/")
	authed.Use(middleware.AuthJWT(deps.JWT, jwtx.SubjectAdmin))
	{
		authed.GET("/auth/me", authH.Me)

		acc := authed.Group("/accounts")
		{
			acc.GET("", accountH.List)
			acc.POST("", accountH.Create)
			acc.PUT("/:id", accountH.Update)
			acc.DELETE("/:id", accountH.Delete)
			acc.POST("/import", accountH.BatchImport)
			acc.GET("/stats", accountH.PoolStats)
		}

		cdk := authed.Group("/cdk")
		{
			cdk.POST("/batches", cdkH.CreateBatch)
		}
	}

	return pool
}
