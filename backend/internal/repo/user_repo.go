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

type UserListFilter struct {
	Keyword  string
	Status   *int
	Page     int
	PageSize int
}

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

func (r *UserRepo) List(ctx context.Context, f UserListFilter) ([]*model.User, int64, error) {
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.PageSize <= 0 || f.PageSize > 200 {
		f.PageSize = 20
	}
	q := r.db.WithContext(ctx).Model(&model.User{}).Where("deleted_at IS NULL")
	if f.Status != nil {
		q = q.Where("status = ?", *f.Status)
	}
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("CAST(id AS CHAR) = ? OR uuid = ? OR email LIKE ? OR phone LIKE ? OR username LIKE ? OR invite_code = ?",
			kw, kw, like, like, like, kw)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []*model.User
	err := q.Order("id DESC").Offset((f.Page - 1) * f.PageSize).Limit(f.PageSize).Find(&items).Error
	return items, total, err
}

func (r *UserRepo) Update(ctx context.Context, id uint64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.User{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(fields).Error
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
