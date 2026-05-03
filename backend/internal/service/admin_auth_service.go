// Package service 后台账号认证。
package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kleinai/backend/internal/dto"
	"github.com/kleinai/backend/internal/model"
	"github.com/kleinai/backend/internal/repo"
	"github.com/kleinai/backend/pkg/crypto"
	"github.com/kleinai/backend/pkg/errcode"
	"github.com/kleinai/backend/pkg/jwtx"
)

// AdminAuthService 后台登录。
type AdminAuthService struct {
	repo *repo.AdminRepo
	jwt  *jwtx.Manager
}

// NewAdminAuthService 构造。
func NewAdminAuthService(r *repo.AdminRepo, jwt *jwtx.Manager) *AdminAuthService {
	return &AdminAuthService{repo: r, jwt: jwt}
}

// Login 后台登录。
func (s *AdminAuthService) Login(ctx context.Context, req *dto.LoginReq, ip string) (*model.AdminUser, *dto.TokenPair, error) {
	username := strings.TrimSpace(req.Account)
	if username == "" {
		return nil, nil, errcode.InvalidParam
	}
	u, err := s.repo.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, nil, errcode.Unauthorized.WithMsg("账号或密码错误")
		}
		return nil, nil, errcode.DBError.Wrap(err)
	}
	if !u.IsActive() {
		return nil, nil, errcode.Forbidden.WithMsg("账号已停用")
	}
	if !crypto.VerifyPassword(u.Password, req.Password) {
		return nil, nil, errcode.Unauthorized.WithMsg("账号或密码错误")
	}
	role, _ := s.repo.GetRoleByID(ctx, u.RoleID)
	roles := []string{}
	if role != nil {
		roles = append(roles, role.Code)
	}

	jti := uuid.NewString()
	access, accExp, err := s.jwt.IssueAccess(u.ID, jwtx.SubjectAdmin, jti, roles)
	if err != nil {
		return nil, nil, errcode.Internal.Wrap(err)
	}
	refresh, refExp, err := s.jwt.IssueRefresh(u.ID, jwtx.SubjectAdmin, jti)
	if err != nil {
		return nil, nil, errcode.Internal.Wrap(err)
	}
	_ = s.repo.UpdateLogin(ctx, u.ID, ip)

	now := time.Now()
	return u, &dto.TokenPair{
		AccessToken:     access,
		RefreshToken:    refresh,
		TokenType:       "Bearer",
		AccessExpireIn:  int64(accExp.Sub(now).Seconds()),
		RefreshExpireIn: int64(refExp.Sub(now).Seconds()),
	}, nil
}

// ChangePassword updates the current admin user's password.
func (s *AdminAuthService) ChangePassword(ctx context.Context, uid uint64, req *dto.ChangePasswordReq) error {
	u, err := s.repo.GetByID(ctx, uid)
	if err != nil {
		return errcode.UserNotFound
	}
	if !crypto.VerifyPassword(u.Password, req.OldPassword) {
		return errcode.Unauthorized.WithMsg("原密码不正确")
	}
	hash, err := crypto.HashPassword(req.NewPassword)
	if err != nil {
		return errcode.Internal.Wrap(err)
	}
	return s.repo.UpdatePassword(ctx, uid, hash)
}
