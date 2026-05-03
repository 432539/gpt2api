// Package handler 管理后台 - CDK handler。
package handler

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kleinai/backend/internal/dto"
	"github.com/kleinai/backend/internal/middleware"
	"github.com/kleinai/backend/internal/service"
	"github.com/kleinai/backend/pkg/errcode"
	"github.com/kleinai/backend/pkg/response"
)

// AdminCDKHandler 管理后台 CDK 批次 handler。
type AdminCDKHandler struct {
	svc *service.CDKService
}

// NewAdminCDKHandler 构造。
func NewAdminCDKHandler(svc *service.CDKService) *AdminCDKHandler {
	return &AdminCDKHandler{svc: svc}
}

// CreateBatch POST /admin/api/v1/cdk/batches
func (h *AdminCDKHandler) CreateBatch(c *gin.Context) {
	var req dto.CDKBatchCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.InvalidParam.Wrap(err))
		return
	}
	var expire *time.Time
	if req.ExpireAt > 0 {
		t := time.Unix(req.ExpireAt, 0).UTC()
		expire = &t
	}
	uid := middleware.UID(c)
	batch, err := h.svc.GenerateBatch(c.Request.Context(), uid, req.BatchNo, req.Name, req.Points, req.Qty, req.PerUserLimit, expire)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{
		"id":        batch.ID,
		"batch_no":  batch.BatchNo,
		"total_qty": batch.TotalQty,
	})
}
