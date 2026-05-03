// Package router api 服务路由组装。
package router

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kleinai/backend/internal/bootstrap"
	"github.com/kleinai/backend/internal/handler"
	"github.com/kleinai/backend/internal/middleware"
	"github.com/kleinai/backend/internal/provider/factory"
	"github.com/kleinai/backend/internal/repo"
	"github.com/kleinai/backend/internal/service"
	"github.com/kleinai/backend/pkg/jwtx"
)

// MountAPI 在 root 上挂载用户端 /api/v1 全部业务路由。
// 注意：未配置 DB 时（dev 降级）会跳过依赖 DB 的路由，仅保留 /ping。
func MountAPI(r *gin.Engine, deps *bootstrap.Deps) {
	v1 := r.Group("/api/v1")

	v1.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"pong": true})
	})

	if deps.DB == nil {
		return
	}

	userRepo := repo.NewUserRepo(deps.DB)
	apiKeyRepo := repo.NewAPIKeyRepo(deps.DB)
	walletRepo := repo.NewWalletRepo(deps.DB)
	accountRepo := repo.NewAccountRepo(deps.DB)
	genRepo := repo.NewGenerationRepo(deps.DB)
	sysCfgRepo := repo.NewSystemConfigRepo(deps.DB)
	proxyRepo := repo.NewProxyRepo(deps.DB)

	authSvc := service.NewAuthService(deps.DB, userRepo, deps.JWT)
	userSvc := service.NewUserService(userRepo)
	keySvc := service.NewAPIKeyService(apiKeyRepo)
	billingSvc := service.NewBillingService(deps.DB, walletRepo)
	cdkSvc := service.NewCDKService(deps.DB, billingSvc)
	sysCfgSvc := service.NewSystemConfigService(sysCfgRepo)
	proxySvc := service.NewProxyService(proxyRepo, deps.AES)

	pool := service.NewAccountPool(accountRepo, 30*time.Second)
	providers := factory.Build()
	genSvc := service.NewGenerationService(deps.DB, genRepo, pool, billingSvc, providers, service.ConfigPriceFn(sysCfgSvc), deps.AES, proxySvc, sysCfgSvc)
	chatSvc := service.NewChatService(deps.DB, genRepo, pool, billingSvc, sysCfgSvc, deps.AES, proxySvc)

	authH := handler.NewAuthHandler(authSvc, userSvc)
	keyH := handler.NewAPIKeyHandler(keySvc)
	billH := handler.NewBillingHandler(billingSvc, cdkSvc)
	genH := handler.NewGenerationHandler(genSvc, chatSvc, genRepo, accountRepo, sysCfgSvc, deps.AES)

	v1.GET("/models", genH.Models)
	v1.GET("/gen/cached/*path", genH.CachedAsset)
	v1.GET("/gen/assets/:task_id/:seq", genH.Asset)

	auth := v1.Group("/auth")
	{
		// 注册 / 登录限流：每 IP 每分钟 30 次
		if deps.Limiter != nil {
			auth.Use(middleware.RateLimitIP(deps.Limiter, 30))
		}
		auth.POST("/register", authH.Register)
		auth.POST("/login", authH.Login)
		auth.POST("/refresh", authH.Refresh)
		auth.POST("/logout", authH.Logout)
	}

	// 需要登录的用户接口
	authed := v1.Group("/")
	authed.Use(middleware.AuthJWT(deps.JWT, jwtx.SubjectUser))
	{
		authed.GET("/users/me", authH.Me)
		authed.POST("/users/password", authH.ChangePassword)

		keys := authed.Group("/keys")
		{
			keys.GET("", keyH.List)
			keys.POST("", keyH.Create)
			keys.POST("/:id/toggle", keyH.Toggle)
			keys.DELETE("/:id", keyH.Delete)
		}

		bill := authed.Group("/billing")
		{
			bill.GET("/logs", billH.Logs)
			bill.POST("/cdk/redeem", billH.RedeemCDK)
		}

		gen := authed.Group("/gen")
		{
			gen.POST("/image", genH.CreateImage)
			gen.POST("/text", genH.CreateText)
			gen.POST("/video", genH.CreateVideo)
			gen.GET("/tasks/:task_id", genH.Get)
			gen.GET("/history", genH.List)
			gen.DELETE("/history", genH.DeleteHistory)
		}
	}
}
