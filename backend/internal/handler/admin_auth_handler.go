// Package handler 管理后台 - auth handler。
package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/kleinai/backend/internal/dto"
	"github.com/kleinai/backend/internal/middleware"
	"github.com/kleinai/backend/internal/repo"
	"github.com/kleinai/backend/internal/service"
	"github.com/kleinai/backend/pkg/errcode"
	"github.com/kleinai/backend/pkg/response"
)

// AdminAuthHandler 后台认证 handler。
type AdminAuthHandler struct {
	auth *service.AdminAuthService
	repo *repo.AdminRepo
}

// NewAdminAuthHandler 构造。
func NewAdminAuthHandler(auth *service.AdminAuthService, r *repo.AdminRepo) *AdminAuthHandler {
	return &AdminAuthHandler{auth: auth, repo: r}
}

// Login POST /admin/api/v1/auth/login
func (h *AdminAuthHandler) Login(c *gin.Context) {
	var req dto.LoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.InvalidParam.Wrap(err))
		return
	}
	u, tok, err := h.auth.Login(c.Request.Context(), &req, c.ClientIP())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{
		"id":       u.ID,
		"username": u.Username,
		"nickname": u.Nickname,
		"role_id":  u.RoleID,
		"token":    tok,
	})
}

// Me GET /admin/api/v1/auth/me
func (h *AdminAuthHandler) Me(c *gin.Context) {
	uid := middleware.MustUID(c)
	u, err := h.repo.GetByID(c.Request.Context(), uid)
	if err != nil {
		response.Fail(c, errcode.UserNotFound)
		return
	}
	role, _ := h.repo.GetRoleByID(c.Request.Context(), u.RoleID)
	roleCode, roleName := "", ""
	if role != nil {
		roleCode, roleName = role.Code, role.Name
	}
	response.OK(c, gin.H{
		"id":        u.ID,
		"username":  u.Username,
		"nickname":  u.Nickname,
		"email":     u.Email,
		"role_id":   u.RoleID,
		"role_code": roleCode,
		"role_name": roleName,
	})
}

// ChangePassword POST /admin/api/v1/auth/password
func (h *AdminAuthHandler) ChangePassword(c *gin.Context) {
	uid := middleware.MustUID(c)
	var req dto.ChangePasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.InvalidParam.Wrap(err))
		return
	}
	if err := h.auth.ChangePassword(c.Request.Context(), uid, &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"ok": true})
}
