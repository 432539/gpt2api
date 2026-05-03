// Package repo 生成任务仓储。
package repo

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/kleinai/backend/internal/model"
)

// GenerationRepo 生成任务仓储。
type GenerationRepo struct{ db *gorm.DB }

type AdminGenerationLogFilter struct {
	Keyword  string
	Kind     string
	Status   *int
	Page     int
	PageSize int
}

type AdminGenerationLogRow struct {
	TaskID     string
	CreatedAt  time.Time
	UserID     uint64
	UserLabel  string
	APIKeyID   *uint64
	KeyLabel   *string
	Kind       string
	ModelCode  string
	Prompt     string
	Status     int8
	DurationMs *int64
	CostPoints int64
	PreviewURL *string
	Error      *string
}

type AdminGenerationUpstreamLogRow struct {
	ID              uint64
	TaskID          string
	Provider        string
	AccountID       *uint64
	Stage           string
	Method          *string
	URL             *string
	StatusCode      int
	DurationMs      int64
	RequestExcerpt  *string
	ResponseExcerpt *string
	Error           *string
	Meta            *string
	CreatedAt       time.Time
}

// NewGenerationRepo 构造。
func NewGenerationRepo(db *gorm.DB) *GenerationRepo { return &GenerationRepo{db: db} }

func (r *GenerationRepo) CreateUpstreamLog(ctx context.Context, log *model.GenerationUpstreamLog) error {
	if log == nil || log.TaskID == "" {
		return nil
	}
	return r.db.WithContext(ctx).Create(log).Error
}

func (r *GenerationRepo) ListUpstreamLogs(ctx context.Context, taskID string) ([]*AdminGenerationUpstreamLogRow, error) {
	var rows []*AdminGenerationUpstreamLogRow
	err := r.db.WithContext(ctx).Table("generation_upstream_log").
		Where("task_id = ?", taskID).
		Order("id ASC").
		Find(&rows).Error
	return rows, err
}

func (r *GenerationRepo) ListAdminLogs(ctx context.Context, f AdminGenerationLogFilter) ([]*AdminGenerationLogRow, int64, error) {
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.PageSize <= 0 || f.PageSize > 200 {
		f.PageSize = 20
	}

	where := []string{"t.deleted_at IS NULL"}
	args := []any{}
	if f.Kind != "" {
		where = append(where, "t.kind = ?")
		args = append(args, f.Kind)
	}
	if f.Status != nil {
		where = append(where, "t.status = ?")
		args = append(args, *f.Status)
	}
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		like := "%" + kw + "%"
		where = append(where, `(t.task_id = ? OR CAST(t.user_id AS CHAR) = ? OR t.model_code LIKE ? OR t.prompt LIKE ? OR u.email LIKE ? OR u.phone LIKE ? OR u.username LIKE ? OR k.name LIKE ? OR k.last4 = ?)`)
		args = append(args, kw, kw, like, like, like, like, like, like, kw)
	}
	whereSQL := strings.Join(where, " AND ")

	var total int64
	countSQL := `SELECT COUNT(1)
FROM generation_task t
LEFT JOIN ` + "`user`" + ` u ON u.id = t.user_id
LEFT JOIN api_key k ON k.id = t.from_api_key_id
WHERE ` + whereSQL
	if err := r.db.WithContext(ctx).Raw(countSQL, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, (f.Page-1)*f.PageSize, f.PageSize)
	querySQL := `SELECT
  t.task_id,
  t.created_at,
  t.user_id,
  COALESCE(NULLIF(u.username, ''), NULLIF(u.email, ''), NULLIF(u.phone, ''), CONCAT('用户 #', t.user_id)) AS user_label,
  t.from_api_key_id AS api_key_id,
  CASE WHEN k.id IS NULL THEN NULL ELSE CONCAT(k.name, ' · ', k.prefix, '…', k.last4) END AS key_label,
  t.kind,
  t.model_code,
  t.prompt,
  t.status,
  CASE
    WHEN t.started_at IS NULL THEN NULL
    ELSE TIMESTAMPDIFF(MICROSECOND, t.started_at, COALESCE(t.finished_at, t.updated_at)) DIV 1000
  END AS duration_ms,
  t.cost_points,
  (SELECT COALESCE(r.thumb_url, r.url) FROM generation_result r WHERE r.task_id = t.task_id AND r.deleted_at IS NULL ORDER BY r.seq ASC, r.id ASC LIMIT 1) AS preview_url,
  t.error
FROM generation_task t
LEFT JOIN ` + "`user`" + ` u ON u.id = t.user_id
LEFT JOIN api_key k ON k.id = t.from_api_key_id
WHERE ` + whereSQL + `
ORDER BY t.id DESC
LIMIT ?, ?`
	var rows []*AdminGenerationLogRow
	if err := r.db.WithContext(ctx).Raw(querySQL, queryArgs...).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// SoftDeleteAdminLogsBefore marks generation logs and their result rows as deleted.
func (r *GenerationRepo) SoftDeleteAdminLogsBefore(ctx context.Context, before time.Time) (int64, error) {
	now := time.Now().UTC()
	var deleted int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var taskIDs []string
		if err := tx.Model(&model.GenerationTask{}).
			Where("deleted_at IS NULL AND created_at < ?", before).
			Pluck("task_id", &taskIDs).Error; err != nil {
			return err
		}
		if len(taskIDs) == 0 {
			return nil
		}
		taskRes := tx.Model(&model.GenerationTask{}).
			Where("deleted_at IS NULL AND task_id IN ?", taskIDs).
			Update("deleted_at", now)
		if taskRes.Error != nil {
			return taskRes.Error
		}
		deleted = taskRes.RowsAffected
		if err := tx.Table("generation_result").
			Where("deleted_at IS NULL AND task_id IN ?", taskIDs).
			Update("deleted_at", now).Error; err != nil {
			return err
		}
		return nil
	})
	return deleted, err
}

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

// UpdateCost updates final task cost after usage-based billing.
func (r *GenerationRepo) UpdateCost(ctx context.Context, taskID string, cost int64) error {
	if cost < 0 {
		cost = 0
	}
	return r.db.WithContext(ctx).Model(&model.GenerationTask{}).
		Where("task_id = ?", taskID).
		Update("cost_points", cost).Error
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
	if kind == "media" {
		q = q.Where("kind IN ?", []string{"image", "video"})
	} else if kind != "" {
		q = q.Where("kind = ?", kind)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var ids []uint64
	if err := q.Select("id").Order("id DESC").Offset((page-1)*pageSize).Limit(pageSize).Pluck("id", &ids).Error; err != nil {
		return nil, 0, err
	}
	if len(ids) == 0 {
		return []*model.GenerationTask{}, total, nil
	}
	var items []*model.GenerationTask
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	order := make(map[uint64]int, len(ids))
	for i, id := range ids {
		order[id] = i
	}
	sort.SliceStable(items, func(i, j int) bool { return order[items[i].ID] < order[items[j].ID] })
	return items, total, nil
}

// ListResultsByTask 查询结果列表。
func (r *GenerationRepo) ListResultsByTask(ctx context.Context, taskID string) ([]*model.GenerationResult, error) {
	var items []*model.GenerationResult
	err := r.db.WithContext(ctx).
		Where("task_id = ? AND deleted_at IS NULL", taskID).Order("seq ASC, id ASC").Find(&items).Error
	return items, err
}

// GetResultByTaskSeq returns one result row by task and sequence.
func (r *GenerationRepo) GetResultByTaskSeq(ctx context.Context, taskID string, seq int) (*model.GenerationResult, error) {
	var item model.GenerationResult
	err := r.db.WithContext(ctx).
		Where("task_id = ? AND seq = ? AND deleted_at IS NULL", taskID, seq).
		First(&item).Error
	if err != nil {
		return nil, mapErr(err)
	}
	return &item, nil
}

// SoftDeleteByUser marks a user's generation tasks and results as deleted.
func (r *GenerationRepo) SoftDeleteByUser(ctx context.Context, userID uint64, failedOnly bool) (int64, error) {
	now := time.Now().UTC()
	var deleted int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		q := tx.Model(&model.GenerationTask{}).
			Where("user_id = ? AND deleted_at IS NULL", userID)
		if failedOnly {
			q = q.Where("status = ?", model.GenStatusFailed)
		}
		var taskIDs []string
		if err := q.Pluck("task_id", &taskIDs).Error; err != nil {
			return err
		}
		if len(taskIDs) == 0 {
			return nil
		}
		taskRes := tx.Model(&model.GenerationTask{}).
			Where("user_id = ? AND deleted_at IS NULL AND task_id IN ?", userID, taskIDs).
			Update("deleted_at", now)
		if taskRes.Error != nil {
			return taskRes.Error
		}
		deleted = taskRes.RowsAffected
		return tx.Table("generation_result").
			Where("deleted_at IS NULL AND task_id IN ?", taskIDs).
			Update("deleted_at", now).Error
	})
	return deleted, err
}

// SoftDeleteByUserBefore marks a user's generation tasks before the cutoff as deleted.
func (r *GenerationRepo) SoftDeleteByUserBefore(ctx context.Context, userID uint64, before time.Time) (int64, error) {
	now := time.Now().UTC()
	var deleted int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var taskIDs []string
		if err := tx.Model(&model.GenerationTask{}).
			Where("user_id = ? AND deleted_at IS NULL AND created_at < ?", userID, before).
			Pluck("task_id", &taskIDs).Error; err != nil {
			return err
		}
		if len(taskIDs) == 0 {
			return nil
		}
		taskRes := tx.Model(&model.GenerationTask{}).
			Where("user_id = ? AND deleted_at IS NULL AND task_id IN ?", userID, taskIDs).
			Update("deleted_at", now)
		if taskRes.Error != nil {
			return taskRes.Error
		}
		deleted = taskRes.RowsAffected
		return tx.Table("generation_result").
			Where("deleted_at IS NULL AND task_id IN ?", taskIDs).
			Update("deleted_at", now).Error
	})
	return deleted, err
}

func truncateStr(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
