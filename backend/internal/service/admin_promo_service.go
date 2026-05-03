package service

import (
	"context"
	"strings"
	"time"

	"github.com/kleinai/backend/internal/dto"
	"github.com/kleinai/backend/internal/model"
	"github.com/kleinai/backend/internal/repo"
	"github.com/kleinai/backend/pkg/errcode"
)

type AdminPromoService struct {
	repo *repo.PromoRepo
}

func NewAdminPromoService(r *repo.PromoRepo) *AdminPromoService {
	return &AdminPromoService{repo: r}
}

func (s *AdminPromoService) List(ctx context.Context, req *dto.AdminPromoListReq) ([]*dto.AdminPromoResp, int64, error) {
	rows, total, err := s.repo.List(ctx, repo.PromoListFilter{
		Keyword:      req.Keyword,
		Status:       req.Status,
		DiscountType: req.DiscountType,
		Page:         req.Page,
		PageSize:     req.PageSize,
	})
	if err != nil {
		return nil, 0, errcode.DBError.Wrap(err)
	}
	out := make([]*dto.AdminPromoResp, 0, len(rows))
	for _, row := range rows {
		out = append(out, promoResp(row))
	}
	return out, total, nil
}

func (s *AdminPromoService) Create(ctx context.Context, req *dto.AdminPromoCreateReq, adminID uint64) (*model.PromoCode, error) {
	now := time.Now().UTC()
	start := now
	if req.StartAt > 0 {
		start = time.Unix(req.StartAt, 0).UTC()
	}
	end := time.Unix(req.EndAt, 0).UTC()
	if !end.After(start) {
		return nil, errcode.InvalidParam.WithMsg("结束时间必须晚于开始时间")
	}
	status := int8(model.PromoStatusEnabled)
	if req.Status != nil {
		status = *req.Status
	}
	applyTo := strings.TrimSpace(req.ApplyTo)
	if applyTo == "" {
		applyTo = "all"
	}
	code := strings.ToUpper(strings.TrimSpace(req.Code))
	row := &model.PromoCode{
		Code:         code,
		Name:         strings.TrimSpace(req.Name),
		DiscountType: req.DiscountType,
		DiscountVal:  req.DiscountVal,
		MinAmount:    req.MinAmount,
		ApplyTo:      applyTo,
		TotalQty:     req.TotalQty,
		PerUserLimit: req.PerUserLimit,
		StartAt:      start,
		EndAt:        end,
		Status:       status,
		CreatedBy:    &adminID,
	}
	if row.PerUserLimit == 0 {
		row.PerUserLimit = 1
	}
	if err := s.repo.Create(ctx, row); err != nil {
		return nil, errcode.DBError.Wrap(err)
	}
	return row, nil
}

func (s *AdminPromoService) Update(ctx context.Context, id uint64, req *dto.AdminPromoUpdateReq) error {
	fields := map[string]any{}
	if req.Code != nil {
		fields["code"] = strings.ToUpper(strings.TrimSpace(*req.Code))
	}
	if req.Name != nil {
		fields["name"] = strings.TrimSpace(*req.Name)
	}
	if req.DiscountType != nil {
		fields["discount_type"] = *req.DiscountType
	}
	if req.DiscountVal != nil {
		fields["discount_val"] = *req.DiscountVal
	}
	if req.MinAmount != nil {
		fields["min_amount"] = *req.MinAmount
	}
	if req.ApplyTo != nil {
		applyTo := strings.TrimSpace(*req.ApplyTo)
		if applyTo == "" {
			applyTo = "all"
		}
		fields["apply_to"] = applyTo
	}
	if req.TotalQty != nil {
		fields["total_qty"] = *req.TotalQty
	}
	if req.PerUserLimit != nil {
		fields["per_user_limit"] = *req.PerUserLimit
	}
	if req.StartAt != nil {
		fields["start_at"] = time.Unix(*req.StartAt, 0).UTC()
	}
	if req.EndAt != nil {
		fields["end_at"] = time.Unix(*req.EndAt, 0).UTC()
	}
	if req.Status != nil {
		fields["status"] = *req.Status
	}
	if err := s.repo.Update(ctx, id, fields); err != nil {
		return errcode.DBError.Wrap(err)
	}
	return nil
}

func (s *AdminPromoService) Delete(ctx context.Context, id uint64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return errcode.DBError.Wrap(err)
	}
	return nil
}

func promoResp(row *model.PromoCode) *dto.AdminPromoResp {
	return &dto.AdminPromoResp{
		ID:           row.ID,
		Code:         row.Code,
		Name:         row.Name,
		DiscountType: row.DiscountType,
		DiscountVal:  row.DiscountVal,
		MinAmount:    row.MinAmount,
		ApplyTo:      row.ApplyTo,
		TotalQty:     row.TotalQty,
		UsedQty:      row.UsedQty,
		PerUserLimit: row.PerUserLimit,
		StartAt:      row.StartAt.Unix(),
		EndAt:        row.EndAt.Unix(),
		Status:       row.Status,
		CreatedAt:    row.CreatedAt.Unix(),
		UpdatedAt:    row.UpdatedAt.Unix(),
	}
}
