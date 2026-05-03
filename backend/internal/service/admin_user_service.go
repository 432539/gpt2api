package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kleinai/backend/internal/dto"
	"github.com/kleinai/backend/internal/model"
	"github.com/kleinai/backend/internal/repo"
	"github.com/kleinai/backend/pkg/crypto"
	"github.com/kleinai/backend/pkg/errcode"
)

type AdminUserService struct {
	users  *repo.UserRepo
	wallet *repo.WalletRepo
}

func NewAdminUserService(users *repo.UserRepo, wallet *repo.WalletRepo) *AdminUserService {
	return &AdminUserService{users: users, wallet: wallet}
}

func (s *AdminUserService) List(ctx context.Context, req *dto.AdminUserListReq) ([]*dto.AdminUserResp, int64, error) {
	items, total, err := s.users.List(ctx, repo.UserListFilter{
		Keyword:  req.Keyword,
		Status:   req.Status,
		Page:     req.Page,
		PageSize: req.PageSize,
	})
	if err != nil {
		return nil, 0, errcode.DBError.Wrap(err)
	}
	out := make([]*dto.AdminUserResp, 0, len(items))
	for _, u := range items {
		out = append(out, adminUserToResp(u))
	}
	return out, total, nil
}

func (s *AdminUserService) Create(ctx context.Context, req *dto.AdminUserCreateReq) (*model.User, error) {
	account := strings.TrimSpace(req.Account)
	if account == "" {
		return nil, errcode.InvalidParam.WithMsg("账号不能为空")
	}
	hash, err := crypto.HashPassword(req.Password)
	if err != nil {
		return nil, errcode.Internal.Wrap(err)
	}
	status := int8(1)
	if req.Status != nil {
		status = *req.Status
	}
	u := &model.User{
		UUID:       uuid.NewString(),
		Password:   hash,
		Status:     status,
		PlanCode:   "free",
		InviteCode: genInviteCode(),
	}
	switch {
	case emailRe.MatchString(account):
		v := strings.ToLower(account)
		u.Email = &v
		if username := strings.TrimSpace(req.Username); username != "" {
			u.Username = &username
		} else if username := defaultUsername(v); username != "" {
			u.Username = &username
		}
	case phoneRe.MatchString(account):
		v := account
		u.Phone = &v
		if username := strings.TrimSpace(req.Username); username != "" {
			u.Username = &username
		}
	default:
		v := account
		u.Username = &v
	}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, wrapDup(err)
	}
	if req.Points > 0 && s.wallet != nil {
		if _, err := s.wallet.Adjust(ctx, u.ID, model.BizRecharge, adminBizID("create"), req.Points, "管理员创建用户赠送", true); err != nil {
			return nil, errcode.DBError.Wrap(err)
		}
		u.Points = req.Points
		u.TotalRecharge = req.Points
	}
	return u, nil
}

func (s *AdminUserService) Update(ctx context.Context, id uint64, req *dto.AdminUserUpdateReq) error {
	fields := map[string]any{}
	if req.Email != nil {
		v := strings.TrimSpace(*req.Email)
		if v == "" {
			fields["email"] = nil
		} else if !emailRe.MatchString(v) {
			return errcode.InvalidParam.WithMsg("邮箱格式不正确")
		} else {
			fields["email"] = strings.ToLower(v)
		}
	}
	if req.Phone != nil {
		v := strings.TrimSpace(*req.Phone)
		if v == "" {
			fields["phone"] = nil
		} else if !phoneRe.MatchString(v) {
			return errcode.InvalidParam.WithMsg("手机号格式不正确")
		} else {
			fields["phone"] = v
		}
	}
	if req.Username != nil {
		v := strings.TrimSpace(*req.Username)
		if v == "" {
			fields["username"] = nil
		} else {
			fields["username"] = v
		}
	}
	if req.Avatar != nil {
		v := strings.TrimSpace(*req.Avatar)
		if v == "" {
			fields["avatar"] = nil
		} else {
			fields["avatar"] = v
		}
	}
	if req.Password != nil && strings.TrimSpace(*req.Password) != "" {
		hash, err := crypto.HashPassword(*req.Password)
		if err != nil {
			return errcode.Internal.Wrap(err)
		}
		fields["password"] = hash
	}
	if req.Status != nil {
		fields["status"] = *req.Status
	}
	if req.PlanCode != nil {
		v := strings.TrimSpace(*req.PlanCode)
		if v == "" {
			v = "free"
		}
		fields["plan_code"] = v
	}
	if req.PlanExpireAt != nil {
		if *req.PlanExpireAt <= 0 {
			fields["plan_expire_at"] = nil
		} else {
			t := time.Unix(*req.PlanExpireAt, 0).UTC()
			fields["plan_expire_at"] = &t
		}
	}
	if err := s.users.Update(ctx, id, fields); err != nil {
		return wrapDup(err)
	}
	return nil
}

func (s *AdminUserService) AdjustPoints(ctx context.Context, id uint64, req *dto.AdminUserAdjustPointsReq) (*dto.AdminUserAdjustPointsResp, error) {
	if s.wallet == nil {
		return nil, errcode.Internal.WithMsg("钱包服务未启用")
	}
	points := req.Points
	biz := model.BizRecharge
	addTotal := true
	remark := strings.TrimSpace(req.Remark)
	if remark == "" {
		remark = "管理员充值"
	}
	if req.Action == "deduct" {
		points = -points
		biz = "admin_deduct"
		addTotal = false
		if remark == "" || remark == "管理员充值" {
			remark = "管理员扣除"
		}
	}
	log, err := s.wallet.Adjust(ctx, id, biz, adminBizID(req.Action), points, remark, addTotal)
	if err != nil {
		if errors.Is(err, repo.ErrInsufficient) {
			return nil, errcode.InvalidParam.WithMsg("用户可用积分不足")
		}
		return nil, errcode.DBError.Wrap(err)
	}
	return &dto.AdminUserAdjustPointsResp{PointsBefore: log.PointsBefore, PointsAfter: log.PointsAfter}, nil
}

func adminUserToResp(u *model.User) *dto.AdminUserResp {
	r := &dto.AdminUserResp{
		ID:            u.ID,
		UUID:          u.UUID,
		Points:        u.Points,
		FrozenPoints:  u.FrozenPoints,
		TotalRecharge: u.TotalRecharge,
		PlanCode:      u.PlanCode,
		InviterID:     u.InviterID,
		InviteCode:    u.InviteCode,
		Status:        u.Status,
		CreatedAt:     u.CreatedAt.Unix(),
		UpdatedAt:     u.UpdatedAt.Unix(),
	}
	if u.Email != nil {
		r.Email = *u.Email
	}
	if u.Phone != nil {
		r.Phone = *u.Phone
	}
	if u.Username != nil {
		r.Username = *u.Username
	}
	if u.Avatar != nil {
		r.Avatar = *u.Avatar
	}
	if u.PlanExpireAt != nil {
		r.PlanExpireAt = u.PlanExpireAt.Unix()
	}
	if u.RegisterIP != nil {
		r.RegisterIP = *u.RegisterIP
	}
	if u.LastLoginAt != nil {
		r.LastLoginAt = u.LastLoginAt.Unix()
	}
	if u.LastLoginIP != nil {
		r.LastLoginIP = *u.LastLoginIP
	}
	return r
}

func adminBizID(action string) string {
	return fmt.Sprintf("admin-%s-%d", action, time.Now().UnixNano())
}
