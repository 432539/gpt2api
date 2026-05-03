// Package handler API Key handler。
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

// APIKeyHandler 用户端 API Key handler。
type APIKeyHandler struct {
	svc *service.APIKeyService
}

// NewAPIKeyHandler 构造。
func NewAPIKeyHandler(svc *service.APIKeyService) *APIKeyHandler {
	return &APIKeyHandler{svc: svc}
}

// Create POST /api/v1/keys
func (h *APIKeyHandler) Create(c *gin.Context) {
	var req dto.APIKeyCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.InvalidParam.Wrap(err))
		return
	}
	uid := middleware.MustUID(c)
	resp, err := h.svc.Create(c.Request.Context(), uid, &req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, resp)
}

// List GET /api/v1/keys
func (h *APIKeyHandler) List(c *gin.Context) {
	uid := middleware.MustUID(c)
	items, err := h.svc.List(c.Request.Context(), uid)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"list": items})
}

// Toggle POST /api/v1/keys/:id/toggle?enable=1
func (h *APIKeyHandler) Toggle(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, errcode.InvalidParam)
		return
	}
	enable := c.DefaultQuery("enable", "1") == "1"
	uid := middleware.MustUID(c)
	if err := h.svc.Toggle(c.Request.Context(), uid, id, enable); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

// Delete DELETE /api/v1/keys/:id
func (h *APIKeyHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, errcode.InvalidParam)
		return
	}
	uid := middleware.MustUID(c)
	if err := h.svc.Delete(c.Request.Context(), uid, id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}
