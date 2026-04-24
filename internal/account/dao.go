package account

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

var ErrNotFound = errors.New("账号不存在")

type DAO struct{ db *sqlx.DB }

func NewDAO(db *sqlx.DB) *DAO { return &DAO{db: db} }

// DB 暴露底层 handle 给刷新器 / 探测器用于直接原子更新(少量场景)。
func (d *DAO) DB() *sqlx.DB { return d.db }

// fill 填充非 db 列的辅助字段。
func fill(a *Account) {
	if a == nil {
		return
	}
	a.HasRT = a.RefreshTokenEnc.Valid && a.RefreshTokenEnc.String != ""
	a.HasST = a.SessionTokenEnc.Valid && a.SessionTokenEnc.String != ""
}

func fillAll(rows []*Account) {
	for _, r := range rows {
		fill(r)
	}
}

func (d *DAO) Create(ctx context.Context, a *Account) (uint64, error) {
	res, err := d.db.ExecContext(ctx,
		`INSERT INTO oai_accounts
         (email, auth_token_enc, refresh_token_enc, session_token_enc, token_expires_at,
          oai_session_id, oai_device_id, client_id, chatgpt_account_id, account_type,
          plan_type, daily_image_quota, status, notes)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.Email, a.AuthTokenEnc, a.RefreshTokenEnc, a.SessionTokenEnc, a.TokenExpiresAt,
		a.OAISessionID, a.OAIDeviceID, a.ClientID, a.ChatGPTAccountID, a.AccountType,
		a.PlanType, a.DailyImageQuota, a.Status, a.Notes,
	)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return uint64(id), nil
}

func (d *DAO) GetByID(ctx context.Context, id uint64) (*Account, error) {
	var a Account
	err := d.db.GetContext(ctx, &a,
		`SELECT * FROM oai_accounts WHERE id = ? AND deleted_at IS NULL`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	fill(&a)
	return &a, err
}

// GetByEmail 精确找;未命中返回 nil, nil(方便 importer 判 upsert)。
func (d *DAO) GetByEmail(ctx context.Context, email string) (*Account, error) {
	var a Account
	err := d.db.GetContext(ctx, &a,
		`SELECT * FROM oai_accounts WHERE email = ? AND deleted_at IS NULL LIMIT 1`, email)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	fill(&a)
	return &a, nil
}

func (d *DAO) List(ctx context.Context, status string, keyword string, offset, limit int) ([]*Account, int64, error) {
	var total int64
	var err error
	var rows []*Account

	where := "deleted_at IS NULL"
	args := []interface{}{}
	if status != "" {
		where += " AND status = ?"
		args = append(args, status)
	}
	if keyword != "" {
		where += " AND (email LIKE ? OR notes LIKE ?)"
		like := "%" + keyword + "%"
		args = append(args, like, like)
	}

	if err = d.db.GetContext(ctx, &total, "SELECT COUNT(*) FROM oai_accounts WHERE "+where, args...); err != nil {
		return nil, 0, err
	}
	argsPage := append([]interface{}{}, args...)
	argsPage = append(argsPage, limit, offset)
	err = d.db.SelectContext(ctx, &rows,
		"SELECT * FROM oai_accounts WHERE "+where+" ORDER BY id DESC LIMIT ? OFFSET ?", argsPage...)
	fillAll(rows)
	return rows, total, err
}

// ListDispatchable 调度器专用:返回 AT 有效且 cooldown 到期的候选账号。
//
// 接受 status IN ('healthy', 'warned'):
//   - healthy: 正常
//   - warned:  RT/ST 刷新失败但 AT 尚未到期,仍可继续出图直到 AT 过期
func (d *DAO) ListDispatchable(ctx context.Context, limit int) ([]*Account, error) {
	rows := make([]*Account, 0, limit)
	now := time.Now()
	err := d.db.SelectContext(ctx, &rows,
		`SELECT * FROM oai_accounts
         WHERE deleted_at IS NULL AND status IN ('healthy', 'warned')
           AND (cooldown_until IS NULL OR cooldown_until <= ?)
           AND (token_expires_at IS NULL OR token_expires_at > ?)
           AND image_quota_remaining <> 0
         ORDER BY
           CASE status WHEN 'healthy' THEN 0 ELSE 1 END,
           CASE WHEN last_used_at IS NULL THEN 0 ELSE 1 END,
           last_used_at ASC
         LIMIT ?`, now, now, limit)
	fillAll(rows)
	return rows, err
}

// ListNeedRefresh 返回需要预刷新的账号(AT 将在 aheadSec 秒内过期)。
// 按 token_expires_at 升序,最快过期的先刷。
func (d *DAO) ListNeedRefresh(ctx context.Context, aheadSec int, limit int) ([]*Account, error) {
	rows := make([]*Account, 0, limit)
	threshold := time.Now().Add(time.Duration(aheadSec) * time.Second)
	err := d.db.SelectContext(ctx, &rows,
		`SELECT * FROM oai_accounts
         WHERE deleted_at IS NULL
           AND status <> 'dead'
           AND (refresh_token_enc IS NOT NULL OR session_token_enc IS NOT NULL)
           AND token_expires_at IS NOT NULL
           AND token_expires_at <= ?
         ORDER BY token_expires_at ASC
         LIMIT ?`, threshold, limit)
	fillAll(rows)
	return rows, err
}

// ListNeedProbeQuota 返回需要探测图片额度的账号。命中以下任一条件即纳入:
//
//	(a) healthy: 从未探测过 / 常规轮询到期 / 剩余额度=0 且已过 reset_at;
//	(b) throttled: 从未探测过 / 常规轮询到期 / cooldown_until 到期 /
//	    image_quota_reset_at 到期。
//
// 这样"因图片额度耗尽而被降为 throttled"的账号可以在重置时间到达后
// 被及时补探,并在额度恢复时自动回到 healthy。
func (d *DAO) ListNeedProbeQuota(ctx context.Context, minIntervalSec int, limit int) ([]*Account, error) {
	rows := make([]*Account, 0, limit)
	threshold := time.Now().Add(-time.Duration(minIntervalSec) * time.Second)
	err := d.db.SelectContext(ctx, &rows,
		`SELECT * FROM oai_accounts
         WHERE deleted_at IS NULL
           AND status IN ('healthy', 'throttled')
           AND (token_expires_at IS NULL OR token_expires_at > NOW())
           AND (
                (
                    status = 'healthy'
                AND (
                        image_quota_updated_at IS NULL
                     OR image_quota_updated_at <= ?
                     OR (image_quota_remaining = 0
                         AND (image_quota_reset_at IS NULL OR image_quota_reset_at <= NOW()))
                    )
                )
             OR (
                    status = 'throttled'
                AND (
                        image_quota_updated_at IS NULL
                     OR image_quota_updated_at <= ?
                     OR (cooldown_until IS NOT NULL AND cooldown_until <= NOW())
                     OR (image_quota_reset_at IS NOT NULL AND image_quota_reset_at <= NOW())
                    )
                )
           )
         ORDER BY CASE status WHEN 'throttled' THEN 0 ELSE 1 END,
                  CASE WHEN image_quota_updated_at IS NULL THEN 0 ELSE 1 END,
                  image_quota_updated_at ASC
         LIMIT ?`, threshold, threshold, limit)
	fillAll(rows)
	return rows, err
}

// ListAllActiveIDs 用于批量刷新 / 批量探测:返回未软删的所有 id。
func (d *DAO) ListAllActiveIDs(ctx context.Context) ([]uint64, error) {
	ids := make([]uint64, 0, 128)
	err := d.db.SelectContext(ctx, &ids,
		`SELECT id FROM oai_accounts WHERE deleted_at IS NULL ORDER BY id ASC`)
	return ids, err
}

// QuotaSummary 全局额度汇总。
type QuotaSummary struct {
	TotalRemaining int64 `db:"total_remaining" json:"total_remaining"` // 所有未软删账号剩余额度之和
	TotalCapacity  int64 `db:"total_capacity"  json:"total_capacity"`  // 所有未软删账号上限之和
	ActiveAccounts int64 `db:"active_accounts" json:"active_accounts"` // 未软删账号总数
}

// SumQuota 汇总所有未软删账号的额度(含 dead/suspicious)。
// 账号失效只影响能否被调度出图,不影响其已探测到的额度数字;
// 全部纳入统计才能正确反映账号池的实际剩余容量。
func (d *DAO) SumQuota(ctx context.Context) (*QuotaSummary, error) {
	var s QuotaSummary
	err := d.db.GetContext(ctx, &s, `
SELECT
  COALESCE(SUM(image_quota_remaining), 0) AS total_remaining,
  COALESCE(SUM(image_quota_total),     0) AS total_capacity,
  COUNT(*)                                AS active_accounts
FROM oai_accounts
WHERE deleted_at IS NULL`)
	return &s, err
}

func (d *DAO) Update(ctx context.Context, a *Account) error {
	_, err := d.db.ExecContext(ctx,
		`UPDATE oai_accounts
         SET email=?, auth_token_enc=?, refresh_token_enc=?, session_token_enc=?, token_expires_at=?,
             oai_session_id=?, oai_device_id=?, client_id=?, chatgpt_account_id=?, account_type=?,
             plan_type=?, daily_image_quota=?,
             status=?, notes=?
         WHERE id = ? AND deleted_at IS NULL`,
		a.Email, a.AuthTokenEnc, a.RefreshTokenEnc, a.SessionTokenEnc, a.TokenExpiresAt,
		a.OAISessionID, a.OAIDeviceID, a.ClientID, a.ChatGPTAccountID, a.AccountType,
		a.PlanType, a.DailyImageQuota,
		a.Status, a.Notes, a.ID,
	)
	return err
}

func (d *DAO) SoftDelete(ctx context.Context, id uint64) error {
	_, err := d.db.ExecContext(ctx,
		`UPDATE oai_accounts SET deleted_at = ? WHERE id = ?`, time.Now(), id)
	return err
}

// SoftDeleteByStatus 按状态批量软删。status 为空时删除全部(调用方需二次确认)。
// 返回删除行数。
func (d *DAO) SoftDeleteByStatus(ctx context.Context, status string) (int64, error) {
	now := time.Now()
	if status == "" {
		res, err := d.db.ExecContext(ctx,
			`UPDATE oai_accounts SET deleted_at = ? WHERE deleted_at IS NULL`, now)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		return n, nil
	}
	res, err := d.db.ExecContext(ctx,
		`UPDATE oai_accounts SET deleted_at = ? WHERE deleted_at IS NULL AND status = ?`,
		now, status)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// EnsureDeviceID 确保账号有 oai_device_id。
// 如果当前为空,原子写入给定的 deviceID;返回最终实际的 device_id(已有则原值)。
func (d *DAO) EnsureDeviceID(ctx context.Context, id uint64, deviceID string) (string, error) {
	_, err := d.db.ExecContext(ctx,
		`UPDATE oai_accounts SET oai_device_id = ?
         WHERE id = ? AND deleted_at IS NULL AND (oai_device_id = '' OR oai_device_id IS NULL)`,
		deviceID, id)
	if err != nil {
		return "", err
	}
	// 回读,兼容其他协程并发填写的情形
	var cur string
	if err := d.db.GetContext(ctx, &cur,
		`SELECT oai_device_id FROM oai_accounts WHERE id = ?`, id); err != nil {
		return "", err
	}
	return cur, nil
}

// EnsureSessionID 确保账号有 oai_session_id(按账号稳定复用)。
// 逻辑与 EnsureDeviceID 完全一致,单独一个函数是为了日志/审计区分用途。
func (d *DAO) EnsureSessionID(ctx context.Context, id uint64, sessionID string) (string, error) {
	_, err := d.db.ExecContext(ctx,
		`UPDATE oai_accounts SET oai_session_id = ?
         WHERE id = ? AND deleted_at IS NULL AND (oai_session_id = '' OR oai_session_id IS NULL)`,
		sessionID, id)
	if err != nil {
		return "", err
	}
	var cur string
	if err := d.db.GetContext(ctx, &cur,
		`SELECT oai_session_id FROM oai_accounts WHERE id = ?`, id); err != nil {
		return "", err
	}
	return cur, nil
}

// MarkUsed 更新 last_used_at + 今日计数。today 是当日零点(用于 today_used_date 比较)。
func (d *DAO) MarkUsed(ctx context.Context, id uint64, today time.Time) error {
	_, err := d.db.ExecContext(ctx,
		`UPDATE oai_accounts
         SET last_used_at = ?,
             today_used_count = CASE WHEN today_used_date = ? THEN today_used_count + 1 ELSE 1 END,
             today_used_date  = ?
         WHERE id = ?`,
		time.Now(), today, today, id)
	return err
}

// SetStatus 迁移状态,可选 cooldownUntil。
func (d *DAO) SetStatus(ctx context.Context, id uint64, status string, cooldownUntil *time.Time) error {
	if cooldownUntil != nil {
		_, err := d.db.ExecContext(ctx,
			`UPDATE oai_accounts SET status=?, cooldown_until=? WHERE id=?`,
			status, *cooldownUntil, id)
		return err
	}
	_, err := d.db.ExecContext(ctx,
		`UPDATE oai_accounts SET status=?, cooldown_until=NULL WHERE id=?`,
		status, id)
	return err
}

// ApplyRefreshResult 原子更新 AT / RT + 过期时间 + 最近刷新信息。
// newRTEnc 为空字符串表示 RT 没有轮转,保持不变。
func (d *DAO) ApplyRefreshResult(
	ctx context.Context,
	id uint64,
	newATEnc string,
	newRTEnc string,
	expiresAt time.Time,
	source string,
) error {
	var err error
	if newRTEnc != "" {
		_, err = d.db.ExecContext(ctx,
			`UPDATE oai_accounts
             SET auth_token_enc = ?,
                 refresh_token_enc = ?,
                 token_expires_at = ?,
                 last_refresh_at = ?,
                 last_refresh_source = ?,
                 refresh_error = '',
                 status = CASE WHEN status IN ('dead','suspicious') THEN 'healthy' ELSE status END
             WHERE id = ? AND deleted_at IS NULL`,
			newATEnc, newRTEnc, expiresAt, time.Now(), source, id)
	} else {
		_, err = d.db.ExecContext(ctx,
			`UPDATE oai_accounts
             SET auth_token_enc = ?,
                 token_expires_at = ?,
                 last_refresh_at = ?,
                 last_refresh_source = ?,
                 refresh_error = '',
                 status = CASE WHEN status IN ('dead','suspicious') THEN 'healthy' ELSE status END
             WHERE id = ? AND deleted_at IS NULL`,
			newATEnc, expiresAt, time.Now(), source, id)
	}
	return err
}

// RecordRefreshError 写入刷新失败原因,同时推进 last_refresh_at(避免 pressed-out 重试)。
//
// markDead=true 时:若当前 AT 尚未过期,只降为 'warned'(账号仍可被调度出图,AT 用完前
// 不要让它离线);若 AT 已过期或为空则真正标 'dead'。
// 这样避免"RT 失效但 AT 还有几天"的账号被提前下线。
func (d *DAO) RecordRefreshError(ctx context.Context, id uint64, source string, reason string, markDead bool) error {
	if markDead {
		_, err := d.db.ExecContext(ctx,
			`UPDATE oai_accounts
             SET last_refresh_at = ?, last_refresh_source = ?, refresh_error = ?,
                 status = CASE
                   WHEN token_expires_at IS NOT NULL AND token_expires_at > NOW() THEN 'warned'
                   ELSE 'dead'
                 END
             WHERE id = ? AND deleted_at IS NULL`,
			time.Now(), source, reason, id)
		return err
	}
	_, err := d.db.ExecContext(ctx,
		`UPDATE oai_accounts
         SET last_refresh_at = ?, last_refresh_source = ?, refresh_error = ?
         WHERE id = ? AND deleted_at IS NULL`,
		time.Now(), source, reason, id)
	return err
}

// ApplyQuotaResult 更新图片额度探测结果;remaining/total = -1 表示保持原值。
func (d *DAO) ApplyQuotaResult(ctx context.Context, id uint64, remaining, total int, resetAt *time.Time) error {
	q := `UPDATE oai_accounts
          SET image_quota_remaining = CASE WHEN ? < 0 THEN image_quota_remaining ELSE ? END,
              image_quota_total     = CASE WHEN ? < 0 THEN image_quota_total     ELSE ? END,
              image_quota_reset_at  = ?,
              image_quota_updated_at = ?
          WHERE id = ? AND deleted_at IS NULL`
	var reset interface{}
	if resetAt != nil {
		reset = *resetAt
	} else {
		reset = nil
	}
	_, err := d.db.ExecContext(ctx, q, remaining, remaining, total, total, reset, time.Now(), id)
	return err
}

// ReserveImageQuota 为一次图片生成预留本地额度。
// image_quota_remaining < 0 表示未知,此时允许通过且保持原值不变。
func (d *DAO) ReserveImageQuota(ctx context.Context, accountID uint64, n int) (bool, error) {
	if n <= 0 {
		return true, nil
	}
	res, err := d.db.ExecContext(ctx,
		`UPDATE oai_accounts
         SET image_quota_remaining = CASE
               WHEN image_quota_remaining < 0 THEN image_quota_remaining
               ELSE image_quota_remaining - ?
             END
         WHERE id = ? AND deleted_at IS NULL
           AND (image_quota_remaining < 0 OR image_quota_remaining >= ?)`,
		n, accountID, n)
	if err != nil {
		return false, err
	}
	affected, _ := res.RowsAffected()
	return affected > 0, nil
}

// ReleaseImageQuota 归还一次失败任务预留的本地额度。
// 未知额度(image_quota_remaining < 0)保持不变;已知额度会回补,并上限裁到 image_quota_total。
func (d *DAO) ReleaseImageQuota(ctx context.Context, accountID uint64, n int) error {
	if n <= 0 {
		return nil
	}
	_, err := d.db.ExecContext(ctx,
		`UPDATE oai_accounts
         SET image_quota_remaining = CASE
               WHEN image_quota_remaining < 0 THEN image_quota_remaining
               WHEN image_quota_total >= 0 THEN LEAST(image_quota_total, image_quota_remaining + ?)
               ELSE image_quota_remaining + ?
             END
         WHERE id = ? AND deleted_at IS NULL`,
		n, n, accountID)
	return err
}

// DecrQuota 生图成功后立即乐观扣减账号剩余额度。
// 这样无需等下次探测即可在前端看到正确剩余数字。
// n 为本次消耗张数;GREATEST(0,...) 防止出现负值。
func (d *DAO) DecrQuota(ctx context.Context, accountID uint64, n int) error {
	if n <= 0 {
		return nil
	}
	_, err := d.db.ExecContext(ctx,
		`UPDATE oai_accounts
         SET image_quota_remaining = GREATEST(0, image_quota_remaining - ?)
         WHERE id = ? AND deleted_at IS NULL`,
		n, accountID)
	return err
}

// ---- cookies ----

func (d *DAO) UpsertCookies(ctx context.Context, accountID uint64, cookieEnc string) error {
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO oai_account_cookies (account_id, cookie_json_enc)
         VALUES (?, ?)
         ON DUPLICATE KEY UPDATE cookie_json_enc = VALUES(cookie_json_enc)`,
		accountID, cookieEnc)
	return err
}

func (d *DAO) GetCookies(ctx context.Context, accountID uint64) (string, error) {
	var enc string
	err := d.db.GetContext(ctx, &enc,
		`SELECT cookie_json_enc FROM oai_account_cookies WHERE account_id = ?`,
		accountID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return enc, err
}

// ---- bindings ----

func (d *DAO) SetBinding(ctx context.Context, accountID, proxyID uint64) error {
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO account_proxy_bindings (account_id, proxy_id)
         VALUES (?, ?)
         ON DUPLICATE KEY UPDATE proxy_id = VALUES(proxy_id), bound_at = CURRENT_TIMESTAMP`,
		accountID, proxyID)
	return err
}

func (d *DAO) RemoveBinding(ctx context.Context, accountID uint64) error {
	_, err := d.db.ExecContext(ctx,
		`DELETE FROM account_proxy_bindings WHERE account_id = ?`, accountID)
	return err
}

func (d *DAO) GetBinding(ctx context.Context, accountID uint64) (*Binding, error) {
	var b Binding
	err := d.db.GetContext(ctx, &b,
		`SELECT * FROM account_proxy_bindings WHERE account_id = ?`, accountID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &b, err
}
