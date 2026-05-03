// Package service 业务编排层。
package service

import (
	"context"

	"github.com/kleinai/backend/internal/dto"
	"github.com/kleinai/backend/internal/repo"
	"github.com/kleinai/backend/pkg/errcode"
)

// UserService 用户读取层。
type UserService struct {
	user *repo.UserRepo
}

// NewUserService 构造。
func NewUserService(u *repo.UserRepo) *UserService { return &UserService{user: u} }

// Me 获取当前用户。
func (s *UserService) Me(ctx context.Context, uid uint64) (*dto.MeResp, error) {
	u, err := s.user.GetByID(ctx, uid)
	if err != nil {
		return nil, errcode.UserNotFound
	}
	return &dto.MeResp{
		UID:        u.ID,
		UUID:       u.UUID,
		Username:   u.Username,
		Email:      u.Email,
		Phone:      u.Phone,
		Avatar:     u.Avatar,
		Points:     u.Points,
		FrozenPts:  u.FrozenPoints,
		PlanCode:   u.PlanCode,
		InviteCode: u.InviteCode,
		CreatedAt:  u.CreatedAt.Unix(),
	}, nil
}
