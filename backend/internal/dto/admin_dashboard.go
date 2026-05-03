package dto

type AdminDashboardOverviewResp struct {
	GeneratedToday    int64                   `json:"generated_today"`
	GeneratedTotal    int64                   `json:"generated_total"`
	ImageToday        int64                   `json:"image_today"`
	ImageTotal        int64                   `json:"image_total"`
	VideoToday        int64                   `json:"video_today"`
	VideoTotal        int64                   `json:"video_total"`
	TextTokensToday   int64                   `json:"text_tokens_today"`
	TextTokensTotal   int64                   `json:"text_tokens_total"`
	CostPointsToday   int64                   `json:"cost_points_today"`
	CostPointsTotal   int64                   `json:"cost_points_total"`
	WalletSpendToday  int64                   `json:"wallet_spend_today"`
	WalletSpendTotal  int64                   `json:"wallet_spend_total"`
	UsersTotal        int64                   `json:"users_total"`
	UsersToday        int64                   `json:"users_today"`
	ActiveUsersToday  int64                   `json:"active_users_today"`
	SuccessRateToday  float64                 `json:"success_rate_today"`
	AccountProviders  []*DashboardProviderRow `json:"account_providers"`
	RecentGenerations []*DashboardRecentTask  `json:"recent_generations"`
	Trend             []*DashboardTrendPoint  `json:"trend"`
}

type DashboardProviderRow struct {
	Provider       string `json:"provider"`
	Total          int64  `json:"total"`
	Enabled        int64  `json:"enabled"`
	Available      int64  `json:"available"`
	Broken         int64  `json:"broken"`
	TestOK         int64  `json:"test_ok"`
	QuotaRemaining int64  `json:"quota_remaining"`
	QuotaTotal     int64  `json:"quota_total"`
	QuotaUsed      int64  `json:"quota_used"`
	SuccessCount   int64  `json:"success_count"`
	ErrorCount     int64  `json:"error_count"`
}

type DashboardRecentTask struct {
	TaskID     string `json:"task_id"`
	CreatedAt  int64  `json:"created_at"`
	UserLabel  string `json:"user_label"`
	Kind       string `json:"kind"`
	ModelCode  string `json:"model_code"`
	Count      int    `json:"count"`
	Status     int8   `json:"status"`
	CostPoints int64  `json:"cost_points"`
}

type DashboardTrendPoint struct {
	Date       string `json:"date"`
	Generated  int64  `json:"generated"`
	CostPoints int64  `json:"cost_points"`
}
