package repo

import (
	"context"
	"strings"

	"gorm.io/gorm"

	"github.com/kleinai/backend/internal/model"
)

type PromoRepo struct{ db *gorm.DB }

func NewPromoRepo(db *gorm.DB) *PromoRepo { return &PromoRepo{db: db} }

type PromoListFilter struct {
	Keyword      string
	Status       *int
	DiscountType *int
	Page         int
	PageSize     int
}

func (r *PromoRepo) List(ctx context.Context, f PromoListFilter) ([]*model.PromoCode, int64, error) {
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.PageSize <= 0 || f.PageSize > 200 {
		f.PageSize = 20
	}
	q := r.db.WithContext(ctx).Model(&model.PromoCode{})
	if f.Status != nil {
		q = q.Where("status = ?", *f.Status)
	}
	if f.DiscountType != nil {
		q = q.Where("discount_type = ?", *f.DiscountType)
	}
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("CAST(id AS CHAR) = ? OR code LIKE ? OR name LIKE ?", kw, like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []*model.PromoCode
	err := q.Order("id DESC").Offset((f.Page - 1) * f.PageSize).Limit(f.PageSize).Find(&rows).Error
	return rows, total, err
}

func (r *PromoRepo) Create(ctx context.Context, row *model.PromoCode) error {
	return r.db.WithContext(ctx).Create(row).Error
}

func (r *PromoRepo) Update(ctx context.Context, id uint64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.PromoCode{}).Where("id = ?", id).Updates(fields).Error
}

func (r *PromoRepo) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&model.PromoCode{}, id).Error
}
