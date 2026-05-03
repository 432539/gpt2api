// Package handler 管理后台 - 账号池 handler。
package handler

import (
	"fmt"
	"strconv"
	"strings"

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
	const maxSub2APIChunk = 500
	var req dto.AccountBatchImportReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.InvalidParam.Wrap(err))
		return
	}
	format := strings.ToLower(strings.TrimSpace(req.Format))
	if format == "" {
		if len(req.Accounts) > 0 {
			format = "sub2api"
		} else {
			format = "lines"
		}
	}
	req.Format = format
	switch format {
	case "sub2api":
		if len(req.Accounts) == 0 {
			response.Fail(c, errcode.InvalidParam.WithMsg("sub2api 导入需提供非空 accounts"))
			return
		}
		if len(req.Accounts) > maxSub2APIChunk {
			response.Fail(c, errcode.InvalidParam.WithMsg(fmt.Sprintf("单次最多导入 %d 条，请拆分多次请求", maxSub2APIChunk)))
			return
		}
	case "lines":
		// ok
	default:
		response.Fail(c, errcode.InvalidParam.WithMsg("format 仅支持 lines / sub2api"))
		return
	}
	uid := middleware.UID(c)
	res, err := h.svc.BatchImport(c.Request.Context(), uid, &req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// BatchDelete POST /admin/api/v1/accounts/batch-delete
func (h *AdminAccountHandler) BatchDelete(c *gin.Context) {
	var req dto.AccountBatchDeleteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.InvalidParam.Wrap(err))
		return
	}
	n, err := h.svc.BatchDeleteByIDs(c.Request.Context(), req.IDs)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, dto.AccountBulkOpResult{Deleted: n})
}

// Purge POST /admin/api/v1/accounts/purge
func (h *AdminAccountHandler) Purge(c *gin.Context) {
	var req dto.AccountPurgeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.InvalidParam.Wrap(err))
		return
	}
	n, err := h.svc.PurgeAccounts(c.Request.Context(), &req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, dto.AccountBulkOpResult{Deleted: n})
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

// Secrets GET /admin/api/v1/accounts/:id/secrets
//
// 仅管理员可见，返回单个账号的明文凭证（解密后），用于编辑面板回显。
func (h *AdminAccountHandler) Secrets(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, errcode.InvalidParam)
		return
	}
	res, err := h.svc.GetSecrets(c.Request.Context(), id)
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
		Page     int    `json:"page"`
		PageSize int    `json:"page_size"`
	}
	_ = c.ShouldBindJSON(&body)
	res, err := h.svc.BatchRefreshOAuth(c.Request.Context(), body.Provider, body.Page, body.PageSize)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

func (h *AdminAccountHandler) BatchProbeQuota(c *gin.Context) {
	var body struct {
		Provider string `json:"provider"`
		Page     int    `json:"page"`
		PageSize int    `json:"page_size"`
	}
	_ = c.ShouldBindJSON(&body)
	res, err := h.svc.BatchProbeQuota(c.Request.Context(), body.Provider, body.Page, body.PageSize)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}
