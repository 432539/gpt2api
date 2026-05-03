// Package service 业务编排层。事务、幂等、跨 repo 协作发生在这里。
package service

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/kleinai/backend/internal/dto"
	"github.com/kleinai/backend/internal/model"
	"github.com/kleinai/backend/internal/repo"
	"github.com/kleinai/backend/pkg/crypto"
	"github.com/kleinai/backend/pkg/errcode"
	"github.com/kleinai/backend/pkg/jwtx"
	"github.com/kleinai/backend/pkg/logger"
)

// AuthService 用户认证。
type AuthService struct {
	db   *gorm.DB
	user *repo.UserRepo
	jwt  *jwtx.Manager
}

// NewAuthService 构造。
func NewAuthService(db *gorm.DB, userRepo *repo.UserRepo, jwt *jwtx.Manager) *AuthService {
	return &AuthService{db: db, user: userRepo, jwt: jwt}
}

var (
	emailRe = regexp.MustCompile(`^[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$`)
	phoneRe = regexp.MustCompile(`^1[3-9]\d{9}$`)
)

// Register 用户注册（事务内创建用户 + 邀请关系 + 注册赠点流水可在后续扩展）。
func (s *AuthService) Register(ctx context.Context, req *dto.RegisterReq, ip string) (*model.User, *dto.TokenPair, error) {
	account := strings.TrimSpace(req.Account)
	if account == "" {
		return nil, nil, errcode.InvalidParam
	}

	user := &model.User{
		UUID:       uuid.NewString(),
		Status:     1,
		PlanCode:   "free",
		InviteCode: genInviteCode(),
		RegisterIP: &ip,
	}

	switch {
	case emailRe.MatchString(account):
		e := strings.ToLower(account)
		user.Email = &e
		if username := defaultUsername(e); username != "" {
			user.Username = &username
		}
	case phoneRe.MatchString(account):
		p := account
		user.Phone = &p
	default:
		user.Username = &account
	}

	hash, err := crypto.HashPassword(req.Password)
	if err != nil {
		return nil, nil, errcode.Internal.Wrap(err)
	}
	user.Password = hash

	if req.InviteCode != "" {
		if inv, err := s.user.GetByInviteCode(ctx, req.InviteCode); err == nil && inv != nil {
			user.InviterID = &inv.ID
		}
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(user).Error; err != nil {
			return wrapDup(err)
		}
		if user.InviterID != nil {
			if err := tx.Exec(
				"INSERT IGNORE INTO user_invite_relation (user_id, inviter_id, invite_code) VALUES (?, ?, ?)",
				user.ID, *user.InviterID, req.InviteCode,
			).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	tok, err := s.issue(user)
	if err != nil {
		return nil, nil, err
	}
	logger.FromCtx(ctx).Info("auth.register", zap.Uint64("uid", user.ID), zap.String("ip", ip))
	return user, tok, nil
}

// Login 登录。
func (s *AuthService) Login(ctx context.Context, req *dto.LoginReq, ip string) (*model.User, *dto.TokenPair, error) {
	u, err := s.user.GetByAccount(ctx, req.Account)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, nil, errcode.UserNotFound
		}
		return nil, nil, errcode.DBError.Wrap(err)
	}
	if !u.IsActive() {
		return nil, nil, errcode.Forbidden.WithMsg("账号已停用")
	}
	if !crypto.VerifyPassword(u.Password, req.Password) {
		return nil, nil, errcode.Unauthorized.WithMsg("账号或密码错误")
	}
	if err := s.user.UpdateLogin(ctx, u.ID, ip); err != nil {
		logger.FromCtx(ctx).Warn("update login failed", zap.Error(err))
	}

	tok, err := s.issue(u)
	if err != nil {
		return nil, nil, err
	}
	return u, tok, nil
}

// Refresh 用 refresh token 换新 access token。
func (s *AuthService) Refresh(ctx context.Context, refresh string) (*dto.TokenPair, error) {
	cl, err := s.jwt.ParseRefresh(refresh)
	if err != nil {
		return nil, errcode.TokenExpired.Wrap(err)
	}
	u, err := s.user.GetByID(ctx, cl.UID)
	if err != nil {
		return nil, errcode.UserNotFound
	}
	if !u.IsActive() {
		return nil, errcode.Forbidden
	}
	return s.issue(u)
}

// ChangePassword 修改密码。
func (s *AuthService) ChangePassword(ctx context.Context, uid uint64, req *dto.ChangePasswordReq) error {
	u, err := s.user.GetByID(ctx, uid)
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
	return s.user.UpdatePassword(ctx, uid, hash)
}

// issue 颁发 access + refresh。
func (s *AuthService) issue(u *model.User) (*dto.TokenPair, error) {
	jti := uuid.NewString()
	access, accExp, err := s.jwt.IssueAccess(u.ID, jwtx.SubjectUser, jti, []string{u.PlanCode})
	if err != nil {
		return nil, errcode.Internal.Wrap(err)
	}
	refresh, refExp, err := s.jwt.IssueRefresh(u.ID, jwtx.SubjectUser, jti)
	if err != nil {
		return nil, errcode.Internal.Wrap(err)
	}
	now := time.Now()
	return &dto.TokenPair{
		AccessToken:     access,
		RefreshToken:    refresh,
		TokenType:       "Bearer",
		AccessExpireIn:  int64(accExp.Sub(now).Seconds()),
		RefreshExpireIn: int64(refExp.Sub(now).Seconds()),
	}, nil
}

// === helpers ===

// genInviteCode 8 位邀请码：K + 7 位大写 base32（无 0/1/I/O）。
func genInviteCode() string {
	const alphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ" // 32 char
	b, _ := crypto.RandomBytes(8)
	out := make([]byte, 8)
	out[0] = 'K'
	for i := 1; i < 8; i++ {
		out[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(out)
}

func defaultUsername(email string) string {
	if i := strings.Index(email, "@"); i > 0 {
		return email[:i]
	}
	return ""
}

// wrapDup 把 MySQL 唯一索引冲突映射成 UserExists。
func wrapDup(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, "Error 1062") || strings.Contains(msg, "Duplicate entry") {
		return errcode.UserExists.Wrap(err)
	}
	return err
}
