// Package repo 数据访问层。
package repo

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/kleinai/backend/internal/model"
)

// AccountRepo 账号池仓储。
type AccountRepo struct{ db *gorm.DB }

// NewAccountRepo 构造。
func NewAccountRepo(db *gorm.DB) *AccountRepo { return &AccountRepo{db: db} }

// Create 创建。
func (r *AccountRepo) Create(ctx context.Context, a *model.Account) error {
	return r.db.WithContext(ctx).Create(a).Error
}

// BatchCreate 批量插入；忽略空切片。
func (r *AccountRepo) BatchCreate(ctx context.Context, items []*model.Account) error {
	if len(items) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(items, 200).Error
}

// GetByID 主键查询。
func (r *AccountRepo) GetByID(ctx context.Context, id uint64) (*model.Account, error) {
	var a model.Account
	err := r.db.WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", id).First(&a).Error
	if err != nil {
		return nil, mapErr(err)
	}
	return &a, nil
}

// AccountListFilter 列表过滤参数。
type AccountListFilter struct {
	Provider string
	Status   *int8
	Keyword  string
	Page     int
	PageSize int
}

// List 分页列表。
func (r *AccountRepo) List(ctx context.Context, f AccountListFilter) ([]*model.Account, int64, error) {
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.PageSize <= 0 || f.PageSize > 200 {
		f.PageSize = 20
	}
	q := r.db.WithContext(ctx).Model(&model.Account{}).Where("deleted_at IS NULL")
	if f.Provider != "" {
		q = q.Where("provider = ?", f.Provider)
	}
	if f.Status != nil {
		q = q.Where("status = ?", *f.Status)
	}
	if f.Keyword != "" {
		k := "%" + f.Keyword + "%"
		q = q.Where("(name LIKE ? OR remark LIKE ?)", k, k)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []*model.Account
	if err := q.Order("id DESC").
		Offset((f.Page - 1) * f.PageSize).Limit(f.PageSize).
		Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// Update 部分字段更新。
func (r *AccountRepo) Update(ctx context.Context, id uint64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.Account{}).
		Where("id = ?", id).Updates(fields).Error
}

// SoftDelete 软删除。
func (r *AccountRepo) SoftDelete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Model(&model.Account{}).
		Where("id = ?", id).Update("deleted_at", time.Now().UTC()).Error
}

// AvailableByProvider 拿出给定 provider 下当前可用的账号（用于调度器装载）。
func (r *AccountRepo) AvailableByProvider(ctx context.Context, provider string) ([]*model.Account, error) {
	var items []*model.Account
	now := time.Now().UTC()
	err := r.db.WithContext(ctx).
		Where("provider = ? AND status = ? AND deleted_at IS NULL", provider, model.AccountStatusEnabled).
		Where("cooldown_until IS NULL OR cooldown_until <= ?", now).
		Order("id ASC").
		Find(&items).Error
	return items, err
}

// MarkUsed 标记调度成功。
func (r *AccountRepo) MarkUsed(ctx context.Context, id uint64) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Model(&model.Account{}).
		Where("id = ?", id).Updates(map[string]any{
		"last_used_at":  now,
		"success_count": gorm.Expr("success_count + 1"),
	}).Error
}

// MarkFailed 标记调度失败 / 进入熔断。
func (r *AccountRepo) MarkFailed(ctx context.Context, id uint64, reason string, cooldown time.Duration) error {
	now := time.Now().UTC()
	fields := map[string]any{
		"error_count": gorm.Expr("error_count + 1"),
		"last_error":  reason,
	}
	if cooldown > 0 {
		until := now.Add(cooldown)
		fields["cooldown_until"] = until
		fields["status"] = model.AccountStatusBroken
	}
	return r.db.WithContext(ctx).Model(&model.Account{}).
		Where("id = ?", id).Updates(fields).Error
}
