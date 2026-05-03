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
//   GET  /v1/health
// 受 API Key 保护：
//   GET  /v1/models
//   POST /v1/images/generations    (Sprint 7 实现)
//   POST /v1/videos/generations    (Sprint 7 实现)
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

	keySvc := service.NewAPIKeyService(apiKeyRepo)
	billingSvc := service.NewBillingService(deps.DB, walletRepo)
	pool := service.NewAccountPool(accountRepo, 30*time.Second)
	providers := factory.Build()
	genSvc := service.NewGenerationService(deps.DB, genRepo, pool, billingSvc, providers, service.DefaultPriceFn, deps.AES)
	openaiH := handler.NewOpenAIHandler(genSvc, genRepo)

	guard := v1.Group("/")
	guard.Use(middleware.AuthAPIKey(keySvc))
	{
		guard.GET("/models", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"object": "list",
				"data": []gin.H{
					{"id": "img-v3", "object": "model", "owned_by": "kleinai"},
					{"id": "img-real", "object": "model", "owned_by": "kleinai"},
					{"id": "img-anime", "object": "model", "owned_by": "kleinai"},
					{"id": "vid-v1", "object": "model", "owned_by": "kleinai"},
				},
			})
		})

		guard.POST("/images/generations", openaiH.ImageGenerations)
		guard.POST("/videos/generations", openaiH.VideoGenerations)
	}
}
