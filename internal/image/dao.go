package image

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

// ErrNotFound 未找到任务。
var ErrNotFound = errors.New("image: task not found")

// DAO image_tasks 表访问对象。
type DAO struct{ db *sqlx.DB }

// NewDAO 构造。
func NewDAO(db *sqlx.DB) *DAO { return &DAO{db: db} }

// Create 插入新任务。
func (d *DAO) Create(ctx context.Context, t *Task) error {
	res, err := d.db.ExecContext(ctx, `
INSERT INTO image_tasks
  (task_id, user_id, key_id, model_id, account_id, prompt, n, size, upscale, status,
   conversation_id, file_ids, result_urls, error, estimated_credit, credit_cost,
   created_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?, NOW())`,
		t.TaskID, t.UserID, t.KeyID, t.ModelID, t.AccountID,
		t.Prompt, t.N, t.Size, ValidateUpscale(t.Upscale),
		nullEmpty(t.Status, StatusQueued),
		t.ConversationID, nullJSON(t.FileIDs), nullJSON(t.ResultURLs),
		t.Error, t.EstimatedCredit, t.CreditCost,
	)
	if err != nil {
		return fmt.Errorf("image dao create: %w", err)
	}
	id, _ := res.LastInsertId()
	t.ID = uint64(id)
	return nil
}

// MarkRunning 标记为运行中(记录起始时间 + account_id)。
func (d *DAO) MarkRunning(ctx context.Context, taskID string, accountID uint64) error {
	_, err := d.db.ExecContext(ctx, `
UPDATE image_tasks
   SET status='running', account_id=?, started_at=NOW()
 WHERE task_id=? AND status IN ('queued','dispatched')`, accountID, taskID)
	return err
}

// SetAccount 在 runOnce 拿到账号 lease 后立刻写入 account_id。
// 独立出来是因为 MarkRunning 只在 status=queued/dispatched 时生效,
// 而调度完成后 status 已经是 running,需要一个幂等的小方法。
// 图片代理端点按 task_id 查账号时依赖这个字段。
func (d *DAO) SetAccount(ctx context.Context, taskID string, accountID uint64) error {
	_, err := d.db.ExecContext(ctx,
		`UPDATE image_tasks SET account_id = ? WHERE task_id = ?`, accountID, taskID)
	return err
}

// MarkSuccess 更新成功状态。
func (d *DAO) MarkSuccess(ctx context.Context, taskID, convID string, fileIDs, resultURLs []string, creditCost int64) error {
	fidB, _ := json.Marshal(fileIDs)
	urlB, _ := json.Marshal(resultURLs)
	_, err := d.db.ExecContext(ctx, `
UPDATE image_tasks
   SET status='success',
       conversation_id=?,
       file_ids=?,
       result_urls=?,
       credit_cost=?,
       finished_at=NOW()
 WHERE task_id=?`, convID, fidB, urlB, creditCost, taskID)
	return err
}

// UpdateCost 仅更新 credit_cost(Runner 成功后由网关层调用)。
func (d *DAO) UpdateCost(ctx context.Context, taskID string, cost int64) error {
	_, err := d.db.ExecContext(ctx,
		`UPDATE image_tasks SET credit_cost = ? WHERE task_id = ?`, cost, taskID)
	return err
}

// MarkFailed 更新失败状态(带错误码)。
func (d *DAO) MarkFailed(ctx context.Context, taskID, errorCode string) error {
	_, err := d.db.ExecContext(ctx, `
UPDATE image_tasks
   SET status='failed', error=?, finished_at=NOW()
 WHERE task_id=?`, truncate(errorCode, 500), taskID)
	return err
}

// Get 根据对外 task_id 查询。
func (d *DAO) Get(ctx context.Context, taskID string) (*Task, error) {
	var t Task
	err := d.db.GetContext(ctx, &t, `
SELECT id, task_id, user_id, key_id, model_id, account_id, prompt, n, size, upscale, status,
       conversation_id, file_ids, result_urls, error, estimated_credit, credit_cost,
       created_at, started_at, finished_at
  FROM image_tasks
 WHERE task_id = ?`, taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// MyTaskFilter 当前用户图片任务筛选条件。
type MyTaskFilter struct {
	Status    string
	Keyword   string
	CreatedAt *time.Time
	CreatedTo *time.Time
}

// ListByUser 按用户分页。
func (d *DAO) ListByUser(ctx context.Context, userID uint64, f MyTaskFilter, limit, offset int) ([]Task, error) {
	if limit <= 0 {
		limit = 20
	}
	where := []string{"user_id = ?"}
	args := []interface{}{userID}
	if f.Status != "" {
		where = append(where, "status = ?")
		args = append(args, f.Status)
	}
	if f.Keyword != "" {
		where = append(where, "prompt LIKE ?")
		args = append(args, "%"+f.Keyword+"%")
	}
	if f.CreatedAt != nil {
		where = append(where, "created_at >= ?")
		args = append(args, *f.CreatedAt)
	}
	if f.CreatedTo != nil {
		where = append(where, "created_at <= ?")
		args = append(args, *f.CreatedTo)
	}
	var out []Task
	err := d.db.SelectContext(ctx, &out, `
SELECT id, task_id, user_id, key_id, model_id, account_id, prompt, n, size, upscale, status,
       conversation_id, file_ids, result_urls, error, estimated_credit, credit_cost,
       created_at, started_at, finished_at
  FROM image_tasks
 WHERE `+strings.Join(where, " AND ")+`
 ORDER BY id DESC
 LIMIT ? OFFSET ?`, append(args, limit, offset)...)
	return out, err
}

// AdminTaskRow 是管理员视角的生成记录行,JOIN 了 users 表的邮箱。
type AdminTaskRow struct {
	Task
	UserEmail string `db:"user_email" json:"user_email"`
}

// AdminTaskFilter 管理员查询过滤条件。
type AdminTaskFilter struct {
	UserID  uint64
	Keyword string // 模糊匹配 prompt / email
	Status  string
}

// ListAdmin 全局分页(admin)。
func (d *DAO) ListAdmin(ctx context.Context, f AdminTaskFilter, limit, offset int) ([]AdminTaskRow, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	where := "1=1"
	args := []interface{}{}
	if f.UserID > 0 {
		where += " AND t.user_id = ?"
		args = append(args, f.UserID)
	}
	if f.Status != "" {
		where += " AND t.status = ?"
		args = append(args, f.Status)
	}
	if f.Keyword != "" {
		like := "%" + f.Keyword + "%"
		where += " AND (t.prompt LIKE ? OR u.email LIKE ?)"
		args = append(args, like, like)
	}

	var total int64
	countSQL := `SELECT COUNT(*) FROM image_tasks t LEFT JOIN users u ON u.id=t.user_id WHERE ` + where
	if err := d.db.GetContext(ctx, &total, countSQL, args...); err != nil {
		return nil, 0, err
	}

	listSQL := `
SELECT t.id, t.task_id, t.user_id, t.key_id, t.model_id, t.account_id,
       t.prompt, t.n, t.size, t.upscale, t.status,
       t.conversation_id, t.file_ids, t.result_urls, t.error,
       t.estimated_credit, t.credit_cost,
       t.created_at, t.started_at, t.finished_at,
       COALESCE(u.email, '') AS user_email
  FROM image_tasks t
  LEFT JOIN users u ON u.id = t.user_id
 WHERE ` + where + `
 ORDER BY t.id DESC
 LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	var out []AdminTaskRow
	err := d.db.SelectContext(ctx, &out, listSQL, args...)
	return out, total, err
}

// DecodeFileIDs 把 JSON 列解出字符串数组。
func (t *Task) DecodeFileIDs() []string {
	var out []string
	if len(t.FileIDs) > 0 {
		_ = json.Unmarshal(t.FileIDs, &out)
	}
	return out
}

// DecodeResultURLs 把 JSON 列解出字符串数组。
func (t *Task) DecodeResultURLs() []string {
	var out []string
	if len(t.ResultURLs) > 0 {
		_ = json.Unmarshal(t.ResultURLs, &out)
	}
	return out
}

// ---- helpers ----

func nullEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func nullJSON(b []byte) interface{} {
	if len(b) == 0 {
		return nil
	}
	return b
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

var _ = time.Now // keep import
