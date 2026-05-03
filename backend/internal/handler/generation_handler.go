// Package handler 用户端生成任务 handler。
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kleinai/backend/internal/dto"
	"github.com/kleinai/backend/internal/middleware"
	"github.com/kleinai/backend/internal/model"
	"github.com/kleinai/backend/internal/provider"
	"github.com/kleinai/backend/internal/repo"
	"github.com/kleinai/backend/internal/service"
	"github.com/kleinai/backend/pkg/crypto"
	"github.com/kleinai/backend/pkg/errcode"
	"github.com/kleinai/backend/pkg/response"
)

// GenerationHandler 生成任务 handler。
type GenerationHandler struct {
	svc     *service.GenerationService
	chatSvc *service.ChatService
	repo    *repo.GenerationRepo
	accRepo *repo.AccountRepo
	cfg     *service.SystemConfigService
	aes     *crypto.AESGCM
}

// NewGenerationHandler 构造。
func NewGenerationHandler(svc *service.GenerationService, chatSvc *service.ChatService, r *repo.GenerationRepo, accRepo *repo.AccountRepo, cfg *service.SystemConfigService, aes *crypto.AESGCM) *GenerationHandler {
	return &GenerationHandler{svc: svc, chatSvc: chatSvc, repo: r, accRepo: accRepo, cfg: cfg, aes: aes}
}

type publicModelResp struct {
	ModelCode        string `json:"model_code"`
	Name             string `json:"name"`
	Kind             string `json:"kind"`
	Provider         string `json:"provider"`
	UpstreamModel    string `json:"upstream_model,omitempty"`
	UnitPoints       int64  `json:"unit_points"`
	InputUnitPoints  int64  `json:"input_unit_points,omitempty"`
	OutputUnitPoints int64  `json:"output_unit_points,omitempty"`
	Enabled          bool   `json:"enabled"`
}

// Models GET /api/v1/models
func (h *GenerationHandler) Models(c *gin.Context) {
	response.OK(c, gin.H{"list": h.publicModels(c.Request.Context())})
}

// CachedAsset GET /api/v1/gen/cached/*path
func (h *GenerationHandler) CachedAsset(c *gin.Context) {
	p := strings.TrimLeft(c.Param("path"), "/")
	if p == "" || strings.Contains(p, "..") || strings.HasPrefix(p, "/") {
		response.Fail(c, errcode.InvalidParam.WithMsg("invalid asset path"))
		return
	}
	root := strings.TrimSpace(os.Getenv("KLEIN_STORAGE_ROOT"))
	if root == "" {
		root = "/app/storage/public"
	}
	c.Header("Cache-Control", "public, max-age=86400")
	c.File(filepath.Join(root, filepath.FromSlash(p)))
}

// CreateImage POST /api/v1/gen/image
func (h *GenerationHandler) CreateImage(c *gin.Context) {
	var req dto.CreateImageReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.InvalidParam.Wrap(err))
		return
	}
	uid := middleware.MustUID(c)

	params := req.Params
	if params == nil {
		params = map[string]any{}
	}
	if req.Ratio != "" {
		params["ratio"] = req.Ratio
		params["aspect_ratio"] = req.Ratio
	}
	if req.Quality != "" {
		params["quality"] = req.Quality
	}

	mode := req.Mode
	if mode == "" {
		if len(req.RefAssets) > 0 {
			mode = "i2i"
		} else {
			mode = "t2i"
		}
	}
	count := req.Count
	if count <= 0 {
		count = 1
	}

	t, err := h.svc.Create(c.Request.Context(), service.CreateRequest{
		UserID:    uid,
		Kind:      provider.KindImage,
		Mode:      provider.Mode(mode),
		ModelCode: req.ModelCode,
		Provider:  model.ProviderGPT,
		Prompt:    req.Prompt,
		NegPrompt: req.NegPrompt,
		Params:    params,
		RefAssets: req.RefAssets,
		Count:     count,
		IdemKey:   c.GetHeader("Idempotency-Key"),
		ClientIP:  c.ClientIP(),
	})
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, taskToResp(t, nil))
}

// CreateVideo POST /api/v1/gen/video
func (h *GenerationHandler) CreateVideo(c *gin.Context) {
	var req dto.CreateVideoReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.InvalidParam.Wrap(err))
		return
	}
	uid := middleware.MustUID(c)

	params := req.Params
	if params == nil {
		params = map[string]any{}
	}
	if req.Ratio != "" {
		params["ratio"] = req.Ratio
		params["aspect_ratio"] = req.Ratio
	}
	if req.Quality != "" {
		params["quality"] = req.Quality
	}
	if req.Duration > 0 {
		params["duration"] = float64(normalizeVideoDuration(req.Duration))
	}

	mode := req.Mode
	if mode == "" {
		if len(req.RefAssets) > 0 {
			mode = "i2v"
		} else {
			mode = "t2v"
		}
	}

	t, err := h.svc.Create(c.Request.Context(), service.CreateRequest{
		UserID:    uid,
		Kind:      provider.KindVideo,
		Mode:      provider.Mode(mode),
		ModelCode: req.ModelCode,
		Provider:  model.ProviderGROK,
		Prompt:    req.Prompt,
		Params:    params,
		RefAssets: req.RefAssets,
		Count:     1,
		IdemKey:   c.GetHeader("Idempotency-Key"),
		ClientIP:  c.ClientIP(),
	})
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, taskToResp(t, nil))
}

// CreateText POST /api/v1/gen/text
func (h *GenerationHandler) CreateText(c *gin.Context) {
	var req dto.CreateTextReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.InvalidParam.Wrap(err))
		return
	}
	if h.chatSvc == nil {
		response.Fail(c, errcode.ResourceMissing.WithMsg("文字创作服务未启用"))
		return
	}
	if strings.TrimSpace(req.ModelCode) == "" {
		req.ModelCode = "grok-4.20-fast"
	}
	if req.MaxTokens <= 0 {
		req.MaxTokens = 1200
	}
	content := any(req.Prompt)
	if len(req.Images) > 0 {
		parts := []map[string]any{{"type": "text", "text": req.Prompt}}
		for _, u := range req.Images {
			if strings.TrimSpace(u) != "" {
				parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]any{"url": strings.TrimSpace(u)}})
			}
		}
		content = parts
	}
	raw, status, err := h.chatSvc.Complete(c.Request.Context(), service.ChatCallRequest{
		UserID:   middleware.MustUID(c),
		ClientIP: c.ClientIP(),
		IdemKey:  c.GetHeader("Idempotency-Key"),
		Body: map[string]any{
			"model":      req.ModelCode,
			"messages":   []map[string]any{{"role": "user", "content": content}},
			"max_tokens": req.MaxTokens,
		},
	})
	if err != nil {
		response.Fail(c, err)
		return
	}
	if status >= 400 {
		response.Fail(c, errcode.GPTUnavailable.WithMsg(string(raw)))
		return
	}
	response.OK(c, parseTextGenerationResp(raw, req.ModelCode))
}

// Get GET /api/v1/gen/tasks/:task_id
func (h *GenerationHandler) Get(c *gin.Context) {
	id := c.Param("task_id")
	uid := middleware.MustUID(c)
	t, err := h.repo.GetByTaskID(c.Request.Context(), id)
	if err != nil || t.UserID != uid {
		response.Fail(c, errcode.ResourceMissing)
		return
	}
	results, _ := h.repo.ListResultsByTask(c.Request.Context(), id)
	response.OK(c, taskToResp(t, results))
}

// List GET /api/v1/gen/history?kind=image|video&page=&page_size=
func (h *GenerationHandler) List(c *gin.Context) {
	uid := middleware.MustUID(c)
	kind := c.Query("kind")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if h.svc != nil {
		h.svc.ReapStaleTasks(c.Request.Context(), uid)
	}
	items, total, err := h.repo.ListByUser(c.Request.Context(), uid, kind, page, pageSize)
	if err != nil {
		response.Fail(c, errcode.DBError.Wrap(err))
		return
	}
	out := make([]*dto.GenerationTaskResp, 0, len(items))
	for _, t := range items {
		results, _ := h.repo.ListResultsByTask(c.Request.Context(), t.TaskID)
		out = append(out, taskToResp(t, results))
	}
	response.Page(c, out, total, page, pageSize)
}

// DeleteHistory DELETE /api/v1/gen/history?scope=all|before_3d|before_7d|failed
func (h *GenerationHandler) DeleteHistory(c *gin.Context) {
	uid := middleware.MustUID(c)
	scope := strings.ToLower(strings.TrimSpace(c.DefaultQuery("scope", "all")))
	var (
		deleted int64
		err     error
	)
	switch scope {
	case "all":
		deleted, err = h.repo.SoftDeleteByUser(c.Request.Context(), uid, false)
	case "before_3d":
		deleted, err = h.repo.SoftDeleteByUserBefore(c.Request.Context(), uid, time.Now().UTC().AddDate(0, 0, -3))
	case "before_7d":
		deleted, err = h.repo.SoftDeleteByUserBefore(c.Request.Context(), uid, time.Now().UTC().AddDate(0, 0, -7))
	case "failed":
		deleted, err = h.repo.SoftDeleteByUser(c.Request.Context(), uid, true)
	default:
		response.Fail(c, errcode.InvalidParam.WithMsg("scope must be all, before_3d, before_7d or failed"))
		return
	}
	if err != nil {
		response.Fail(c, errcode.DBError.Wrap(err))
		return
	}
	response.OK(c, gin.H{"deleted": deleted})
}

// Asset GET /api/v1/gen/assets/:task_id/:seq?thumb=1
func (h *GenerationHandler) Asset(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("task_id"))
	seq, _ := strconv.Atoi(c.Param("seq"))
	t, err := h.repo.GetByTaskID(c.Request.Context(), taskID)
	if err != nil {
		response.Fail(c, errcode.ResourceMissing)
		return
	}
	result, err := h.repo.GetResultByTaskSeq(c.Request.Context(), taskID, seq)
	if err != nil {
		response.Fail(c, errcode.ResourceMissing)
		return
	}
	rawURL := result.URL
	if c.Query("thumb") == "1" {
		if result.ThumbURL != nil && *result.ThumbURL != "" {
			rawURL = *result.ThumbURL
		} else if derived := deriveGrokPreviewImageURL(result.URL); derived != "" {
			rawURL = derived
		}
	}
	target := normalizeGrokAssetURL(rawURL)
	if target == "" {
		response.Fail(c, errcode.ResourceMissing)
		return
	}
	cookie, err := h.grokCookieForTask(c.Request.Context(), t)
	if err != nil {
		response.Fail(c, errcode.GPTUnavailable.WithMsg("资源下载凭证不可用"))
		return
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, target, nil)
	if err != nil {
		response.Fail(c, errcode.InvalidParam.Wrap(err))
		return
	}
	req.Header.Set("Cookie", cookie)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://grok.com/")
	req.Header.Set("Accept", "*/*")
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		response.Fail(c, errcode.GPTUnavailable.Wrap(err))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		response.Fail(c, errcode.GPTUnavailable.WithMsg(fmt.Sprintf("资源下载失败 HTTP %d", resp.StatusCode)))
		return
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Header("Content-Type", contentType)
	c.Header("Cache-Control", "private, max-age=300")
	if disp := assetDisposition(rawURL); disp != "" {
		c.Header("Content-Disposition", disp)
	}
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, resp.Body)
}

// === helpers ===

func parseTextGenerationResp(raw []byte, fallbackModel string) *dto.TextGenerationResp {
	var obj struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	_ = json.Unmarshal(raw, &obj)
	content := ""
	if len(obj.Choices) > 0 {
		content = obj.Choices[0].Message.Content
	}
	if obj.Model == "" {
		obj.Model = fallbackModel
	}
	return &dto.TextGenerationResp{
		ID:               obj.ID,
		ModelCode:        obj.Model,
		Content:          content,
		PromptTokens:     obj.Usage.PromptTokens,
		CompletionTokens: obj.Usage.CompletionTokens,
		TotalTokens:      obj.Usage.TotalTokens,
	}
}

func taskToResp(t *model.GenerationTask, results []*model.GenerationResult) *dto.GenerationTaskResp {
	r := &dto.GenerationTaskResp{
		TaskID:     t.TaskID,
		Kind:       t.Kind,
		Status:     t.Status,
		Progress:   t.Progress,
		ModelCode:  t.ModelCode,
		Prompt:     t.Prompt,
		CostPoints: t.CostPoints,
		CreatedAt:  t.CreatedAt.Unix(),
	}
	if t.Error != nil {
		r.Error = *t.Error
	}
	for _, gr := range results {
		row := dto.GenerationResultResp{URL: generationResultURL(t.TaskID, int(gr.Seq), gr.URL, false)}
		if gr.ThumbURL != nil {
			row.ThumbURL = generationResultURL(t.TaskID, int(gr.Seq), *gr.ThumbURL, true)
		} else if t.Kind == string(provider.KindVideo) {
			if derived := deriveGrokPreviewImageURL(gr.URL); derived != "" {
				row.ThumbURL = generationResultURL(t.TaskID, int(gr.Seq), derived, true)
			}
		}
		if gr.Width != nil {
			row.Width = *gr.Width
		}
		if gr.Height != nil {
			row.Height = *gr.Height
		}
		if gr.DurationMs != nil {
			row.DurationMs = *gr.DurationMs
		}
		r.Results = append(r.Results, row)
	}
	return r
}

func normalizeVideoDuration(sec int) int {
	for _, v := range []int{6, 10} {
		if sec <= v {
			return v
		}
	}
	return 10
}

func generationAssetURL(taskID string, seq int, thumb bool) string {
	u := fmt.Sprintf("/api/v1/gen/assets/%s/%d", url.PathEscape(taskID), seq)
	if thumb {
		u += "?thumb=1"
	}
	return u
}

func generationResultURL(taskID string, seq int, rawURL string, thumb bool) string {
	v := strings.TrimSpace(rawURL)
	if v == "" {
		return ""
	}
	if strings.HasPrefix(v, "/api/v1/gen/cached/") || strings.HasPrefix(v, "data:") {
		return v
	}
	if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
		if !strings.Contains(v, "assets.grok.com") {
			return v
		}
		return generationAssetURL(taskID, seq, thumb)
	}
	return generationAssetURL(taskID, seq, thumb)
}

func deriveGrokPreviewImageURL(videoURL string) string {
	v := strings.TrimSpace(videoURL)
	if v == "" {
		return ""
	}
	lower := strings.ToLower(v)
	if !strings.Contains(lower, "assets.grok.com") && !strings.Contains(lower, "generated_video") && !strings.Contains(lower, "/generated/") {
		return ""
	}
	for _, marker := range []string{"/generated_video.mp4", "/generated_video.webm", "/generated_video"} {
		if idx := strings.LastIndex(lower, marker); idx >= 0 {
			return v[:idx] + "/preview_image.jpg"
		}
	}
	if strings.HasSuffix(lower, ".mp4") || strings.HasSuffix(lower, ".webm") {
		if idx := strings.LastIndex(v, "/"); idx >= 0 {
			return v[:idx] + "/preview_image.jpg"
		}
	}
	return ""
}

func normalizeGrokAssetURL(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") || strings.HasPrefix(v, "data:") || strings.HasPrefix(v, "/api/") {
		return v
	}
	if strings.HasPrefix(v, "/") {
		v = strings.TrimLeft(v, "/")
	}
	return "https://assets.grok.com/" + v
}

func (h *GenerationHandler) grokCookieForTask(ctx context.Context, t *model.GenerationTask) (string, error) {
	if t.AccountID == nil || h.accRepo == nil || h.aes == nil {
		return "", fmt.Errorf("missing account")
	}
	acc, err := h.accRepo.GetByID(ctx, *t.AccountID)
	if err != nil {
		return "", err
	}
	plain, err := h.aes.Decrypt(acc.CredentialEnc)
	if err != nil {
		return "", err
	}
	cred := strings.TrimSpace(string(plain))
	if strings.Contains(cred, "=") {
		if !strings.Contains(cred, "sso-rw=") {
			token := extractSSOValue(cred)
			if token != "" {
				cred = strings.TrimRight(cred, "; ") + "; sso-rw=" + token
			}
		}
		return cred, nil
	}
	return "sso=" + cred + "; sso-rw=" + cred, nil
}

func extractSSOValue(cookie string) string {
	for _, part := range strings.Split(cookie, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "sso=") {
			return strings.TrimPrefix(part, "sso=")
		}
	}
	return strings.TrimSpace(cookie)
}

func assetDisposition(rawURL string) string {
	lower := strings.ToLower(rawURL)
	name := "asset"
	if i := strings.LastIndex(rawURL, "/"); i >= 0 && i+1 < len(rawURL) {
		name = rawURL[i+1:]
	}
	if strings.Contains(lower, ".mp4") || strings.Contains(lower, "generated_video") {
		return fmt.Sprintf(`inline; filename="%s"`, name)
	}
	return fmt.Sprintf(`inline; filename="%s"`, name)
}

func (h *GenerationHandler) publicModels(ctx context.Context) []publicModelResp {
	raw := ""
	if h.cfg != nil {
		raw = h.cfg.GetString(ctx, "billing.model_prices", "")
	}
	var rows []publicModelResp
	seen := map[string]bool{}
	if raw != "" {
		var stored []struct {
			ModelCode        string `json:"model_code"`
			Name             string `json:"name"`
			Kind             string `json:"kind"`
			Provider         string `json:"provider"`
			UpstreamModel    string `json:"upstream_model"`
			UnitPoints       int64  `json:"unit_points"`
			InputUnitPoints  int64  `json:"input_unit_points"`
			OutputUnitPoints int64  `json:"output_unit_points"`
			Enabled          *bool  `json:"enabled"`
		}
		if err := json.Unmarshal([]byte(raw), &stored); err == nil {
			for _, row := range stored {
				if row.ModelCode == "" || row.Kind == "" {
					continue
				}
				seen[row.ModelCode] = true
				enabled := true
				if row.Enabled != nil {
					enabled = *row.Enabled
				}
				if !enabled {
					continue
				}
				rows = append(rows, publicModelResp{
					ModelCode:        row.ModelCode,
					Name:             fallbackString(row.Name, row.ModelCode),
					Kind:             row.Kind,
					Provider:         row.Provider,
					UpstreamModel:    row.UpstreamModel,
					UnitPoints:       row.UnitPoints,
					InputUnitPoints:  row.InputUnitPoints,
					OutputUnitPoints: row.OutputUnitPoints,
					Enabled:          true,
				})
			}
		}
	}
	for _, row := range defaultPublicModels() {
		if !seen[row.ModelCode] {
			rows = append(rows, row)
		}
	}
	return rows
}

func defaultPublicModels() []publicModelResp {
	return []publicModelResp{
		{ModelCode: "grok-4.20-fast", Name: "Grok Fast", Kind: "text", Provider: "grok", UpstreamModel: "grok-4.20-fast", InputUnitPoints: 100, OutputUnitPoints: 300, Enabled: true},
		{ModelCode: "grok-4.20-auto", Name: "Grok Auto", Kind: "text", Provider: "grok", UpstreamModel: "grok-4.20-auto", InputUnitPoints: 150, OutputUnitPoints: 450, Enabled: true},
		{ModelCode: "grok-4.20-expert", Name: "Grok Expert", Kind: "text", Provider: "grok", UpstreamModel: "grok-4.20-expert", InputUnitPoints: 200, OutputUnitPoints: 600, Enabled: true},
		{ModelCode: "grok-4.20-heavy", Name: "Grok Heavy", Kind: "text", Provider: "grok", UpstreamModel: "grok-4.20-heavy", InputUnitPoints: 400, OutputUnitPoints: 1200, Enabled: true},
		{ModelCode: "gpt-image-2", Name: "GPT Image 2", Kind: "image", Provider: "gpt", UpstreamModel: "gpt-image-2", UnitPoints: 0, Enabled: true},
		{ModelCode: "grok-imagine-video", Name: "Grok Imagine 视频", Kind: "video", Provider: "grok", UpstreamModel: "grok-imagine-video", UnitPoints: 2000, Enabled: true},
	}
}

func fallbackString(v, fallback string) string {
	if strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}
