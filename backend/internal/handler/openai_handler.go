// Package handler OpenAI / NewAPI compatible downstream protocol handlers.
package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kleinai/backend/internal/middleware"
	"github.com/kleinai/backend/internal/model"
	"github.com/kleinai/backend/internal/provider"
	grokweb "github.com/kleinai/backend/internal/provider/grok"
	"github.com/kleinai/backend/internal/repo"
	"github.com/kleinai/backend/internal/service"
)

// OpenAIHandler serves /v1 compatible downstream APIs.
type OpenAIHandler struct {
	svc     *service.GenerationService
	chatSvc *service.ChatService
	repo    *repo.GenerationRepo
}

// NewOpenAIHandler constructs OpenAIHandler.
func NewOpenAIHandler(svc *service.GenerationService, chatSvc *service.ChatService, r *repo.GenerationRepo) *OpenAIHandler {
	return &OpenAIHandler{svc: svc, chatSvc: chatSvc, repo: r}
}

type modelItem struct {
	ID       string         `json:"id"`
	Object   string         `json:"object"`
	OwnedBy  string         `json:"owned_by"`
	Kind     string         `json:"kind,omitempty"`
	Endpoint string         `json:"endpoint,omitempty"`
	Meta     map[string]any `json:"meta,omitempty"`
}

// Models GET /v1/models.
func (h *OpenAIHandler) Models(c *gin.Context) {
	data := []modelItem{
		{ID: "gpt-4o-mini", Object: "model", OwnedBy: "kleinai", Kind: "text", Endpoint: "/v1/chat/completions"},
		{ID: "gpt-image-2", Object: "model", OwnedBy: "openai", Kind: "image", Endpoint: "/v1/images/generations", Meta: gin.H{"edits": true, "mode": "responses_image_generation"}},
		{ID: "grok-imagine-video", Object: "model", OwnedBy: "grok", Kind: "video", Endpoint: "/v1/video/generations", Meta: gin.H{"modes": []string{"text_to_video", "image_to_video", "multi_image_to_video"}}},
		{ID: "vid-v1", Object: "model", OwnedBy: "kleinai", Kind: "video", Endpoint: "/v1/video/generations", Meta: gin.H{"alias_of": "grok-imagine-video"}},
		{ID: "vid-i2v", Object: "model", OwnedBy: "kleinai", Kind: "video", Endpoint: "/v1/video/generations", Meta: gin.H{"alias_of": "grok-imagine-video"}},
	}
	for _, id := range grokweb.ChatModelIDs() {
		data = append(data, modelItem{ID: id, Object: "model", OwnedBy: "grok", Kind: "text", Endpoint: "/v1/chat/completions"})
	}
	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   data,
	})
}

// ChatCompletions POST /v1/chat/completions.
func (h *OpenAIHandler) ChatCompletions(c *gin.Context) {
	if !middleware.APIKeyScopeAllow(c, "chat") {
		jsonError(c, http.StatusForbidden, "scope_not_allowed", "current api key does not allow chat completions")
		return
	}
	k := middleware.APIKeyFromCtx(c)
	if k == nil {
		jsonError(c, http.StatusUnauthorized, "invalid_api_key", "api key required")
		return
	}
	var body map[string]any
	if err := c.ShouldBindJSON(&body); err != nil {
		jsonError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if _, ok := body["messages"]; !ok {
		jsonError(c, http.StatusBadRequest, "invalid_request_error", "messages is required")
		return
	}
	req := service.ChatCallRequest{
		UserID:   k.UserID,
		APIKeyID: &k.ID,
		ClientIP: c.ClientIP(),
		IdemKey:  c.GetHeader("Idempotency-Key"),
		Body:     body,
	}
	if stream, _ := body["stream"].(bool); stream {
		if err := h.chatSvc.Stream(c.Request.Context(), req, c.Writer); err != nil {
			jsonError(c, http.StatusBadGateway, "upstream_error", err.Error())
		}
		return
	}
	raw, status, err := h.chatSvc.Complete(c.Request.Context(), req)
	if err != nil {
		jsonError(c, status, "chat_completion_failed", err.Error())
		return
	}
	c.Data(status, "application/json; charset=utf-8", raw)
}

type imageReq struct {
	Model          string         `json:"model"`
	Prompt         string         `json:"prompt"`
	N              int            `json:"n"`
	Count          int            `json:"count"`
	Size           string         `json:"size"`
	Quality        string         `json:"quality"`
	Style          string         `json:"style"`
	ResponseFormat string         `json:"response_format"`
	Image          string         `json:"image"`
	Images         []string       `json:"images"`
	RefAssets      []string       `json:"ref_assets"`
	Async          bool           `json:"async"`
	CallbackURL    string         `json:"callback_url"`
	Params         map[string]any `json:"params"`
}

// ImageGenerations POST /v1/images/generations.
func (h *OpenAIHandler) ImageGenerations(c *gin.Context) {
	h.createImage(c, false)
}

// ImageEdits POST /v1/images/edits.
func (h *OpenAIHandler) ImageEdits(c *gin.Context) {
	h.createImage(c, true)
}

func (h *OpenAIHandler) createImage(c *gin.Context, edit bool) {
	if !middleware.APIKeyScopeAllow(c, "image") {
		jsonError(c, http.StatusForbidden, "scope_not_allowed", "current api key does not allow image generation")
		return
	}
	req, err := bindImageReq(c)
	if err != nil {
		jsonError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		jsonError(c, http.StatusBadRequest, "invalid_request_error", "prompt is required")
		return
	}
	if req.Model == "" {
		req.Model = "gpt-image-2"
	}
	count := req.N
	if count <= 0 {
		count = req.Count
	}
	if count <= 0 {
		count = 1
	}
	if count > 4 {
		jsonError(c, http.StatusBadRequest, "invalid_request_error", "n/count must be less than or equal to 4")
		return
	}

	refs := collectRefs(req.Image, req.Images, req.RefAssets)
	if edit && len(refs) == 0 {
		jsonError(c, http.StatusBadRequest, "invalid_request_error", "image is required for image edits")
		return
	}
	mode := provider.ModeT2I
	if len(refs) > 0 {
		mode = provider.ModeI2I
	}
	params := mergeParams(req.Params, gin.H{
		"size":            req.Size,
		"quality":         req.Quality,
		"style":           req.Style,
		"response_format": req.ResponseFormat,
		"callback_url":    req.CallbackURL,
	})
	if edit {
		params["operation"] = "edit"
	}

	t, ok := h.createTask(c, service.CreateRequest{
		Kind:      provider.KindImage,
		Mode:      mode,
		ModelCode: req.Model,
		Provider:  model.ProviderGPT,
		Prompt:    req.Prompt,
		Params:    params,
		RefAssets: refs,
		Count:     count,
	})
	if !ok {
		return
	}
	if req.Async {
		c.JSON(http.StatusOK, taskEnvelope(t, nil))
		return
	}
	h.respondTaskResult(c, t, 60*time.Second)
}

type videoReq struct {
	Model       string         `json:"model"`
	Prompt      string         `json:"prompt"`
	N           int            `json:"n"`
	Duration    int            `json:"duration"`
	Size        string         `json:"size"`
	Ratio       string         `json:"ratio"`
	AspectRatio string         `json:"aspect_ratio"`
	Quality     string         `json:"quality"`
	FPS         int            `json:"fps"`
	Image       string         `json:"image"`
	Images      []string       `json:"images"`
	RefAssets   []string       `json:"ref_assets"`
	Async       *bool          `json:"async"`
	CallbackURL string         `json:"callback_url"`
	Params      map[string]any `json:"params"`
}

// VideoGenerations POST /v1/video/generations.
func (h *OpenAIHandler) VideoGenerations(c *gin.Context) {
	if !middleware.APIKeyScopeAllow(c, "video") {
		jsonError(c, http.StatusForbidden, "scope_not_allowed", "current api key does not allow video generation")
		return
	}
	var req videoReq
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		jsonError(c, http.StatusBadRequest, "invalid_request_error", "prompt is required")
		return
	}
	if req.Model == "" {
		req.Model = "grok-imagine-video"
	}
	if req.N <= 0 {
		req.N = 1
	}
	if req.N > 4 {
		jsonError(c, http.StatusBadRequest, "invalid_request_error", "n must be less than or equal to 4")
		return
	}
	refs := collectRefs(req.Image, req.Images, req.RefAssets)
	mode := provider.ModeT2V
	if len(refs) > 0 {
		mode = provider.ModeI2V
	}
	aspect := req.AspectRatio
	if aspect == "" {
		aspect = req.Ratio
	}
	params := mergeParams(req.Params, gin.H{
		"duration":     float64(req.Duration),
		"size":         req.Size,
		"aspect_ratio": aspect,
		"quality":      req.Quality,
		"fps":          req.FPS,
		"callback_url": req.CallbackURL,
	})

	t, ok := h.createTask(c, service.CreateRequest{
		Kind:      provider.KindVideo,
		Mode:      mode,
		ModelCode: grokweb.NormalizeVideoModel(req.Model),
		Provider:  model.ProviderGROK,
		Prompt:    req.Prompt,
		Params:    params,
		RefAssets: refs,
		Count:     req.N,
	})
	if !ok {
		return
	}
	async := true
	if req.Async != nil {
		async = *req.Async
	}
	if async {
		c.JSON(http.StatusOK, taskEnvelope(t, nil))
		return
	}
	h.respondTaskResult(c, t, 10*time.Minute)
}

// GetImageTask GET /v1/images/generations/:task_id.
func (h *OpenAIHandler) GetImageTask(c *gin.Context) {
	h.getTask(c, provider.KindImage)
}

// GetVideoTask GET /v1/video/generations/:task_id.
func (h *OpenAIHandler) GetVideoTask(c *gin.Context) {
	h.getTask(c, provider.KindVideo)
}

func (h *OpenAIHandler) getTask(c *gin.Context, kind provider.Kind) {
	taskID := strings.TrimSpace(c.Param("task_id"))
	t, err := h.repo.GetByTaskID(c.Request.Context(), taskID)
	if err != nil {
		jsonError(c, http.StatusNotFound, "not_found", "task not found")
		return
	}
	k := middleware.APIKeyFromCtx(c)
	if k == nil || t.UserID != k.UserID || t.Kind != string(kind) {
		jsonError(c, http.StatusNotFound, "not_found", "task not found")
		return
	}
	results, _ := h.repo.ListResultsByTask(c.Request.Context(), t.TaskID)
	c.JSON(http.StatusOK, taskEnvelope(t, results))
}

func (h *OpenAIHandler) createTask(c *gin.Context, req service.CreateRequest) (*model.GenerationTask, bool) {
	k := middleware.APIKeyFromCtx(c)
	if k == nil {
		jsonError(c, http.StatusUnauthorized, "invalid_api_key", "api key required")
		return nil, false
	}
	req.UserID = k.UserID
	req.APIKeyID = &k.ID
	req.IdemKey = c.GetHeader("Idempotency-Key")
	req.ClientIP = c.ClientIP()
	t, err := h.svc.Create(c.Request.Context(), req)
	if err != nil {
		jsonError(c, http.StatusBadRequest, "billing_or_pool_error", err.Error())
		return nil, false
	}
	return t, true
}

func (h *OpenAIHandler) respondTaskResult(c *gin.Context, t *model.GenerationTask, timeout time.Duration) {
	fresh, results := h.waitTask(c, t.TaskID, timeout)
	if fresh == nil {
		c.JSON(http.StatusAccepted, taskEnvelope(t, nil))
		return
	}
	if fresh.Status == model.GenStatusFailed || fresh.Status == model.GenStatusRefunded {
		msg := "generation failed"
		if fresh.Error != nil && *fresh.Error != "" {
			msg = *fresh.Error
		}
		jsonError(c, http.StatusBadRequest, "generation_failed", msg)
		return
	}
	if fresh.Kind == string(provider.KindVideo) {
		c.JSON(http.StatusOK, videoResultEnvelope(fresh, results))
		return
	}
	c.JSON(http.StatusOK, imageResultEnvelope(fresh, results))
}

func (h *OpenAIHandler) waitTask(c *gin.Context, taskID string, timeout time.Duration) (*model.GenerationTask, []*model.GenerationResult) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		t, err := h.repo.GetByTaskID(c.Request.Context(), taskID)
		if err == nil && (t.Status == model.GenStatusSucceeded || t.Status == model.GenStatusFailed || t.Status == model.GenStatusRefunded) {
			items, _ := h.repo.ListResultsByTask(c.Request.Context(), taskID)
			return t, items
		}
		select {
		case <-c.Request.Context().Done():
			return nil, nil
		case <-time.After(500 * time.Millisecond):
		}
	}
	return nil, nil
}

func bindImageReq(c *gin.Context) (*imageReq, error) {
	ct := c.GetHeader("Content-Type")
	if strings.Contains(strings.ToLower(ct), "multipart/form-data") {
		if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
			return nil, err
		}
		req := &imageReq{
			Model:          c.PostForm("model"),
			Prompt:         c.PostForm("prompt"),
			Size:           c.PostForm("size"),
			Quality:        c.PostForm("quality"),
			Style:          c.PostForm("style"),
			ResponseFormat: c.PostForm("response_format"),
			Image:          c.PostForm("image"),
			CallbackURL:    c.PostForm("callback_url"),
		}
		req.N, _ = strconv.Atoi(c.DefaultPostForm("n", "1"))
		req.Count, _ = strconv.Atoi(c.DefaultPostForm("count", "0"))
		req.Async = parseBool(c.PostForm("async"))
		req.Images = c.PostFormArray("images")
		req.RefAssets = c.PostFormArray("ref_assets")
		if req.Image == "" && len(req.Images) == 0 {
			if _, _, err := c.Request.FormFile("image"); err == nil {
				return nil, fmt.Errorf("multipart file upload is not supported yet; use image URL or images[] URL")
			}
		}
		return req, nil
	}
	var req imageReq
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, err
	}
	return &req, nil
}

func imageResultEnvelope(t *model.GenerationTask, results []*model.GenerationResult) gin.H {
	data := make([]gin.H, 0, len(results))
	for _, r := range results {
		row := gin.H{"url": normalizeOpenAIResultURL(r.URL)}
		if r.Width != nil {
			row["width"] = *r.Width
		}
		if r.Height != nil {
			row["height"] = *r.Height
		}
		data = append(data, row)
	}
	return gin.H{
		"created": t.CreatedAt.Unix(),
		"data":    data,
		"task_id": t.TaskID,
		"usage":   usageEnvelope(t),
	}
}

func videoResultEnvelope(t *model.GenerationTask, results []*model.GenerationResult) gin.H {
	data := make([]gin.H, 0, len(results))
	for _, r := range results {
		row := gin.H{"url": normalizeOpenAIResultURL(r.URL)}
		if r.ThumbURL != nil {
			row["cover_url"] = normalizeOpenAIResultURL(*r.ThumbURL)
		}
		if r.DurationMs != nil {
			row["duration_ms"] = *r.DurationMs
		}
		if r.Width != nil {
			row["width"] = *r.Width
		}
		if r.Height != nil {
			row["height"] = *r.Height
		}
		data = append(data, row)
	}
	return gin.H{
		"id":      t.TaskID,
		"object":  "video.generation",
		"created": t.CreatedAt.Unix(),
		"model":   t.ModelCode,
		"data":    data,
		"usage":   usageEnvelope(t),
	}
}

func taskEnvelope(t *model.GenerationTask, results []*model.GenerationResult) gin.H {
	out := gin.H{
		"id":       t.TaskID,
		"task_id":  t.TaskID,
		"object":   t.Kind + ".generation.task",
		"status":   statusName(t.Status),
		"progress": t.Progress,
		"created":  t.CreatedAt.Unix(),
		"model":    t.ModelCode,
		"kind":     t.Kind,
		"mode":     t.Mode,
		"usage":    usageEnvelope(t),
		"error":    nil,
	}
	if t.Error != nil && *t.Error != "" {
		out["error"] = gin.H{"message": *t.Error}
	}
	if len(results) > 0 {
		if t.Kind == string(provider.KindVideo) {
			out["result"] = videoResultEnvelope(t, results)
		} else {
			out["result"] = imageResultEnvelope(t, results)
		}
	}
	return out
}

func normalizeOpenAIResultURL(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") || strings.HasPrefix(v, "data:") || strings.HasPrefix(v, "/api/") {
		return v
	}
	return "https://assets.grok.com/" + strings.TrimLeft(v, "/")
}

func usageEnvelope(t *model.GenerationTask) gin.H {
	return gin.H{
		"total_cost":   t.CostPoints,
		"total_points": float64(t.CostPoints) / 100,
	}
}

func statusName(status int8) string {
	switch status {
	case model.GenStatusPending:
		return "queued"
	case model.GenStatusRunning:
		return "running"
	case model.GenStatusSucceeded:
		return "succeeded"
	case model.GenStatusFailed:
		return "failed"
	case model.GenStatusRefunded:
		return "refunded"
	default:
		return "unknown"
	}
}

func collectRefs(one string, many []string, refs []string) []string {
	out := make([]string, 0, 1+len(many)+len(refs))
	if strings.TrimSpace(one) != "" {
		out = append(out, strings.TrimSpace(one))
	}
	for _, v := range many {
		if strings.TrimSpace(v) != "" {
			out = append(out, strings.TrimSpace(v))
		}
	}
	for _, v := range refs {
		if strings.TrimSpace(v) != "" {
			out = append(out, strings.TrimSpace(v))
		}
	}
	return out
}

func mergeParams(base map[string]any, vals map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range vals {
		switch x := v.(type) {
		case string:
			if strings.TrimSpace(x) != "" {
				out[k] = x
			}
		case int:
			if x > 0 {
				out[k] = x
			}
		case float64:
			if x > 0 {
				out[k] = x
			}
		default:
			if v != nil {
				out[k] = v
			}
		}
	}
	return out
}

func parseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func jsonError(c *gin.Context, status int, kind, msg string) {
	c.AbortWithStatusJSON(status, gin.H{
		"error": gin.H{
			"type":    kind,
			"code":    kind,
			"message": msg,
		},
	})
}
