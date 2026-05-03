// Package handler 用户端生成任务 handler。
package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kleinai/backend/internal/dto"
	"github.com/kleinai/backend/internal/middleware"
	"github.com/kleinai/backend/internal/model"
	"github.com/kleinai/backend/internal/provider"
	"github.com/kleinai/backend/internal/repo"
	"github.com/kleinai/backend/internal/service"
	"github.com/kleinai/backend/pkg/errcode"
	"github.com/kleinai/backend/pkg/response"
)

// GenerationHandler 生成任务 handler。
type GenerationHandler struct {
	svc  *service.GenerationService
	repo *repo.GenerationRepo
}

// NewGenerationHandler 构造。
func NewGenerationHandler(svc *service.GenerationService, r *repo.GenerationRepo) *GenerationHandler {
	return &GenerationHandler{svc: svc, repo: r}
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
	}
	if req.Quality != "" {
		params["quality"] = req.Quality
	}
	if req.Duration > 0 {
		params["duration"] = float64(req.Duration)
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

	items, total, err := h.repo.ListByUser(c.Request.Context(), uid, kind, page, pageSize)
	if err != nil {
		response.Fail(c, errcode.DBError.Wrap(err))
		return
	}
	out := make([]*dto.GenerationTaskResp, 0, len(items))
	for _, t := range items {
		out = append(out, taskToResp(t, nil))
	}
	response.Page(c, out, total, page, pageSize)
}

// === helpers ===

func taskToResp(t *model.GenerationTask, results []*model.GenerationResult) *dto.GenerationTaskResp {
	r := &dto.GenerationTaskResp{
		TaskID:     t.TaskID,
		Kind:       t.Kind,
		Status:     t.Status,
		Progress:   t.Progress,
		ModelCode:  t.ModelCode,
		CostPoints: t.CostPoints,
		CreatedAt:  t.CreatedAt.Unix(),
	}
	if t.Error != nil {
		r.Error = *t.Error
	}
	for _, gr := range results {
		row := dto.GenerationResultResp{URL: gr.URL}
		if gr.ThumbURL != nil {
			row.ThumbURL = *gr.ThumbURL
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
