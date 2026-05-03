// Package repo 钱包流水仓储。
package repo

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/kleinai/backend/internal/model"
)

// WalletRepo 钱包 / 流水仓储。
type WalletRepo struct{ db *gorm.DB }

// NewWalletRepo 构造。
func NewWalletRepo(db *gorm.DB) *WalletRepo { return &WalletRepo{db: db} }

// ErrInsufficient 余额不足。
var ErrInsufficient = errors.New("repo: insufficient points")

// Income 在事务中给用户加点 + 写入流水。
func (r *WalletRepo) Income(ctx context.Context, userID uint64, biz, bizID string, points int64, remark string) (*model.WalletLog, error) {
	if points <= 0 {
		return nil, errors.New("income points must >0")
	}
	var log *model.WalletLog
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		u, err := lockUser(tx, userID)
		if err != nil {
			return err
		}
		before := u.Points
		after := before + points
		if err := tx.Model(&model.User{}).Where("id = ?", userID).
			UpdateColumn("points", after).Error; err != nil {
			return err
		}
		log = &model.WalletLog{
			UserID:       userID,
			Direction:    1,
			BizType:      biz,
			BizID:        bizID,
			Points:       points,
			PointsBefore: before,
			PointsAfter:  after,
		}
		if remark != "" {
			log.Remark = &remark
		}
		return tx.Create(log).Error
	})
	return log, err
}

// Freeze 预冻结：从 points 扣，写入 frozen_points + wallet_log（dir=-1 / status=frozen 由 service 控制）。
//
// 失败原因：
//   - ErrInsufficient: 余额不足
func (r *WalletRepo) Freeze(ctx context.Context, userID uint64, biz, bizID string, points int64, remark string) (*model.WalletLog, error) {
	if points <= 0 {
		return nil, errors.New("freeze points must >0")
	}
	var log *model.WalletLog
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		u, err := lockUser(tx, userID)
		if err != nil {
			return err
		}
		if u.Points < points {
			return ErrInsufficient
		}
		before := u.Points
		after := before - points
		if err := tx.Model(&model.User{}).Where("id = ?", userID).
			UpdateColumns(map[string]any{
				"points":        after,
				"frozen_points": gorm.Expr("frozen_points + ?", points),
			}).Error; err != nil {
			return err
		}
		log = &model.WalletLog{
			UserID:       userID,
			Direction:    -1,
			BizType:      biz,
			BizID:        bizID,
			Points:       -points,
			PointsBefore: before,
			PointsAfter:  after,
		}
		if remark != "" {
			log.Remark = &remark
		}
		return tx.Create(log).Error
	})
	return log, err
}

// Settle 结算：将之前 freeze 的 points 从 frozen_points 中清掉（落地为消费）。
func (r *WalletRepo) Settle(ctx context.Context, userID uint64, points int64) error {
	if points <= 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		u, err := lockUser(tx, userID)
		if err != nil {
			return err
		}
		newFrozen := u.FrozenPoints - points
		if newFrozen < 0 {
			newFrozen = 0
		}
		return tx.Model(&model.User{}).Where("id = ?", userID).
			Update("frozen_points", newFrozen).Error
	})
}

// Refund 退款：把 freeze 的 points 还回 points + 写入 wallet_log + 写 refund_record。
func (r *WalletRepo) Refund(ctx context.Context, userID uint64, taskID, reason string, points int64) error {
	if points <= 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		u, err := lockUser(tx, userID)
		if err != nil {
			return err
		}
		newFrozen := u.FrozenPoints - points
		if newFrozen < 0 {
			newFrozen = 0
		}
		before := u.Points
		after := before + points
		if err := tx.Model(&model.User{}).Where("id = ?", userID).
			UpdateColumns(map[string]any{
				"points":        after,
				"frozen_points": newFrozen,
			}).Error; err != nil {
			return err
		}
		remarkCopy := reason
		log := &model.WalletLog{
			UserID:       userID,
			Direction:    1,
			BizType:      model.BizRefund,
			BizID:        taskID,
			Points:       points,
			PointsBefore: before,
			PointsAfter:  after,
			Remark:       &remarkCopy,
		}
		if err := tx.Create(log).Error; err != nil {
			return err
		}
		return tx.Create(&model.RefundRecord{
			TaskID:    taskID,
			UserID:    userID,
			Points:    points,
			Reason:    reason,
			Operator:  "system",
			CreatedAt: time.Now().UTC(),
		}).Error
	})
}

// ListUserLogs 钱包流水分页。
func (r *WalletRepo) ListUserLogs(ctx context.Context, userID uint64, page, pageSize int) ([]*model.WalletLog, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}
	q := r.db.WithContext(ctx).Model(&model.WalletLog{}).Where("user_id = ?", userID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []*model.WalletLog
	err := q.Order("id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&items).Error
	return items, total, err
}

// === helpers ===

// lockUser SELECT ... FOR UPDATE。
func lockUser(tx *gorm.DB, userID uint64) (*model.User, error) {
	var u model.User
	if err := tx.Set("gorm:query_option", "FOR UPDATE").
		Where("id = ?", userID).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}
