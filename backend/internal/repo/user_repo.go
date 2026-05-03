// Package repo 数据访问层。零业务判断；禁止 SELECT *。
package repo

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/kleinai/backend/internal/model"
)

// UserRepo 用户表访问。
type UserRepo struct{ db *gorm.DB }

// NewUserRepo 构造。
func NewUserRepo(db *gorm.DB) *UserRepo { return &UserRepo{db: db} }

func (r *UserRepo) Create(ctx context.Context, u *model.User) error {
	return r.db.WithContext(ctx).Create(u).Error
}

func (r *UserRepo) GetByID(ctx context.Context, id uint64) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&u).Error
	if err != nil {
		return nil, mapErr(err)
	}
	return &u, nil
}

func (r *UserRepo) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).Where("email = ? AND deleted_at IS NULL", email).First(&u).Error
	if err != nil {
		return nil, mapErr(err)
	}
	return &u, nil
}

func (r *UserRepo) GetByPhone(ctx context.Context, phone string) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).Where("phone = ? AND deleted_at IS NULL", phone).First(&u).Error
	if err != nil {
		return nil, mapErr(err)
	}
	return &u, nil
}

// GetByAccount 按邮箱 / 手机 / 用户名匹配（仅一种命中）。
func (r *UserRepo) GetByAccount(ctx context.Context, account string) (*model.User, error) {
	account = strings.TrimSpace(account)
	if account == "" {
		return nil, ErrNotFound
	}
	var u model.User
	tx := r.db.WithContext(ctx).
		Where("(email = ? OR phone = ? OR username = ?) AND deleted_at IS NULL",
			account, account, account)
	if err := tx.First(&u).Error; err != nil {
		return nil, mapErr(err)
	}
	return &u, nil
}

func (r *UserRepo) GetByInviteCode(ctx context.Context, code string) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).Where("invite_code = ? AND deleted_at IS NULL", code).First(&u).Error
	if err != nil {
		return nil, mapErr(err)
	}
	return &u, nil
}

func (r *UserRepo) UpdateLogin(ctx context.Context, id uint64, ip string) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Model(&model.User{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"last_login_at": now,
			"last_login_ip": ip,
		}).Error
}

func (r *UserRepo) UpdatePassword(ctx context.Context, id uint64, hash string) error {
	return r.db.WithContext(ctx).Model(&model.User{}).
		Where("id = ?", id).
		Update("password", hash).Error
}

// ErrNotFound 显式语义。
var ErrNotFound = errors.New("repo: not found")

// mapErr 把 gorm.ErrRecordNotFound 映射为 ErrNotFound。
func mapErr(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}
