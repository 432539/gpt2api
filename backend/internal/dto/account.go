// Package dto 入参 / 出参 DTO。
package dto

// AccountCreateReq 创建账号。credential 为明文（api_key / cookie / 或 OAuth 仅 RT 时的唯一字段）。
// OAuth（GPT）推荐与 sora2ok 一致：access_token（AT）+ refresh_token（RT）+ session_token（ST，可选）+ client_id（可选）；
// 仍可只填 credential 作为仅 RT 的旧版表单。
type AccountCreateReq struct {
	Provider     string  `json:"provider"      binding:"required,oneof=gpt grok"`
	Name         string  `json:"name"          binding:"required,min=1,max=128"`
	AuthType     string  `json:"auth_type"     binding:"required,oneof=api_key cookie oauth"`
	Credential   string  `json:"credential"    binding:"omitempty"`
	AccessToken  string  `json:"access_token"  binding:"omitempty"`
	RefreshToken string  `json:"refresh_token" binding:"omitempty"`
	SessionToken string  `json:"session_token" binding:"omitempty"`
	ClientID     string  `json:"client_id"     binding:"omitempty,max=128"`
	BaseURL      string  `json:"base_url"      binding:"omitempty,url"`
	ProxyID      *uint64 `json:"proxy_id"      binding:"omitempty"`
	Weight       int     `json:"weight"        binding:"omitempty,min=1,max=1000"`
	RPMLimit     int     `json:"rpm_limit"     binding:"omitempty,min=0"`
	TPMLimit     int     `json:"tpm_limit"     binding:"omitempty,min=0"`
	DailyQuota   int     `json:"daily_quota"   binding:"omitempty,min=0"`
	MonthlyQuota int     `json:"monthly_quota" binding:"omitempty,min=0"`
	Remark       string  `json:"remark"        binding:"omitempty,max=255"`
}

// AccountUpdateReq 更新账号；任意字段留空表示不变。
// OAuth 账号可使用 access_token / refresh_token / session_token / client_id 替换原值。
type AccountUpdateReq struct {
	Name         *string `json:"name"          binding:"omitempty,min=1,max=128"`
	Credential   *string `json:"credential"`
	AccessToken  *string `json:"access_token"`
	RefreshToken *string `json:"refresh_token"`
	SessionToken *string `json:"session_token"`
	ClientID     *string `json:"client_id"     binding:"omitempty,max=128"`
	BaseURL      *string `json:"base_url"      binding:"omitempty,url"`
	ProxyID      *uint64 `json:"proxy_id"      binding:"omitempty"`
	Weight       *int    `json:"weight"        binding:"omitempty,min=1,max=1000"`
	RPMLimit     *int    `json:"rpm_limit"     binding:"omitempty,min=0"`
	TPMLimit     *int    `json:"tpm_limit"     binding:"omitempty,min=0"`
	DailyQuota   *int    `json:"daily_quota"   binding:"omitempty,min=0"`
	MonthlyQuota *int    `json:"monthly_quota" binding:"omitempty,min=0"`
	Status       *int8   `json:"status"        binding:"omitempty,oneof=-1 0 1 2"`
	Remark       *string `json:"remark"        binding:"omitempty,max=255"`
}

// AccountBatchImportReq 批量导入。
//
// 文本输入示例（每行一条）：
//
//	sk-xxxxx
//	sk-yyyyy@https://api.example.com
//	namedbob@@sk-zzzzz
//
// 当 auth_type=oauth 时，每行的 credential 实际为 OpenAI Codex CLI 的 refresh_token。
//
// format=sub2api：兼容 sub2api / Codex 等导出的 JSON（顶层含 accounts[]）。
// 单次请求 accounts 建议 ≤500 条，大块导入请前端分批 POST。
type AccountBatchImportReq struct {
	Format   string `json:"format"    binding:"omitempty,oneof=lines sub2api"`
	Provider string `json:"provider"  binding:"required,oneof=gpt grok"`
	// lines 模式必填；sub2api 可省略（由每条 account.type / platform 推导）
	AuthType string  `json:"auth_type" binding:"omitempty,oneof=api_key cookie oauth"`
	BaseURL  string  `json:"base_url"  binding:"omitempty,url"`
	ProxyID  *uint64 `json:"proxy_id" binding:"omitempty"`
	Weight   int     `json:"weight"    binding:"omitempty,min=1,max=1000"`
	// lines 模式：多行文本
	Text string `json:"text"`
	// sub2api 模式：解析后的账号切片
	Accounts []Sub2APIAccountItem `json:"accounts"`
}

// Sub2APIAccountItem sub2api 导出 JSON 中单条账号（字段名与常见导出保持一致）。
type Sub2APIAccountItem struct {
	Name        string        `json:"name"`
	Platform    string        `json:"platform"`
	Type        string        `json:"type"`
	Priority    int           `json:"priority"`
	Concurrency int           `json:"concurrency"`
	Credentials *Sub2APICreds `json:"credentials"`
}

// BatchImportResult 批量导入结果。
type BatchImportResult struct {
	Imported int `json:"imported"`
	Skipped  int `json:"skipped"`
}

// Sub2APICreds sub2api credentials 对象。
type Sub2APICreds struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ClientID         string `json:"client_id"`
	IDToken          string `json:"id_token"`
	Email            string `json:"email"`
	ChatgptAccountID string `json:"chatgpt_account_id"`
	ChatgptUserID    string `json:"chatgpt_user_id"`
	OrganizationID   string `json:"organization_id"`
	PlanType         string `json:"plan_type"`
}

// AccountTestResp 账号连通性测试结果。
type AccountTestResp struct {
	OK                  bool   `json:"ok"`
	LatencyMs           int    `json:"latency_ms"`
	Error               string `json:"error,omitempty"`
	PlanType            string `json:"plan_type,omitempty"`
	DefaultModel        string `json:"default_model,omitempty"`
	ImageQuotaRemaining int    `json:"image_quota_remaining,omitempty"`
	ImageQuotaTotal     int    `json:"image_quota_total,omitempty"`
	ImageQuotaResetAt   int64  `json:"image_quota_reset_at,omitempty"`
}

// AccountRefreshResp OAuth RT 刷新结果。
type AccountRefreshResp struct {
	OK           bool  `json:"ok"`
	ExpiresIn    int64 `json:"expires_in,omitempty"`
	RefreshedAt  int64 `json:"refreshed_at"`
	HasRefreshTK bool  `json:"has_refresh_token"`
}

// AccountListReq 列表过滤。
type AccountBatchProbeResp struct {
	Probed    int      `json:"probed"`
	FailedIDs []uint64 `json:"failed_ids"`
	Page      int      `json:"page"`
	PageSize  int      `json:"page_size"`
	Total     int64    `json:"total"`
	HasMore   bool     `json:"has_more"`
	NextPage  int      `json:"next_page,omitempty"`
}

// AccountBatchRefreshResp 批量刷新 OAuth 结果。
type AccountBatchRefreshResp struct {
	Refreshed int      `json:"refreshed"`
	FailedIDs []uint64 `json:"failed_ids"`
	Page      int      `json:"page"`
	PageSize  int      `json:"page_size"`
	Total     int64    `json:"total"`
	HasMore   bool     `json:"has_more"`
	NextPage  int      `json:"next_page,omitempty"`
}

type AccountListReq struct {
	Provider string `form:"provider"  binding:"omitempty,oneof=gpt grok"`
	Status   *int8  `form:"status"`
	Keyword  string `form:"keyword"   binding:"omitempty,max=64"`
	Page     int    `form:"page"      binding:"omitempty,min=1"`
	PageSize int    `form:"page_size" binding:"omitempty,min=1,max=1000"`
}

// AccountResp 输出，已脱敏 credential。
type AccountResp struct {
	ID                  uint64 `json:"id"`
	Provider            string `json:"provider"`
	Name                string `json:"name"`
	AuthType            string `json:"auth_type"`
	CredentialMask      string `json:"credential_mask"`
	BaseURL             string `json:"base_url,omitempty"`
	ProxyID             uint64 `json:"proxy_id,omitempty"`
	Weight              int    `json:"weight"`
	RPMLimit            int    `json:"rpm_limit"`
	TPMLimit            int    `json:"tpm_limit"`
	DailyQuota          int    `json:"daily_quota"`
	MonthlyQuota        int    `json:"monthly_quota"`
	Status              int8   `json:"status"`
	CooldownUntil       int64  `json:"cooldown_until,omitempty"`
	LastUsedAt          int64  `json:"last_used_at,omitempty"`
	LastError           string `json:"last_error,omitempty"`
	ErrorCount          int    `json:"error_count"`
	SuccessCount        uint64 `json:"success_count"`
	Remark              string `json:"remark,omitempty"`
	HasRefreshToken     bool   `json:"has_refresh_token"`
	HasAccessToken      bool   `json:"has_access_token"`
	AccessTokenExpireAt int64  `json:"access_token_expire_at,omitempty"`
	LastRefreshAt       int64  `json:"last_refresh_at,omitempty"`
	LastTestAt          int64  `json:"last_test_at,omitempty"`
	LastTestStatus      int8   `json:"last_test_status"`
	LastTestLatencyMs   int    `json:"last_test_latency_ms"`
	LastTestError       string `json:"last_test_error,omitempty"`
	PlanType            string `json:"plan_type,omitempty"`
	DefaultModel        string `json:"default_model,omitempty"`
	ImageQuotaRemaining int    `json:"image_quota_remaining,omitempty"`
	ImageQuotaTotal     int    `json:"image_quota_total,omitempty"`
	ImageQuotaResetAt   int64  `json:"image_quota_reset_at,omitempty"`
	CreatedAt           int64  `json:"created_at"`
	UpdatedAt           int64  `json:"updated_at"`
}

// AccountBatchDeleteReq 按 ID 批量软删（最多 2000 条）。
type AccountBatchDeleteReq struct {
	IDs []uint64 `json:"ids" binding:"required,min=1,max=2000,dive,min=1"`
}

// AccountPurgeReq 按条件批量软删。
// invalid：status∈{0,2} 或 last_test_status=失败。
// all：当前列表中未软删的全部账号；须 confirm=DELETE_ALL_ACCOUNTS。
type AccountPurgeReq struct {
	Scope    string `json:"scope" binding:"required,oneof=all invalid zero_quota"`
	Provider string `json:"provider" binding:"omitempty,oneof=gpt grok"`
	Confirm  string `json:"confirm"`
}

// AccountBulkOpResult 批量删除结果。
type AccountBulkOpResult struct {
	Deleted int64 `json:"deleted"`
}

// AccountSecretsResp 仅管理员可见，返回单个账号的明文凭证。
// 用于编辑面板回显已存值；解密失败的字段返回空串。
type AccountSecretsResp struct {
	Credential   string `json:"credential,omitempty"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	SessionToken string `json:"session_token,omitempty"`
	ClientID     string `json:"client_id,omitempty"`
}
