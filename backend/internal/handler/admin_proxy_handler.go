package handler

import (
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"

	"github.com/kleinai/backend/internal/dto"
	"github.com/kleinai/backend/internal/middleware"
	"github.com/kleinai/backend/internal/service"
	"github.com/kleinai/backend/pkg/errcode"
	"github.com/kleinai/backend/pkg/response"
)

// AdminProxyHandler /admin/api/v1/proxies 资源 handler。
type AdminProxyHandler struct {
	svc     *service.ProxyService
	testSvc *service.AccountTestService // 复用 TestProxy 探测能力
}

// NewAdminProxyHandler 构造。
func NewAdminProxyHandler(svc *service.ProxyService, t *service.AccountTestService) *AdminProxyHandler {
	return &AdminProxyHandler{svc: svc, testSvc: t}
}

// List GET /admin/api/v1/proxies
func (h *AdminProxyHandler) List(c *gin.Context) {
	var req dto.ProxyListReq
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
		pageSize = 50
	}
	response.Page(c, items, total, page, pageSize)
}

// Create POST /admin/api/v1/proxies
func (h *AdminProxyHandler) Create(c *gin.Context) {
	var req dto.ProxyCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.InvalidParam.Wrap(err))
		return
	}
	uid := middleware.UID(c)
	p, err := h.svc.Create(c.Request.Context(), uid, &req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"id": p.ID})
}

// BatchImport POST /admin/api/v1/proxies/import
func (h *AdminProxyHandler) BatchImport(c *gin.Context) {
	var req dto.ProxyBatchImportReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.InvalidParam.Wrap(err))
		return
	}
	uid := middleware.UID(c)
	res, err := h.svc.ImportText(c.Request.Context(), uid, req.Text)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Update PUT /admin/api/v1/proxies/:id
func (h *AdminProxyHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, errcode.InvalidParam)
		return
	}
	var req dto.ProxyUpdateReq
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

// Delete DELETE /admin/api/v1/proxies/:id
func (h *AdminProxyHandler) Delete(c *gin.Context) {
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

// BatchDelete POST /admin/api/v1/proxies/batch-delete
func (h *AdminProxyHandler) BatchDelete(c *gin.Context) {
	var req dto.ProxyBatchDeleteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.InvalidParam.Wrap(err))
		return
	}
	deleted, err := h.svc.BatchDelete(c.Request.Context(), req.IDs)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, dto.ProxyBatchDeleteResult{Deleted: deleted})
}

// Test POST /admin/api/v1/proxies/:id/test
//
// 通过 https://www.google.com/generate_204 探测代理可达性。
func (h *AdminProxyHandler) Test(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, errcode.InvalidParam)
		return
	}
	if h.testSvc == nil {
		response.Fail(c, errcode.Internal.WithMsg("测试服务未启用"))
		return
	}
	p, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, errcode.ResourceMissing)
		return
	}
	res, err := h.testSvc.TestProxy(c.Request.Context(), p)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// BatchTest POST /admin/api/v1/proxies/batch-test
func (h *AdminProxyHandler) BatchTest(c *gin.Context) {
	if h.testSvc == nil {
		response.Fail(c, errcode.Internal.WithMsg("测试服务未启用"))
		return
	}
	var req dto.ProxyBatchTestReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.InvalidParam.Wrap(err))
		return
	}
	okCount := 0
	failCount := 0
	tested := 0
	ids := make([]uint64, 0, len(req.IDs))
	var mu sync.Mutex
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for _, id := range req.IDs {
		p, err := h.svc.GetByID(c.Request.Context(), id)
		if err != nil || p == nil {
			continue
		}
		wg.Add(1)
		proxy := p
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-c.Request.Context().Done():
				return
			}
			defer func() { <-sem }()
			res, err := h.testSvc.TestProxy(c.Request.Context(), proxy)
			mu.Lock()
			defer mu.Unlock()
			tested++
			ids = append(ids, proxy.ID)
			if err != nil || res == nil || !res.OK {
				failCount++
				return
			}
			okCount++
		}()
	}
	wg.Wait()
	response.OK(c, dto.ProxyBatchTestResult{
		Tested: tested,
		OK:     okCount,
		Failed: failCount,
		IDs:    ids,
	})
}
