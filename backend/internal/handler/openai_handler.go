// Package handler OpenAI 兼容协议 handler。
package handler

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kleinai/backend/internal/middleware"
	"github.com/kleinai/backend/internal/model"
	"github.com/kleinai/backend/internal/provider"
	"github.com/kleinai/backend/internal/repo"
	"github.com/kleinai/backend/internal/service"
)

// OpenAIHandler 兼容协议入口（同步等待结果）。
type OpenAIHandler struct {
	svc  *service.GenerationService
	repo *repo.GenerationRepo
}

// NewOpenAIHandler 构造。
func NewOpenAIHandler(svc *service.GenerationService, r *repo.GenerationRepo) *OpenAIHandler {
	return &OpenAIHandler{svc: svc, repo: r}
}

// imageReq OpenAI 风格请求。
type imageReq struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	N      int    `json:"n"`
	Size   string `json:"size"`
}

// ImageGenerations POST /v1/images/generations
func (h *OpenAIHandler) ImageGenerations(c *gin.Context) {
	if !middleware.APIKeyScopeAllow(c, "image") {
		jsonError(c, 403, "scope_not_allowed", "current api key does not allow image generation")
		return
	}
	var req imageReq
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonError(c, 400, "invalid_request_error", err.Error())
		return
	}
	if req.Prompt == "" {
		jsonError(c, 400, "invalid_request_error", "prompt is required")
		return
	}
	if req.N <= 0 {
		req.N = 1
	}
	k := middleware.APIKeyFromCtx(c)
	if k == nil {
		jsonError(c, 401, "invalid_api_key", "api key required")
		return
	}

	t, err := h.svc.Create(c.Request.Context(), service.CreateRequest{
		UserID:    k.UserID,
		APIKeyID:  &k.ID,
		Kind:      provider.KindImage,
		Mode:      provider.ModeT2I,
		ModelCode: req.Model,
		Provider:  model.ProviderGPT,
		Prompt:    req.Prompt,
		Params:    map[string]any{"size": req.Size},
		Count:     req.N,
		IdemKey:   c.GetHeader("Idempotency-Key"),
		ClientIP:  c.ClientIP(),
	})
	if err != nil {
		jsonError(c, 400, "billing_or_pool_error", err.Error())
		return
	}

	results := h.waitTask(c, t.TaskID, 60*time.Second)
	out := []gin.H{}
	for _, r := range results {
		out = append(out, gin.H{"url": r.URL})
	}
	c.JSON(200, gin.H{
		"created": time.Now().Unix(),
		"data":    out,
		"task_id": t.TaskID,
	})
}

type videoReq struct {
	Model    string `json:"model"`
	Prompt   string `json:"prompt"`
	Duration int    `json:"duration"`
	Size     string `json:"size"`
}

// VideoGenerations POST /v1/videos/generations
func (h *OpenAIHandler) VideoGenerations(c *gin.Context) {
	if !middleware.APIKeyScopeAllow(c, "video") {
		jsonError(c, 403, "scope_not_allowed", "current api key does not allow video generation")
		return
	}
	var req videoReq
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonError(c, 400, "invalid_request_error", err.Error())
		return
	}
	if req.Prompt == "" {
		jsonError(c, 400, "invalid_request_error", "prompt is required")
		return
	}
	k := middleware.APIKeyFromCtx(c)
	if k == nil {
		jsonError(c, 401, "invalid_api_key", "api key required")
		return
	}

	params := map[string]any{}
	if req.Duration > 0 {
		params["duration"] = float64(req.Duration)
	}
	if req.Size != "" {
		params["size"] = req.Size
	}

	t, err := h.svc.Create(c.Request.Context(), service.CreateRequest{
		UserID:    k.UserID,
		APIKeyID:  &k.ID,
		Kind:      provider.KindVideo,
		Mode:      provider.ModeT2V,
		ModelCode: req.Model,
		Provider:  model.ProviderGROK,
		Prompt:    req.Prompt,
		Params:    params,
		Count:     1,
		IdemKey:   c.GetHeader("Idempotency-Key"),
		ClientIP:  c.ClientIP(),
	})
	if err != nil {
		jsonError(c, 400, "billing_or_pool_error", err.Error())
		return
	}

	results := h.waitTask(c, t.TaskID, 10*time.Minute)
	out := []gin.H{}
	for _, r := range results {
		row := gin.H{"url": r.URL}
		if r.DurationMs != nil {
			row["duration_ms"] = *r.DurationMs
		}
		out = append(out, row)
	}
	c.JSON(200, gin.H{
		"created": time.Now().Unix(),
		"data":    out,
		"task_id": t.TaskID,
	})
}

// waitTask 简易轮询任务（开发期）；生产应换为 WS / SSE / 直接异步返回 task_id。
func (h *OpenAIHandler) waitTask(c *gin.Context, taskID string, timeout time.Duration) []*model.GenerationResult {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		t, err := h.repo.GetByTaskID(c.Request.Context(), taskID)
		if err == nil && (t.Status == model.GenStatusSucceeded || t.Status == model.GenStatusFailed || t.Status == model.GenStatusRefunded) {
			items, _ := h.repo.ListResultsByTask(c.Request.Context(), taskID)
			return items
		}
		select {
		case <-c.Request.Context().Done():
			return nil
		case <-time.After(500 * time.Millisecond):
		}
	}
	return nil
}

func jsonError(c *gin.Context, status int, kind, msg string) {
	c.AbortWithStatusJSON(status, gin.H{
		"error": gin.H{
			"type":    kind,
			"code":    strconv.Itoa(status),
			"message": msg,
		},
	})
}
