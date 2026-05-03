// Package handler 管理后台 - 账号池 handler。
package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kleinai/backend/internal/dto"
	"github.com/kleinai/backend/internal/middleware"
	"github.com/kleinai/backend/internal/service"
	"github.com/kleinai/backend/pkg/errcode"
	"github.com/kleinai/backend/pkg/response"
)

// AdminAccountHandler /admin/api/v1/accounts 资源 handler。
type AdminAccountHandler struct {
	svc  *service.AccountAdminService
	pool *service.AccountPool
}

// NewAdminAccountHandler 构造。
func NewAdminAccountHandler(svc *service.AccountAdminService, pool *service.AccountPool) *AdminAccountHandler {
	return &AdminAccountHandler{svc: svc, pool: pool}
}

// List GET /admin/api/v1/accounts
func (h *AdminAccountHandler) List(c *gin.Context) {
	var req dto.AccountListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Fail(c, errcode.InvalidParam.Wrap(err))
		return
	}
	items, total, err := h.svc.List(c.Request.Context(), &req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	page, pageSize := req.Page, req.PageSize
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	response.Page(c, items, total, page, pageSize)
}

// Create POST /admin/api/v1/accounts
func (h *AdminAccountHandler) Create(c *gin.Context) {
	var req dto.AccountCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.InvalidParam.Wrap(err))
		return
	}
	uid := middleware.UID(c)
	a, err := h.svc.Create(c.Request.Context(), uid, &req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"id": a.ID})
}

// Update PUT /admin/api/v1/accounts/:id
func (h *AdminAccountHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, errcode.InvalidParam)
		return
	}
	var req dto.AccountUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.InvalidParam.Wrap(err))
		return
	}
	if err := h.svc.Update(c.Request.Context(), id, &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

// Delete DELETE /admin/api/v1/accounts/:id
func (h *AdminAccountHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, errcode.InvalidParam)
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

// BatchImport POST /admin/api/v1/accounts/import
func (h *AdminAccountHandler) BatchImport(c *gin.Context) {
	var req dto.AccountBatchImportReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.InvalidParam.Wrap(err))
		return
	}
	uid := middleware.UID(c)
	n, err := h.svc.BatchImport(c.Request.Context(), uid, &req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"imported": n})
}

// PoolStats GET /admin/api/v1/accounts/stats
func (h *AdminAccountHandler) PoolStats(c *gin.Context) {
	response.OK(c, gin.H{"pool": h.pool.Stats()})
}

// Test POST /admin/api/v1/accounts/:id/test
func (h *AdminAccountHandler) Test(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, errcode.InvalidParam)
		return
	}
	res, err := h.svc.Test(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// RefreshOAuth POST /admin/api/v1/accounts/:id/refresh
func (h *AdminAccountHandler) RefreshOAuth(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, errcode.InvalidParam)
		return
	}
	res, err := h.svc.RefreshOAuth(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// BatchRefresh POST /admin/api/v1/accounts/batch-refresh
//
// Body: { "provider": "gpt" } 留空表示全部。
func (h *AdminAccountHandler) BatchRefresh(c *gin.Context) {
	var body struct {
		Provider string `json:"provider"`
	}
	_ = c.ShouldBindJSON(&body)
	ok, failed, err := h.svc.BatchRefreshOAuth(c.Request.Context(), body.Provider)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"refreshed": ok, "failed_ids": failed})
}
