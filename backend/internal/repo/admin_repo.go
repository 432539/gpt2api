// Package repo 管理后台仓储。
package repo

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/kleinai/backend/internal/model"
)

// AdminRepo 后台账号仓储。
type AdminRepo struct{ db *gorm.DB }

// NewAdminRepo 构造。
func NewAdminRepo(db *gorm.DB) *AdminRepo { return &AdminRepo{db: db} }

// GetByUsername 用户名查询。
func (r *AdminRepo) GetByUsername(ctx context.Context, username string) (*model.AdminUser, error) {
	var u model.AdminUser
	err := r.db.WithContext(ctx).
		Where("username = ? AND deleted_at IS NULL", username).First(&u).Error
	if err != nil {
		return nil, mapErr(err)
	}
	return &u, nil
}

// GetByID 主键查询。
func (r *AdminRepo) GetByID(ctx context.Context, id uint64) (*model.AdminUser, error) {
	var u model.AdminUser
	err := r.db.WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", id).First(&u).Error
	if err != nil {
		return nil, mapErr(err)
	}
	return &u, nil
}

// UpdateLogin 写入最后登录信息。
func (r *AdminRepo) UpdateLogin(ctx context.Context, id uint64, ip string) error {
	return r.db.WithContext(ctx).Model(&model.AdminUser{}).
		Where("id = ?", id).Updates(map[string]any{
		"last_login_at": time.Now().UTC(),
		"last_login_ip": ip,
	}).Error
}

// GetRoleByID 角色查询。
func (r *AdminRepo) GetRoleByID(ctx context.Context, id uint64) (*model.AdminRole, error) {
	var role model.AdminRole
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&role).Error; err != nil {
		return nil, mapErr(err)
	}
	return &role, nil
}

// UpdatePassword updates the admin user's password hash.
func (r *AdminRepo) UpdatePassword(ctx context.Context, id uint64, hash string) error {
	return r.db.WithContext(ctx).Model(&model.AdminUser{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Update("password", hash).Error
}
