// Package service 计费引擎核心：预扣 / 结算 / 退款。
//
// 流程（与 docs/02-后端规范.md §6 对齐）：
//
//   1. PreDeduct  → wallet.Freeze + 写 consume_record(status=0)
//   2. (上游调用)
//   3. Settle     → wallet.Settle  + 更新 consume_record(status=1)
//   4. Failure    → wallet.Refund  + 更新 consume_record(status=2) + 写 refund_record
//
// 任务 ID（task_id）在调用方生成，应使用 ULID 字符串。
package service

import (
	"context"
	"errors"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/kleinai/backend/internal/model"
	"github.com/kleinai/backend/internal/repo"
	"github.com/kleinai/backend/pkg/errcode"
	"github.com/kleinai/backend/pkg/logger"
)

// BillingService 计费引擎。
type BillingService struct {
	db     *gorm.DB
	wallet *repo.WalletRepo
}

// NewBillingService 构造。
func NewBillingService(db *gorm.DB, w *repo.WalletRepo) *BillingService {
	return &BillingService{db: db, wallet: w}
}

// PreDeductReq 预扣点请求。
type PreDeductReq struct {
	UserID      uint64
	TaskID      string
	Kind        string // image / video
	ModelCode   string
	Count       int
	UnitPoints  int64
}

// PreDeduct 预扣点 + 写 consume_record(status=frozen)。
func (s *BillingService) PreDeduct(ctx context.Context, req PreDeductReq) error {
	total := req.UnitPoints * int64(req.Count)
	if total <= 0 {
		return errcode.InvalidParam.WithMsg("invalid cost")
	}

	if _, err := s.wallet.Freeze(ctx, req.UserID, model.BizConsume, req.TaskID, total, req.ModelCode); err != nil {
		if errors.Is(err, repo.ErrInsufficient) {
			return errcode.InsufficientPoints
		}
		return errcode.DBError.Wrap(err)
	}

	rec := &model.ConsumeRecord{
		TaskID:      req.TaskID,
		UserID:      req.UserID,
		Kind:        req.Kind,
		ModelCode:   req.ModelCode,
		Count:       req.Count,
		UnitPoints:  req.UnitPoints,
		TotalPoints: total,
		Status:      model.ConsumeStatusFrozen,
	}
	if err := s.db.WithContext(ctx).Create(rec).Error; err != nil {
		// 写 consume_record 失败需要回滚已 freeze 的点
		_ = s.wallet.Refund(ctx, req.UserID, req.TaskID, "rollback consume_record failed", total)
		return errcode.DBError.Wrap(err)
	}
	return nil
}

// Settle 结算消费：将 frozen → 落地。account_id 可写入实际消耗的池中账号。
func (s *BillingService) Settle(ctx context.Context, taskID string, accountID *uint64) error {
	var rec model.ConsumeRecord
	if err := s.db.WithContext(ctx).Where("task_id = ?", taskID).First(&rec).Error; err != nil {
		return errcode.ResourceMissing
	}
	if rec.Status != model.ConsumeStatusFrozen {
		// 已结算或退款，幂等返回
		return nil
	}
	if err := s.wallet.Settle(ctx, rec.UserID, rec.TotalPoints); err != nil {
		return errcode.DBError.Wrap(err)
	}
	updates := map[string]any{"status": model.ConsumeStatusSettled}
	if accountID != nil {
		updates["account_id"] = *accountID
	}
	if err := s.db.WithContext(ctx).Model(&model.ConsumeRecord{}).
		Where("task_id = ?", taskID).Updates(updates).Error; err != nil {
		return errcode.DBError.Wrap(err)
	}
	logger.FromCtx(ctx).Info("billing.settle", zap.String("task", taskID), zap.Int64("points", rec.TotalPoints))
	return nil
}

// FailRefund 失败退款：解冻 + 退还 + 标记 status=refunded。
func (s *BillingService) FailRefund(ctx context.Context, taskID, reason string) error {
	var rec model.ConsumeRecord
	if err := s.db.WithContext(ctx).Where("task_id = ?", taskID).First(&rec).Error; err != nil {
		return errcode.ResourceMissing
	}
	if rec.Status != model.ConsumeStatusFrozen {
		return nil
	}
	if err := s.wallet.Refund(ctx, rec.UserID, taskID, reason, rec.TotalPoints); err != nil {
		return errcode.DBError.Wrap(err)
	}
	if err := s.db.WithContext(ctx).Model(&model.ConsumeRecord{}).
		Where("task_id = ?", taskID).
		Update("status", model.ConsumeStatusRefunded).Error; err != nil {
		return errcode.DBError.Wrap(err)
	}
	logger.FromCtx(ctx).Info("billing.refund", zap.String("task", taskID), zap.Int64("points", rec.TotalPoints), zap.String("reason", reason))
	return nil
}

// GrantPoints 赠送 / 兑换码 / 邀请奖励：直接增加点数 + 写流水。
func (s *BillingService) GrantPoints(ctx context.Context, userID uint64, biz, bizID string, points int64, remark string) error {
	if _, err := s.wallet.Income(ctx, userID, biz, bizID, points, remark); err != nil {
		return errcode.DBError.Wrap(err)
	}
	return nil
}

// ListWalletLogs 用户钱包流水。
func (s *BillingService) ListWalletLogs(ctx context.Context, userID uint64, page, pageSize int) ([]*model.WalletLog, int64, error) {
	logs, total, err := s.wallet.ListUserLogs(ctx, userID, page, pageSize)
	if err != nil {
		return nil, 0, errcode.DBError.Wrap(err)
	}
	return logs, total, nil
}
