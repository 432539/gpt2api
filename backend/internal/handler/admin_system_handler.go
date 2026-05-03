package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/kleinai/backend/internal/middleware"
	"github.com/kleinai/backend/internal/service"
	"github.com/kleinai/backend/pkg/errcode"
	"github.com/kleinai/backend/pkg/response"
)

// AdminSystemHandler /admin/api/v1/system 资源 handler。
type AdminSystemHandler struct {
	svc *service.SystemConfigService
}

// NewAdminSystemHandler 构造。
func NewAdminSystemHandler(svc *service.SystemConfigService) *AdminSystemHandler {
	return &AdminSystemHandler{svc: svc}
}

// GetSettings GET /admin/api/v1/system/settings
func (h *AdminSystemHandler) GetSettings(c *gin.Context) {
	all, err := h.svc.GetAll(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, all)
}

// UpdateSettings PUT /admin/api/v1/system/settings
//
// Body: { "<key>": <any-json>, ... }
func (h *AdminSystemHandler) UpdateSettings(c *gin.Context) {
	var body map[string]any
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Fail(c, errcode.InvalidParam.Wrap(err))
		return
	}
	if len(body) == 0 {
		response.OK(c, gin.H{"updated": 0})
		return
	}
	uid := middleware.UID(c)
	if err := h.svc.UpsertMany(c.Request.Context(), body, uid); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"updated": len(body)})
}
