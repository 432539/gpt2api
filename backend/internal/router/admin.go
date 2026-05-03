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
	userRepo := repo.NewUserRepo(deps.DB)
	accountRepo := repo.NewAccountRepo(deps.DB)
	walletRepo := repo.NewWalletRepo(deps.DB)
	generationRepo := repo.NewGenerationRepo(deps.DB)
	proxyRepo := repo.NewProxyRepo(deps.DB)
	sysCfgRepo := repo.NewSystemConfigRepo(deps.DB)
	promoRepo := repo.NewPromoRepo(deps.DB)
	dashboardRepo := repo.NewDashboardRepo(deps.DB)

	// === pool ===
	pool := service.NewAccountPool(accountRepo, 30*time.Second)

	// === services ===
	adminAuth := service.NewAdminAuthService(adminRepo, deps.JWT)
	adminUserSvc := service.NewAdminUserService(userRepo, walletRepo)
	accountAdmin := service.NewAccountAdminService(accountRepo, pool, deps.AES)
	billingSvc := service.NewBillingService(deps.DB, walletRepo)
	cdkSvc := service.NewCDKService(deps.DB, billingSvc)
	promoSvc := service.NewAdminPromoService(promoRepo)
	sysCfgSvc := service.NewSystemConfigService(sysCfgRepo)
	proxySvc := service.NewProxyService(proxyRepo, deps.AES)
	openaiOAuth := service.NewOpenAIOAuthService(sysCfgSvc)
	accountTest := service.NewAccountTestService(accountRepo, proxySvc, sysCfgSvc, openaiOAuth, deps.AES)
	// 把测试服务注入 AccountAdminService，使 Test/Refresh/BatchRefresh 走得通。
	accountAdmin.SetTestService(accountTest)

	// === handlers ===
	authH := handler.NewAdminAuthHandler(adminAuth, adminRepo)
	userH := handler.NewAdminUserHandler(adminUserSvc)
	accountH := handler.NewAdminAccountHandler(accountAdmin, pool)
	cdkH := handler.NewAdminCDKHandler(cdkSvc)
	billingH := handler.NewAdminBillingHandler(walletRepo)
	promoH := handler.NewAdminPromoHandler(promoSvc)
	proxyH := handler.NewAdminProxyHandler(proxySvc, accountTest)
	sysH := handler.NewAdminSystemHandler(sysCfgSvc)
	logH := handler.NewAdminLogHandler(generationRepo, accountRepo, deps.AES)
	dashboardH := handler.NewAdminDashboardHandler(dashboardRepo)

	// auth 公开
	auth := v1.Group("/auth")
	if deps.Limiter != nil {
		auth.Use(middleware.RateLimitIP(deps.Limiter, 30))
	}
	auth.POST("/login", authH.Login)
	v1.GET("/logs/generations/:task_id/preview", logH.GenerationPreview)

	// 登录后接口
	authed := v1.Group("/")
	authed.Use(middleware.AuthJWT(deps.JWT, jwtx.SubjectAdmin))
	{
		authed.GET("/auth/me", authH.Me)
		authed.POST("/auth/password", authH.ChangePassword)
		authed.GET("/dashboard/overview", dashboardH.Overview)

		users := authed.Group("/users")
		{
			users.GET("", userH.List)
			users.POST("", userH.Create)
			users.PUT("/:id", userH.Update)
			users.POST("/:id/points", userH.AdjustPoints)
		}

		acc := authed.Group("/accounts")
		{
			acc.GET("", accountH.List)
			acc.POST("", accountH.Create)
			acc.POST("/import", accountH.BatchImport)
			acc.POST("/batch-delete", accountH.BatchDelete)
			acc.POST("/purge", accountH.Purge)
			acc.POST("/batch-refresh", accountH.BatchRefresh)
			acc.POST("/batch-probe", accountH.BatchProbeQuota)
			acc.GET("/stats", accountH.PoolStats)
			acc.PUT("/:id", accountH.Update)
			acc.DELETE("/:id", accountH.Delete)
			acc.POST("/:id/test", accountH.Test)
			acc.POST("/:id/refresh", accountH.RefreshOAuth)
			acc.GET("/:id/secrets", accountH.Secrets)
		}

		// 代理管理：CRUD + 连通性测试
		proxies := authed.Group("/proxies")
		{
			proxies.GET("", proxyH.List)
			proxies.POST("", proxyH.Create)
			proxies.PUT("/:id", proxyH.Update)
			proxies.DELETE("/:id", proxyH.Delete)
			proxies.POST("/:id/test", proxyH.Test)
		}

		// 系统配置：通用 KV（代理全局开关 / OAuth 客户端 / refresh 阈值等）
		sys := authed.Group("/system")
		{
			sys.GET("/settings", sysH.GetSettings)
			sys.PUT("/settings", sysH.UpdateSettings)
			sys.GET("/cache", sysH.CacheStats)
			sys.DELETE("/cache", sysH.CleanCache)
		}

		cdk := authed.Group("/cdk")
		{
			cdk.POST("/batches", cdkH.CreateBatch)
		}

		billing := authed.Group("/billing")
		{
			billing.GET("/wallet-logs", billingH.WalletLogs)
		}

		promo := authed.Group("/promo")
		{
			promo.GET("/codes", promoH.List)
			promo.POST("/codes", promoH.Create)
			promo.PUT("/codes/:id", promoH.Update)
			promo.DELETE("/codes/:id", promoH.Delete)
		}

		logs := authed.Group("/logs")
		{
			logs.GET("/generations", logH.GenerationLogs)
			logs.GET("/generations/:task_id/upstream", logH.GenerationUpstreamLogs)
			logs.DELETE("/generations", logH.PurgeGenerationLogs)
		}
	}

	return pool
}
