// Package repo 生成任务仓储。
package repo

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/kleinai/backend/internal/model"
)

// GenerationRepo 生成任务仓储。
type GenerationRepo struct{ db *gorm.DB }

// NewGenerationRepo 构造。
func NewGenerationRepo(db *gorm.DB) *GenerationRepo { return &GenerationRepo{db: db} }

// Create 创建任务。
func (r *GenerationRepo) Create(ctx context.Context, t *model.GenerationTask) error {
	return r.db.WithContext(ctx).Create(t).Error
}

// GetByTaskID 通过 task_id 查询。
func (r *GenerationRepo) GetByTaskID(ctx context.Context, taskID string) (*model.GenerationTask, error) {
	var t model.GenerationTask
	err := r.db.WithContext(ctx).
		Where("task_id = ? AND deleted_at IS NULL", taskID).First(&t).Error
	if err != nil {
		return nil, mapErr(err)
	}
	return &t, nil
}

// GetByIdem 幂等查询：(user_id, idem_key)。
func (r *GenerationRepo) GetByIdem(ctx context.Context, userID uint64, idem string) (*model.GenerationTask, error) {
	var t model.GenerationTask
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND idem_key = ? AND deleted_at IS NULL", userID, idem).First(&t).Error
	if err != nil {
		return nil, mapErr(err)
	}
	return &t, nil
}

// SetRunning 标记任务进入运行态。
func (r *GenerationRepo) SetRunning(ctx context.Context, taskID string, accountID uint64) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Model(&model.GenerationTask{}).
		Where("task_id = ? AND status = ?", taskID, model.GenStatusPending).
		Updates(map[string]any{
			"status":     model.GenStatusRunning,
			"account_id": accountID,
			"started_at": now,
			"progress":   5,
		}).Error
}

// UpdateProgress 更新进度（0-100）。
func (r *GenerationRepo) UpdateProgress(ctx context.Context, taskID string, progress int8) error {
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}
	return r.db.WithContext(ctx).Model(&model.GenerationTask{}).
		Where("task_id = ?", taskID).Update("progress", progress).Error
}

// SetSucceeded 任务成功 + 写入结果。
func (r *GenerationRepo) SetSucceeded(ctx context.Context, taskID string, results []*model.GenerationResult) error {
	if taskID == "" {
		return errors.New("empty task_id")
	}
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.GenerationTask{}).
			Where("task_id = ?", taskID).
			Updates(map[string]any{
				"status":      model.GenStatusSucceeded,
				"progress":    100,
				"finished_at": now,
			}).Error; err != nil {
			return err
		}
		if len(results) > 0 {
			return tx.CreateInBatches(results, 100).Error
		}
		return nil
	})
}

// SetFailed 任务失败。
func (r *GenerationRepo) SetFailed(ctx context.Context, taskID, reason string) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Model(&model.GenerationTask{}).
		Where("task_id = ?", taskID).
		Updates(map[string]any{
			"status":      model.GenStatusFailed,
			"error":       truncateStr(reason, 240),
			"finished_at": now,
		}).Error
}

// ListByUser 用户任务列表。
func (r *GenerationRepo) ListByUser(ctx context.Context, userID uint64, kind string, page, pageSize int) ([]*model.GenerationTask, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	q := r.db.WithContext(ctx).Model(&model.GenerationTask{}).
		Where("user_id = ? AND deleted_at IS NULL", userID)
	if kind != "" {
		q = q.Where("kind = ?", kind)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []*model.GenerationTask
	err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error
	return items, total, err
}

// ListResultsByTask 查询结果列表。
func (r *GenerationRepo) ListResultsByTask(ctx context.Context, taskID string) ([]*model.GenerationResult, error) {
	var items []*model.GenerationResult
	err := r.db.WithContext(ctx).
		Where("task_id = ?", taskID).Order("seq ASC, id ASC").Find(&items).Error
	return items, err
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
