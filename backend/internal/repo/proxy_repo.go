package repo

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/kleinai/backend/internal/model"
)

// ProxyRepo 代理仓储。
type ProxyRepo struct{ db *gorm.DB }

// NewProxyRepo 构造。
func NewProxyRepo(db *gorm.DB) *ProxyRepo { return &ProxyRepo{db: db} }

// Create 新增。
func (r *ProxyRepo) Create(ctx context.Context, p *model.Proxy) error {
	return r.db.WithContext(ctx).Create(p).Error
}

// GetByID 主键查询（未软删）。
func (r *ProxyRepo) GetByID(ctx context.Context, id uint64) (*model.Proxy, error) {
	var p model.Proxy
	err := r.db.WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", id).First(&p).Error
	if err != nil {
		return nil, mapErr(err)
	}
	return &p, nil
}

// ProxyListFilter 列表过滤。
type ProxyListFilter struct {
	Status   *int8
	Keyword  string
	Page     int
	PageSize int
}

// List 分页列表。
func (r *ProxyRepo) List(ctx context.Context, f ProxyListFilter) ([]*model.Proxy, int64, error) {
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.PageSize <= 0 || f.PageSize > 200 {
		f.PageSize = 50
	}
	q := r.db.WithContext(ctx).Model(&model.Proxy{}).Where("deleted_at IS NULL")
	if f.Status != nil {
		q = q.Where("status = ?", *f.Status)
	}
	if f.Keyword != "" {
		k := "%" + f.Keyword + "%"
		q = q.Where("(name LIKE ? OR host LIKE ? OR remark LIKE ?)", k, k, k)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []*model.Proxy
	if err := q.Order("id DESC").
		Offset((f.Page - 1) * f.PageSize).Limit(f.PageSize).
		Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ListEnabled 取所有启用代理（用于下拉、调度）。
func (r *ProxyRepo) ListEnabled(ctx context.Context) ([]*model.Proxy, error) {
	var items []*model.Proxy
	err := r.db.WithContext(ctx).
		Where("status = ? AND deleted_at IS NULL", model.ProxyStatusEnabled).
		Order("id ASC").Find(&items).Error
	return items, err
}

// Update 部分字段更新。
func (r *ProxyRepo) Update(ctx context.Context, id uint64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.Proxy{}).
		Where("id = ?", id).Updates(fields).Error
}

// SoftDelete 软删除。
func (r *ProxyRepo) SoftDelete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Model(&model.Proxy{}).
		Where("id = ?", id).Update("deleted_at", time.Now().UTC()).Error
}

// MarkCheck 记录探测结果。
func (r *ProxyRepo) MarkCheck(ctx context.Context, id uint64, ok bool, latencyMs int, errMsg string) error {
	now := time.Now().UTC()
	st := model.ProxyCheckOK
	if !ok {
		st = model.ProxyCheckFail
	}
	fields := map[string]any{
		"last_check_at":  now,
		"last_check_ok":  st,
		"last_check_ms":  latencyMs,
		"last_error":     errMsg,
	}
	return r.db.WithContext(ctx).Model(&model.Proxy{}).
		Where("id = ?", id).Updates(fields).Error
}
