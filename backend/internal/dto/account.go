// Package dto 入参 / 出参 DTO。
package dto

// AccountCreateReq 创建账号。credential 为明文，服务层负责加密。
type AccountCreateReq struct {
	Provider     string  `json:"provider"      binding:"required,oneof=gpt grok"`
	Name         string  `json:"name"          binding:"required,min=1,max=128"`
	AuthType     string  `json:"auth_type"     binding:"required,oneof=api_key cookie oauth"`
	Credential   string  `json:"credential"    binding:"required"`
	BaseURL      string  `json:"base_url"      binding:"omitempty,url"`
	ProxyID      *uint64 `json:"proxy_id"      binding:"omitempty"`
	Weight       int     `json:"weight"        binding:"omitempty,min=1,max=1000"`
	RPMLimit     int     `json:"rpm_limit"     binding:"omitempty,min=0"`
	TPMLimit     int     `json:"tpm_limit"     binding:"omitempty,min=0"`
	DailyQuota   int     `json:"daily_quota"   binding:"omitempty,min=0"`
	MonthlyQuota int     `json:"monthly_quota" binding:"omitempty,min=0"`
	Remark       string  `json:"remark"        binding:"omitempty,max=255"`
}

// AccountUpdateReq 更新账号；credential 留空表示不变。
type AccountUpdateReq struct {
	Name         *string `json:"name"          binding:"omitempty,min=1,max=128"`
	Credential   *string `json:"credential"`
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
//   sk-xxxxx
//   sk-yyyyy@https://api.example.com
//   namedbob@@sk-zzzzz
type AccountBatchImportReq struct {
	Provider string `json:"provider"  binding:"required,oneof=gpt grok"`
	AuthType string `json:"auth_type" binding:"required,oneof=api_key cookie oauth"`
	BaseURL  string `json:"base_url"  binding:"omitempty,url"`
	Weight   int    `json:"weight"    binding:"omitempty,min=1,max=1000"`
	Text     string `json:"text"      binding:"required"`
}

// AccountTestResp 账号连通性测试结果。
type AccountTestResp struct {
	OK        bool   `json:"ok"`
	LatencyMs int    `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

// AccountRefreshResp OAuth RT 刷新结果。
type AccountRefreshResp struct {
	OK           bool  `json:"ok"`
	ExpiresIn    int64 `json:"expires_in,omitempty"`
	RefreshedAt  int64 `json:"refreshed_at"`
	HasRefreshTK bool  `json:"has_refresh_token"`
}

// AccountListReq 列表过滤。
type AccountListReq struct {
	Provider string `form:"provider"  binding:"omitempty,oneof=gpt grok"`
	Status   *int8  `form:"status"`
	Keyword  string `form:"keyword"   binding:"omitempty,max=64"`
	Page     int    `form:"page"      binding:"omitempty,min=1"`
	PageSize int    `form:"page_size" binding:"omitempty,min=1,max=200"`
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
	CreatedAt           int64  `json:"created_at"`
	UpdatedAt           int64  `json:"updated_at"`
}
