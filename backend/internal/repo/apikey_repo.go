// Package repo API Key 仓储。
package repo

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/kleinai/backend/internal/model"
)

// APIKeyRepo API Key 数据访问层。
type APIKeyRepo struct{ db *gorm.DB }

// NewAPIKeyRepo 构造。
func NewAPIKeyRepo(db *gorm.DB) *APIKeyRepo { return &APIKeyRepo{db: db} }

// Create 创建。
func (r *APIKeyRepo) Create(ctx context.Context, k *model.APIKey) error {
	return r.db.WithContext(ctx).Create(k).Error
}

// GetByHash 通过 hash 查 key（用于鉴权）。
func (r *APIKeyRepo) GetByHash(ctx context.Context, hash string) (*model.APIKey, error) {
	var k model.APIKey
	err := r.db.WithContext(ctx).
		Where("hash = ? AND deleted_at IS NULL", hash).First(&k).Error
	if err != nil {
		return nil, mapErr(err)
	}
	return &k, nil
}

// ListByUser 用户拥有的 keys。
func (r *APIKeyRepo) ListByUser(ctx context.Context, userID uint64) ([]*model.APIKey, error) {
	var items []*model.APIKey
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Order("id DESC").
		Find(&items).Error
	return items, err
}

// GetByID 主键查（含 user_id 校验）。
func (r *APIKeyRepo) GetByID(ctx context.Context, userID, id uint64) (*model.APIKey, error) {
	var k model.APIKey
	err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ? AND deleted_at IS NULL", id, userID).First(&k).Error
	if err != nil {
		return nil, mapErr(err)
	}
	return &k, nil
}

// UpdateStatus 启用 / 禁用。
func (r *APIKeyRepo) UpdateStatus(ctx context.Context, userID, id uint64, status int8) error {
	return r.db.WithContext(ctx).Model(&model.APIKey{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("status", status).Error
}

// SoftDelete 软删除。
func (r *APIKeyRepo) SoftDelete(ctx context.Context, userID, id uint64) error {
	return r.db.WithContext(ctx).Model(&model.APIKey{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("deleted_at", time.Now().UTC()).Error
}

// MarkUsed 异步：更新 last_used_at（不阻塞鉴权）。
func (r *APIKeyRepo) MarkUsed(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Model(&model.APIKey{}).
		Where("id = ?", id).
		Update("last_used_at", time.Now().UTC()).Error
}

// ListByLast4 通过 last4 + status=1 拿候选（鉴权用）。
func (r *APIKeyRepo) ListByLast4(ctx context.Context, last4 string) ([]*model.APIKey, error) {
	var items []*model.APIKey
	err := r.db.WithContext(ctx).
		Where("last4 = ? AND status = 1 AND deleted_at IS NULL", last4).
		Find(&items).Error
	return items, err
}
