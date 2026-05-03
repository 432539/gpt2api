// Package router OpenAI 兼容服务路由。
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
)

// MountOpenAI 挂载 /v1（OpenAI 兼容）。
//
// 公开路由（无需鉴权）：
//
//	GET  /v1/health
//
// 受 API Key 保护：
//
//	GET  /v1/models
//	POST /v1/chat/completions
//	POST /v1/images/generations
//	POST /v1/images/edits
//	GET  /v1/images/generations/:task_id
//	POST /v1/video/generations
//	GET  /v1/video/generations/:task_id
func MountOpenAI(r *gin.Engine, deps *bootstrap.Deps) {
	v1 := r.Group("/v1")
	v1.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	if deps.DB == nil {
		// 降级：DB 未连，受 KEY 保护的路由不挂载
		return
	}

	apiKeyRepo := repo.NewAPIKeyRepo(deps.DB)
	walletRepo := repo.NewWalletRepo(deps.DB)
	accountRepo := repo.NewAccountRepo(deps.DB)
	genRepo := repo.NewGenerationRepo(deps.DB)
	sysCfgRepo := repo.NewSystemConfigRepo(deps.DB)
	proxyRepo := repo.NewProxyRepo(deps.DB)

	keySvc := service.NewAPIKeyService(apiKeyRepo)
	billingSvc := service.NewBillingService(deps.DB, walletRepo)
	sysCfgSvc := service.NewSystemConfigService(sysCfgRepo)
	proxySvc := service.NewProxyService(proxyRepo, deps.AES)
	pool := service.NewAccountPool(accountRepo, 30*time.Second)
	providers := factory.Build()
	genSvc := service.NewGenerationService(deps.DB, genRepo, pool, billingSvc, providers, service.ConfigPriceFn(sysCfgSvc), deps.AES, proxySvc, sysCfgSvc)
	chatSvc := service.NewChatService(deps.DB, genRepo, pool, billingSvc, sysCfgSvc, deps.AES, proxySvc)
	openaiH := handler.NewOpenAIHandler(genSvc, chatSvc, genRepo)

	guard := v1.Group("/")
	guard.Use(middleware.AuthAPIKey(keySvc))
	{
		guard.GET("/models", openaiH.Models)
		guard.POST("/chat/completions", openaiH.ChatCompletions)
		guard.POST("/images/generations", openaiH.ImageGenerations)
		guard.GET("/images/generations/:task_id", openaiH.GetImageTask)
		guard.POST("/images/edits", openaiH.ImageEdits)
		guard.POST("/video/generations", openaiH.VideoGenerations)
		guard.GET("/video/generations/:task_id", openaiH.GetVideoTask)
		// Backward-compatible alias kept for older clients.
		guard.POST("/videos/generations", openaiH.VideoGenerations)
		guard.GET("/videos/generations/:task_id", openaiH.GetVideoTask)
	}
}
