package usage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

// QueryDAO 提供 usage_logs 的只读查询/聚合能力。
// 与 logger 的写入路径解耦,单独维护。
type QueryDAO struct{ db *sqlx.DB }

func NewQueryDAO(db *sqlx.DB) *QueryDAO { return &QueryDAO{db: db} }

// Filter 是列表/聚合的公共过滤条件。
// 所有字段都可选;非零才参与 WHERE。
type Filter struct {
	UserID    uint64
	KeyID     uint64
	ModelID   uint64
	AccountID uint64
	Type      string // "chat" | "image"
	Status    string
	Since     time.Time
	Until     time.Time
}

// ItemRow 列表返回行(含部分展开字段,省一次 JOIN)。
type ItemRow struct {
	ID               uint64    `db:"id" json:"id"`
	UserID           uint64    `db:"user_id" json:"user_id"`
	KeyID            uint64    `db:"key_id" json:"key_id"`
	ModelID          uint64    `db:"model_id" json:"model_id"`
	ModelSlug        string    `db:"model_slug" json:"model_slug"`
	AccountID        uint64    `db:"account_id" json:"account_id"`
	RequestID        string    `db:"request_id" json:"request_id"`
	Type             string    `db:"type" json:"type"`
	InputTokens      int       `db:"input_tokens" json:"input_tokens"`
	OutputTokens     int       `db:"output_tokens" json:"output_tokens"`
	CacheReadTokens  int       `db:"cache_read_tokens" json:"cache_read_tokens"`
	CacheWriteTokens int       `db:"cache_write_tokens" json:"cache_write_tokens"`
	ImageCount       int       `db:"image_count" json:"image_count"`
	CreditCost       int64     `db:"credit_cost" json:"credit_cost"`
	DurationMs       int       `db:"duration_ms" json:"duration_ms"`
	Status           string    `db:"status" json:"status"`
	ErrorCode        string    `db:"error_code" json:"error_code"`
	IP               string    `db:"ip" json:"ip"`
	CreatedAt        time.Time `db:"created_at" json:"created_at"`
}

// ModelStat 按模型聚合。
type ModelStat struct {
	ModelID      uint64 `db:"model_id" json:"model_id"`
	ModelSlug    string `db:"model_slug" json:"model_slug"`
	Type         string `db:"type" json:"type"`
	Requests     int64  `db:"requests" json:"requests"`
	Failures     int64  `db:"failures" json:"failures"`
	InputTokens  int64  `db:"input_tokens" json:"input_tokens"`
	OutputTokens int64  `db:"output_tokens" json:"output_tokens"`
	ImageCount   int64  `db:"image_count" json:"image_count"`
	CreditCost   int64  `db:"credit_cost" json:"credit_cost"`
	AvgDurMs     int64  `db:"avg_dur_ms" json:"avg_dur_ms"`
}

// UserStat 按用户聚合。
type UserStat struct {
	UserID     uint64 `db:"user_id" json:"user_id"`
	Email      string `db:"email" json:"email"`
	Requests   int64  `db:"requests" json:"requests"`
	Failures   int64  `db:"failures" json:"failures"`
	CreditCost int64  `db:"credit_cost" json:"credit_cost"`
}

// DailyPoint 按天聚合,用于图表。
type DailyPoint struct {
	Day          string `db:"day" json:"day"`
	Requests     int64  `db:"requests" json:"requests"`
	Failures     int64  `db:"failures" json:"failures"`
	InputTokens  int64  `db:"input_tokens" json:"input_tokens"`
	OutputTokens int64  `db:"output_tokens" json:"output_tokens"`
	ImageCount   int64  `db:"image_count" json:"image_count"`
	CreditCost   int64  `db:"credit_cost" json:"credit_cost"`
}

// Overall 整体汇总。
type Overall struct {
	Requests     int64 `db:"requests" json:"requests"`
	Failures     int64 `db:"failures" json:"failures"`
	ChatRequests int64 `db:"chat_requests" json:"chat_requests"`
	ImageImages  int64 `db:"image_images" json:"image_images"`
	InputTokens  int64 `db:"input_tokens" json:"input_tokens"`
	OutputTokens int64 `db:"output_tokens" json:"output_tokens"`
	CreditCost   int64 `db:"credit_cost" json:"credit_cost"`
}

const imageCountExpr = `CASE
  WHEN u.type <> 'image' THEN 0
  WHEN u.image_count > 0 THEN u.image_count
  WHEN u.status = 'success' THEN 1
  ELSE 0
END`

// ---------- 内部 ----------

// buildWhere 根据 filter 生成 WHERE 片段 + 参数列表。
// 永远至少返回 "WHERE 1=1" 以便调用方直接 `+` 其它条件。
func (d *QueryDAO) buildWhere(f Filter) (string, []any) {
	b := strings.Builder{}
	b.WriteString("WHERE 1=1")
	args := make([]any, 0, 6)
	if f.UserID > 0 {
		b.WriteString(" AND u.user_id = ?")
		args = append(args, f.UserID)
	}
	if f.KeyID > 0 {
		b.WriteString(" AND u.key_id = ?")
		args = append(args, f.KeyID)
	}
	if f.ModelID > 0 {
		b.WriteString(" AND u.model_id = ?")
		args = append(args, f.ModelID)
	}
	if f.AccountID > 0 {
		b.WriteString(" AND u.account_id = ?")
		args = append(args, f.AccountID)
	}
	if f.Type != "" {
		b.WriteString(" AND u.type = ?")
		args = append(args, f.Type)
	}
	if f.Status != "" {
		b.WriteString(" AND u.status = ?")
		args = append(args, f.Status)
	}
	if !f.Since.IsZero() {
		b.WriteString(" AND u.created_at >= ?")
		args = append(args, f.Since)
	}
	if !f.Until.IsZero() {
		b.WriteString(" AND u.created_at < ?")
		args = append(args, f.Until)
	}
	return b.String(), args
}

// List 分页查询原始日志。
func (d *QueryDAO) List(ctx context.Context, f Filter, offset, limit int) ([]ItemRow, int64, error) {
	where, args := d.buildWhere(f)
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	q := fmt.Sprintf(`
SELECT u.id, u.user_id, u.key_id, u.model_id,
       COALESCE(m.slug, '') AS model_slug,
       u.account_id, u.request_id, u.type,
       u.input_tokens, u.output_tokens, u.cache_read_tokens, u.cache_write_tokens,
       u.image_count, u.credit_cost, u.duration_ms, u.status, u.error_code, u.ip,
       u.created_at
FROM usage_logs u
LEFT JOIN models m ON m.id = u.model_id
%s
ORDER BY u.id DESC
LIMIT ? OFFSET ?`, where)

	rows := make([]ItemRow, 0, limit)
	err := d.db.SelectContext(ctx, &rows, q, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}

	countQ := fmt.Sprintf(`SELECT COUNT(*) FROM usage_logs u %s`, where)
	var total int64
	if err := d.db.GetContext(ctx, &total, countQ, args...); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// Overall 汇总。
func (d *QueryDAO) Overall(ctx context.Context, f Filter) (Overall, error) {
	where, args := d.buildWhere(f)
	q := fmt.Sprintf(`
SELECT COUNT(*)                                                                AS requests,
       COALESCE(SUM(CASE WHEN u.status = 'failed' THEN 1 ELSE 0 END), 0)       AS failures,
       COALESCE(SUM(CASE WHEN u.type   = 'chat'   THEN 1 ELSE 0 END), 0)       AS chat_requests,
       COALESCE(SUM(`+imageCountExpr+`), 0)                                    AS image_images,
       COALESCE(SUM(u.input_tokens),  0)                                       AS input_tokens,
       COALESCE(SUM(u.output_tokens), 0)                                       AS output_tokens,
       COALESCE(SUM(u.credit_cost),   0)                                       AS credit_cost
FROM usage_logs u %s`, where)

	var out Overall
	if err := d.db.GetContext(ctx, &out, q, args...); err != nil {
		return out, err
	}
	return out, nil
}

// ByModel 按模型聚合。
func (d *QueryDAO) ByModel(ctx context.Context, f Filter, limit int) ([]ModelStat, error) {
	where, args := d.buildWhere(f)
	if limit <= 0 {
		limit = 50
	}
	q := fmt.Sprintf(`
SELECT u.model_id,
       COALESCE(m.slug, '')                                AS model_slug,
       COALESCE(MAX(u.type), '')                           AS type,
       COUNT(*)                                            AS requests,
       COALESCE(SUM(CASE WHEN u.status='failed' THEN 1 ELSE 0 END), 0) AS failures,
       COALESCE(SUM(u.input_tokens),  0)                   AS input_tokens,
       COALESCE(SUM(u.output_tokens), 0)                   AS output_tokens,
       COALESCE(SUM(`+imageCountExpr+`), 0)                AS image_count,
       COALESCE(SUM(u.credit_cost),   0)                   AS credit_cost,
       /* AVG 返回 DECIMAL(driver 会给 []uint8),必须 CAST 回整数才能 scan 进 int64 */
       COALESCE(CAST(AVG(u.duration_ms) AS SIGNED), 0)     AS avg_dur_ms
FROM usage_logs u
LEFT JOIN models m ON m.id = u.model_id
%s
GROUP BY u.model_id, m.slug
ORDER BY requests DESC
LIMIT ?`, where)

	rows := make([]ModelStat, 0, limit)
	err := d.db.SelectContext(ctx, &rows, q, append(args, limit)...)
	return rows, err
}

// ByUser 按用户聚合。只给管理员用,会 JOIN users。
func (d *QueryDAO) ByUser(ctx context.Context, f Filter, limit int) ([]UserStat, error) {
	where, args := d.buildWhere(f)
	if limit <= 0 {
		limit = 50
	}
	q := fmt.Sprintf(`
SELECT u.user_id,
       COALESCE(us.email, '')                              AS email,
       COUNT(*)                                            AS requests,
       COALESCE(SUM(CASE WHEN u.status='failed' THEN 1 ELSE 0 END), 0) AS failures,
       COALESCE(SUM(u.credit_cost),   0)                   AS credit_cost
FROM usage_logs u
LEFT JOIN users us ON us.id = u.user_id
%s
GROUP BY u.user_id, us.email
ORDER BY credit_cost DESC
LIMIT ?`, where)

	rows := make([]UserStat, 0, limit)
	err := d.db.SelectContext(ctx, &rows, q, append(args, limit)...)
	return rows, err
}

// Daily 按天聚合最近 N 天。
func (d *QueryDAO) Daily(ctx context.Context, f Filter, days int) ([]DailyPoint, error) {
	if days <= 0 || days > 180 {
		days = 14
	}
	// 强制把 since 限制到 days 窗口内
	since := time.Now().AddDate(0, 0, -days+1).Truncate(24 * time.Hour)
	if f.Since.IsZero() || f.Since.Before(since) {
		f.Since = since
	}
	where, args := d.buildWhere(f)

	q := fmt.Sprintf(`
SELECT DATE_FORMAT(u.created_at, '%%Y-%%m-%%d')            AS day,
       COUNT(*)                                            AS requests,
       COALESCE(SUM(CASE WHEN u.status='failed' THEN 1 ELSE 0 END), 0) AS failures,
       COALESCE(SUM(u.input_tokens),  0)                   AS input_tokens,
       COALESCE(SUM(u.output_tokens), 0)                   AS output_tokens,
       COALESCE(SUM(`+imageCountExpr+`), 0)                AS image_count,
       COALESCE(SUM(u.credit_cost),   0)                   AS credit_cost
FROM usage_logs u
%s
GROUP BY day
ORDER BY day ASC`, where)

	rows := make([]DailyPoint, 0, days)
	err := d.db.SelectContext(ctx, &rows, q, args...)
	return rows, err
}
