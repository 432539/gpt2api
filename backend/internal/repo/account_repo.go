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
	if f.PageSize <= 0 || f.PageSize > 1000 {
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

// SoftDeleteMany 按 ID 列表批量软删（仅未删除行）。
func (r *AccountRepo) SoftDeleteMany(ctx context.Context, ids []uint64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	now := time.Now().UTC()
	res := r.db.WithContext(ctx).Model(&model.Account{}).
		Where("id IN ? AND deleted_at IS NULL", ids).
		Update("deleted_at", now)
	return res.RowsAffected, res.Error
}

// SoftDeleteInvalid 软删：已禁用、熔断、或最近连通测试失败。
func (r *AccountRepo) SoftDeleteInvalid(ctx context.Context, provider string) (int64, error) {
	now := time.Now().UTC()
	q := r.db.WithContext(ctx).Model(&model.Account{}).Where("deleted_at IS NULL").
		Where("(last_test_status = ? OR status IN (?, ?))",
			model.AccountTestFail, model.AccountStatusDisabled, model.AccountStatusBroken)
	if provider != "" {
		q = q.Where("provider = ?", provider)
	}
	res := q.Update("deleted_at", now)
	return res.RowsAffected, res.Error
}

// SoftDeleteAll 软删当前条件下所有账号（未删行）。provider 空表示两池全量。
// SoftDeleteZeroQuota soft-deletes accounts that have been quota-probed and have no remaining image quota.
func (r *AccountRepo) SoftDeleteZeroQuota(ctx context.Context, provider string) (int64, error) {
	now := time.Now().UTC()
	q := r.db.WithContext(ctx).Model(&model.Account{}).Where("deleted_at IS NULL").
		Where("oauth_meta IS NOT NULL").
		Where("JSON_EXTRACT(oauth_meta, '$.image_quota_remaining') IS NOT NULL").
		Where("CAST(JSON_UNQUOTE(JSON_EXTRACT(oauth_meta, '$.image_quota_remaining')) AS SIGNED) <= 0")
	if provider != "" {
		q = q.Where("provider = ?", provider)
	}
	res := q.Update("deleted_at", now)
	return res.RowsAffected, res.Error
}

func (r *AccountRepo) SoftDeleteAll(ctx context.Context, provider string) (int64, error) {
	now := time.Now().UTC()
	q := r.db.WithContext(ctx).Model(&model.Account{}).Where("deleted_at IS NULL")
	if provider != "" {
		q = q.Where("provider = ?", provider)
	}
	res := q.Update("deleted_at", now)
	return res.RowsAffected, res.Error
}

// AvailableByProvider 拿出给定 provider 下当前可用的账号（用于调度器装载）。
func (r *AccountRepo) AvailableByProvider(ctx context.Context, provider string) ([]*model.Account, error) {
	var items []*model.Account
	now := time.Now().UTC()
	err := r.db.WithContext(ctx).
		Where("provider = ? AND deleted_at IS NULL", provider).
		Where("(status = ? OR (status = ? AND cooldown_until IS NOT NULL AND cooldown_until <= ?))", model.AccountStatusEnabled, model.AccountStatusBroken, now).
		Where("cooldown_until IS NULL OR cooldown_until <= ?", now).
		Where("access_token_expires_at IS NULL OR access_token_expires_at > ?", now).
		Order("id ASC").
		Find(&items).Error
	return items, err
}

// MarkUsed 标记调度成功。
func (r *AccountRepo) MarkUsed(ctx context.Context, id uint64) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Model(&model.Account{}).
		Where("id = ?", id).Updates(map[string]any{
		"last_used_at":   now,
		"success_count":  gorm.Expr("success_count + 1"),
		"error_count":    0,
		"status":         model.AccountStatusEnabled,
		"cooldown_until": nil,
		"last_error":     nil,
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
	} else {
		fields["cooldown_until"] = nil
		fields["status"] = model.AccountStatusEnabled
	}
	return r.db.WithContext(ctx).Model(&model.Account{}).
		Where("id = ?", id).Updates(fields).Error
}
