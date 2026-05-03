package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kleinai/backend/internal/dto"
	"github.com/kleinai/backend/internal/model"
	"github.com/kleinai/backend/internal/repo"
	"github.com/kleinai/backend/pkg/crypto"
	"github.com/kleinai/backend/pkg/errcode"
	"github.com/kleinai/backend/pkg/response"
)

type AdminLogHandler struct {
	gen *repo.GenerationRepo
	acc *repo.AccountRepo
	aes *crypto.AESGCM
}

func NewAdminLogHandler(gen *repo.GenerationRepo, acc *repo.AccountRepo, aes *crypto.AESGCM) *AdminLogHandler {
	return &AdminLogHandler{gen: gen, acc: acc, aes: aes}
}

func (h *AdminLogHandler) GenerationLogs(c *gin.Context) {
	var req dto.AdminGenerationLogListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Fail(c, errcode.InvalidParam.Wrap(err))
		return
	}
	rows, total, err := h.gen.ListAdminLogs(c.Request.Context(), repo.AdminGenerationLogFilter{
		Keyword:  req.Keyword,
		Kind:     req.Kind,
		Status:   req.Status,
		Page:     req.Page,
		PageSize: req.PageSize,
	})
	if err != nil {
		response.Fail(c, errcode.DBError.Wrap(err))
		return
	}
	out := make([]*dto.AdminGenerationLogResp, 0, len(rows))
	for _, r := range rows {
		item := &dto.AdminGenerationLogResp{
			TaskID:     r.TaskID,
			CreatedAt:  r.CreatedAt.Unix(),
			UserID:     r.UserID,
			UserLabel:  r.UserLabel,
			Kind:       r.Kind,
			ModelCode:  r.ModelCode,
			Prompt:     r.Prompt,
			Status:     r.Status,
			CostPoints: r.CostPoints,
		}
		if r.APIKeyID != nil {
			item.APIKeyID = *r.APIKeyID
		}
		if r.KeyLabel != nil {
			item.KeyLabel = *r.KeyLabel
		}
		if r.DurationMs != nil {
			item.DurationMs = *r.DurationMs
		}
		if r.PreviewURL != nil && *r.PreviewURL != "" {
			item.PreviewURL = fmt.Sprintf("/admin/api/v1/logs/generations/%s/preview", r.TaskID)
		}
		if r.Error != nil {
			item.Error = *r.Error
		}
		out = append(out, item)
	}
	page, pageSize := req.Page, req.PageSize
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	response.Page(c, out, total, page, pageSize)
}

func (h *AdminLogHandler) GenerationUpstreamLogs(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("task_id"))
	if taskID == "" {
		response.Fail(c, errcode.InvalidParam.WithMsg("empty task_id"))
		return
	}
	rows, err := h.gen.ListUpstreamLogs(c.Request.Context(), taskID)
	if err != nil {
		response.Fail(c, errcode.DBError.Wrap(err))
		return
	}
	out := make([]*dto.AdminGenerationUpstreamLogResp, 0, len(rows))
	for _, r := range rows {
		item := &dto.AdminGenerationUpstreamLogResp{
			ID:         r.ID,
			TaskID:     r.TaskID,
			Provider:   r.Provider,
			AccountID:  r.AccountID,
			Stage:      r.Stage,
			StatusCode: r.StatusCode,
			DurationMs: r.DurationMs,
			CreatedAt:  r.CreatedAt.Unix(),
		}
		if r.Method != nil {
			item.Method = *r.Method
		}
		if r.URL != nil {
			item.URL = *r.URL
		}
		if r.RequestExcerpt != nil {
			item.RequestExcerpt = *r.RequestExcerpt
		}
		if r.ResponseExcerpt != nil {
			item.ResponseExcerpt = *r.ResponseExcerpt
		}
		if r.Error != nil {
			item.Error = *r.Error
		}
		if r.Meta != nil {
			item.Meta = *r.Meta
		}
		out = append(out, item)
	}
	response.OK(c, out)
}

// GenerationPreview proxies a request-log preview through the admin origin.
func (h *AdminLogHandler) GenerationPreview(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("task_id"))
	t, err := h.gen.GetByTaskID(c.Request.Context(), taskID)
	if err != nil {
		response.Fail(c, errcode.ResourceMissing)
		return
	}
	results, err := h.gen.ListResultsByTask(c.Request.Context(), taskID)
	if err != nil || len(results) == 0 {
		response.Fail(c, errcode.ResourceMissing)
		return
	}
	r := results[0]
	rawURL := r.URL
	if t.Kind != "video" && r.ThumbURL != nil && *r.ThumbURL != "" {
		rawURL = *r.ThumbURL
	}
	if t.Kind == "video" && c.Query("thumb") == "1" && r.ThumbURL != nil && *r.ThumbURL != "" {
		rawURL = *r.ThumbURL
	}
	h.servePreviewURL(c, t, rawURL)
}

func (h *AdminLogHandler) servePreviewURL(c *gin.Context, t *model.GenerationTask, rawURL string) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		response.Fail(c, errcode.ResourceMissing)
		return
	}
	if strings.HasPrefix(rawURL, "/api/v1/gen/cached/") {
		serveAdminCachedAsset(c, strings.TrimPrefix(rawURL, "/api/v1/gen/cached/"))
		return
	}
	if strings.HasPrefix(rawURL, "/admin/api/v1/") {
		response.Fail(c, errcode.ResourceMissing)
		return
	}
	target := adminNormalizeGrokAssetURL(rawURL)
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		if !strings.Contains(target, "assets.grok.com") {
			c.Redirect(http.StatusFound, target)
			return
		}
		cookie, err := h.grokCookieForTask(c.Request.Context(), t)
		if err != nil {
			response.Fail(c, errcode.GPTUnavailable.WithMsg("资源下载凭证不可用"))
			return
		}
		proxyRemoteAsset(c, target, cookie, rawURL)
		return
	}
	response.Fail(c, errcode.ResourceMissing)
}

func serveAdminCachedAsset(c *gin.Context, rel string) {
	rel = strings.TrimLeft(rel, "/")
	if rel == "" || strings.Contains(rel, "..") {
		response.Fail(c, errcode.InvalidParam.WithMsg("invalid asset path"))
		return
	}
	root := strings.TrimSpace(os.Getenv("KLEIN_STORAGE_ROOT"))
	if root == "" {
		root = "/app/storage/public"
	}
	c.Header("Cache-Control", "public, max-age=86400")
	c.File(filepath.Join(root, filepath.FromSlash(rel)))
}

func proxyRemoteAsset(c *gin.Context, target, cookie, rawURL string) {
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, target, nil)
	if err != nil {
		response.Fail(c, errcode.InvalidParam.Wrap(err))
		return
	}
	req.Header.Set("Cookie", cookie)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://grok.com/")
	req.Header.Set("Accept", "*/*")
	resp, err := (&http.Client{Timeout: 2 * time.Minute}).Do(req)
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
	c.Header("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, adminAssetName(rawURL)))
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, resp.Body)
}

func adminNormalizeGrokAssetURL(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") || strings.HasPrefix(v, "data:") {
		return v
	}
	return "https://assets.grok.com/" + strings.TrimLeft(v, "/")
}

func (h *AdminLogHandler) grokCookieForTask(ctx context.Context, t *model.GenerationTask) (string, error) {
	if t.AccountID == nil || h.acc == nil || h.aes == nil {
		return "", fmt.Errorf("missing account")
	}
	acc, err := h.acc.GetByID(ctx, *t.AccountID)
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
			if token := adminCookieValue(cred, "sso"); token != "" {
				cred = strings.TrimRight(cred, "; ") + "; sso-rw=" + token
			}
		}
		return cred, nil
	}
	return "sso=" + cred + "; sso-rw=" + cred, nil
}

func adminCookieValue(cookie, name string) string {
	prefix := name + "="
	for _, part := range strings.Split(cookie, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, prefix) {
			return strings.TrimPrefix(part, prefix)
		}
	}
	return ""
}

func adminAssetName(rawURL string) string {
	name := "asset"
	if i := strings.LastIndex(rawURL, "/"); i >= 0 && i+1 < len(rawURL) {
		name = rawURL[i+1:]
	}
	name = strings.TrimSpace(strings.Split(name, "?")[0])
	if name == "" {
		return "asset"
	}
	return name
}

func (h *AdminLogHandler) PurgeGenerationLogs(c *gin.Context) {
	var req dto.AdminGenerationLogPurgeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.InvalidParam.Wrap(err))
		return
	}
	before := time.Now().UTC().AddDate(0, 0, -req.Days)
	deleted, err := h.gen.SoftDeleteAdminLogsBefore(c.Request.Context(), before)
	if err != nil {
		response.Fail(c, errcode.DBError.Wrap(err))
		return
	}
	response.OK(c, &dto.AdminGenerationLogPurgeResp{Deleted: deleted})
}
