package repo

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/kleinai/backend/internal/dto"
)

type DashboardRepo struct{ db *gorm.DB }

func NewDashboardRepo(db *gorm.DB) *DashboardRepo { return &DashboardRepo{db: db} }

type dashboardGenerationAgg struct {
	GeneratedToday  int64
	GeneratedTotal  int64
	ImageToday      int64
	ImageTotal      int64
	VideoToday      int64
	VideoTotal      int64
	TextTokensToday int64
	TextTokensTotal int64
	CostToday       int64
	CostTotal       int64
	SuccessToday    int64
	FinishedToday   int64
}

type dashboardWalletAgg struct {
	SpendToday int64
	SpendTotal int64
}

type dashboardUserAgg struct {
	UsersTotal       int64
	UsersToday       int64
	ActiveUsersToday int64
}

type dashboardProviderRow struct {
	Provider       string
	Total          int64
	Enabled        int64
	Available      int64
	Broken         int64
	TestOK         int64
	QuotaRemaining int64
	QuotaTotal     int64
	SuccessCount   int64
	ErrorCount     int64
}

type dashboardRecentRow struct {
	TaskID     string
	CreatedAt  time.Time
	UserLabel  string
	Kind       string
	ModelCode  string
	Count      int
	Status     int8
	CostPoints int64
}

type dashboardTrendRow struct {
	Date           string
	GeneratedCount int64
	CostPoints     int64
}

func (r *DashboardRepo) Overview(ctx context.Context) (*dto.AdminDashboardOverviewResp, error) {
	var gen dashboardGenerationAgg
	genSQL := `SELECT
  COUNT(CASE WHEN DATE(created_at) = CURDATE() THEN 1 END) AS generated_today,
  COUNT(1) AS generated_total,
  COALESCE(SUM(CASE WHEN kind = 'image' AND DATE(created_at) = CURDATE() THEN count ELSE 0 END), 0) AS image_today,
  COALESCE(SUM(CASE WHEN kind = 'image' THEN count ELSE 0 END), 0) AS image_total,
  COALESCE(SUM(CASE WHEN kind = 'video' AND DATE(created_at) = CURDATE() THEN count ELSE 0 END), 0) AS video_today,
  COALESCE(SUM(CASE WHEN kind = 'video' THEN count ELSE 0 END), 0) AS video_total,
  COALESCE(SUM(CASE WHEN DATE(created_at) = CURDATE() THEN CEIL(CHAR_LENGTH(prompt) / 4) ELSE 0 END), 0) AS text_tokens_today,
  COALESCE(SUM(CEIL(CHAR_LENGTH(prompt) / 4)), 0) AS text_tokens_total,
  COALESCE(SUM(CASE WHEN DATE(created_at) = CURDATE() THEN cost_points ELSE 0 END), 0) AS cost_today,
  COALESCE(SUM(cost_points), 0) AS cost_total,
  COUNT(CASE WHEN DATE(created_at) = CURDATE() AND status = 2 THEN 1 END) AS success_today,
  COUNT(CASE WHEN DATE(created_at) = CURDATE() AND status IN (2,3,4) THEN 1 END) AS finished_today
FROM generation_task
WHERE deleted_at IS NULL`
	if err := r.db.WithContext(ctx).Raw(genSQL).Scan(&gen).Error; err != nil {
		return nil, err
	}

	var wallet dashboardWalletAgg
	walletSQL := `SELECT
  COALESCE(SUM(CASE WHEN DATE(created_at) = CURDATE() AND direction < 0 THEN ABS(points) ELSE 0 END), 0) AS spend_today,
  COALESCE(SUM(CASE WHEN direction < 0 THEN ABS(points) ELSE 0 END), 0) AS spend_total
FROM wallet_log`
	if err := r.db.WithContext(ctx).Raw(walletSQL).Scan(&wallet).Error; err != nil {
		return nil, err
	}

	var users dashboardUserAgg
	userSQL := `SELECT
  COUNT(1) AS users_total,
  COUNT(CASE WHEN DATE(created_at) = CURDATE() THEN 1 END) AS users_today,
  COUNT(CASE WHEN DATE(last_login_at) = CURDATE() THEN 1 END) AS active_users_today
FROM ` + "`user`" + `
WHERE deleted_at IS NULL`
	if err := r.db.WithContext(ctx).Raw(userSQL).Scan(&users).Error; err != nil {
		return nil, err
	}

	var providers []*dashboardProviderRow
	providerSQL := `SELECT
  provider,
  COUNT(1) AS total,
  COALESCE(SUM(CASE WHEN status = 1 THEN 1 ELSE 0 END), 0) AS enabled,
  COALESCE(SUM(CASE WHEN status = 1 AND (cooldown_until IS NULL OR cooldown_until <= UTC_TIMESTAMP()) THEN 1 ELSE 0 END), 0) AS available,
  COALESCE(SUM(CASE WHEN status = 2 THEN 1 ELSE 0 END), 0) AS broken,
  COALESCE(SUM(CASE WHEN last_test_status = 1 THEN 1 ELSE 0 END), 0) AS test_ok,
  COALESCE(SUM(CAST(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(oauth_meta, '$.image_quota_remaining')), '0') AS SIGNED)), 0) AS quota_remaining,
  COALESCE(SUM(CAST(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(oauth_meta, '$.image_quota_total')), '0') AS SIGNED)), 0) AS quota_total,
  COALESCE(SUM(success_count), 0) AS success_count,
  COALESCE(SUM(error_count), 0) AS error_count
FROM account
WHERE deleted_at IS NULL
GROUP BY provider
ORDER BY provider ASC`
	if err := r.db.WithContext(ctx).Raw(providerSQL).Scan(&providers).Error; err != nil {
		return nil, err
	}

	var recent []*dashboardRecentRow
	recentSQL := `SELECT
  t.task_id,
  t.created_at,
  COALESCE(NULLIF(u.username, ''), NULLIF(u.email, ''), NULLIF(u.phone, ''), CONCAT('用户 #', t.user_id)) AS user_label,
  t.kind,
  t.model_code,
  t.count,
  t.status,
  t.cost_points
FROM generation_task t
LEFT JOIN ` + "`user`" + ` u ON u.id = t.user_id
WHERE t.deleted_at IS NULL
ORDER BY t.id DESC
LIMIT 8`
	if err := r.db.WithContext(ctx).Raw(recentSQL).Scan(&recent).Error; err != nil {
		return nil, err
	}

	var trendRows []*dashboardTrendRow
	trendSQL := `SELECT
  DATE(created_at) AS date,
  COUNT(1) AS generated_count,
  COALESCE(SUM(cost_points), 0) AS cost_points
FROM generation_task
WHERE deleted_at IS NULL AND created_at >= DATE_SUB(CURDATE(), INTERVAL 6 DAY)
GROUP BY DATE(created_at)
ORDER BY DATE(created_at) ASC`
	if err := r.db.WithContext(ctx).Raw(trendSQL).Scan(&trendRows).Error; err != nil {
		return nil, err
	}
	trendMap := map[string]*dashboardTrendRow{}
	for _, row := range trendRows {
		trendMap[row.Date] = row
	}

	resp := &dto.AdminDashboardOverviewResp{
		GeneratedToday:    gen.GeneratedToday,
		GeneratedTotal:    gen.GeneratedTotal,
		ImageToday:        gen.ImageToday,
		ImageTotal:        gen.ImageTotal,
		VideoToday:        gen.VideoToday,
		VideoTotal:        gen.VideoTotal,
		TextTokensToday:   gen.TextTokensToday,
		TextTokensTotal:   gen.TextTokensTotal,
		CostPointsToday:   gen.CostToday,
		CostPointsTotal:   gen.CostTotal,
		WalletSpendToday:  wallet.SpendToday,
		WalletSpendTotal:  wallet.SpendTotal,
		UsersTotal:        users.UsersTotal,
		UsersToday:        users.UsersToday,
		ActiveUsersToday:  users.ActiveUsersToday,
		AccountProviders:  make([]*dto.DashboardProviderRow, 0, len(providers)),
		RecentGenerations: make([]*dto.DashboardRecentTask, 0, len(recent)),
		Trend:             make([]*dto.DashboardTrendPoint, 0, 7),
	}
	if gen.FinishedToday > 0 {
		resp.SuccessRateToday = float64(gen.SuccessToday) / float64(gen.FinishedToday)
	}
	for _, p := range providers {
		quotaUsed := p.QuotaTotal - p.QuotaRemaining
		if quotaUsed < 0 {
			quotaUsed = 0
		}
		resp.AccountProviders = append(resp.AccountProviders, &dto.DashboardProviderRow{
			Provider:       p.Provider,
			Total:          p.Total,
			Enabled:        p.Enabled,
			Available:      p.Available,
			Broken:         p.Broken,
			TestOK:         p.TestOK,
			QuotaRemaining: p.QuotaRemaining,
			QuotaTotal:     p.QuotaTotal,
			QuotaUsed:      quotaUsed,
			SuccessCount:   p.SuccessCount,
			ErrorCount:     p.ErrorCount,
		})
	}
	for _, row := range recent {
		resp.RecentGenerations = append(resp.RecentGenerations, &dto.DashboardRecentTask{
			TaskID:     row.TaskID,
			CreatedAt:  row.CreatedAt.Unix(),
			UserLabel:  row.UserLabel,
			Kind:       row.Kind,
			ModelCode:  row.ModelCode,
			Count:      row.Count,
			Status:     row.Status,
			CostPoints: row.CostPoints,
		})
	}
	now := time.Now()
	for i := 6; i >= 0; i-- {
		day := now.AddDate(0, 0, -i).Format("2006-01-02")
		point := &dto.DashboardTrendPoint{Date: day}
		if row := trendMap[day]; row != nil {
			point.Generated = row.GeneratedCount
			point.CostPoints = row.CostPoints
		}
		resp.Trend = append(resp.Trend, point)
	}
	return resp, nil
}
