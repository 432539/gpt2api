// Package model 定义 GORM 实体。
package model

import "time"

// Provider 提供商。
const (
	ProviderGPT  = "gpt"
	ProviderGROK = "grok"
)

// AuthType 认证类型。
const (
	AuthTypeAPIKey = "api_key"
	AuthTypeCookie = "cookie"
	AuthTypeOAuth  = "oauth"
)

// Account 状态：1启用 0停用 2熔断 -1禁用。
const (
	AccountStatusEnabled  = 1
	AccountStatusDisabled = 0
	AccountStatusBroken   = 2
	AccountStatusBanned   = -1
)

// 测试结果：0未测 1OK 2失败。
const (
	AccountTestUnknown = 0
	AccountTestOK      = 1
	AccountTestFail    = 2
)

// Account 第三方账号实体。表 `account`。
type Account struct {
	ID                   uint64     `gorm:"primaryKey;column:id" json:"id"`
	Provider             string     `gorm:"column:provider;size:32;not null;index:idx_provider_status,priority:1" json:"provider"`
	Name                 string     `gorm:"column:name;size:128;not null" json:"name"`
	AuthType             string     `gorm:"column:auth_type;size:32;not null" json:"auth_type"`
	CredentialEnc        []byte     `gorm:"column:credential_enc;type:blob;not null" json:"-"`
	OAuthMeta            *string    `gorm:"column:oauth_meta;type:json" json:"oauth_meta,omitempty"`
	AccessTokenEnc       []byte     `gorm:"column:access_token_enc;type:blob" json:"-"`
	RefreshTokenEnc      []byte     `gorm:"column:refresh_token_enc;type:blob" json:"-"`
	SessionTokenEnc      []byte     `gorm:"column:session_token_enc;type:blob" json:"-"`
	AccessTokenExpiresAt *time.Time `gorm:"column:access_token_expires_at" json:"access_token_expires_at,omitempty"`
	LastRefreshAt        *time.Time `gorm:"column:last_refresh_at" json:"last_refresh_at,omitempty"`
	BaseURL              *string    `gorm:"column:base_url;size:255" json:"base_url,omitempty"`
	ProxyID              *uint64    `gorm:"column:proxy_id" json:"proxy_id,omitempty"`
	ModelWhitelist       *string    `gorm:"column:model_whitelist;type:json" json:"model_whitelist,omitempty"`
	Weight               int        `gorm:"column:weight;not null;default:10" json:"weight"`
	RPMLimit             int        `gorm:"column:rpm_limit;not null;default:0" json:"rpm_limit"`
	TPMLimit             int        `gorm:"column:tpm_limit;not null;default:0" json:"tpm_limit"`
	DailyQuota           int        `gorm:"column:daily_quota;not null;default:0" json:"daily_quota"`
	MonthlyQuota         int        `gorm:"column:monthly_quota;not null;default:0" json:"monthly_quota"`
	Status               int8       `gorm:"column:status;not null;default:1;index:idx_provider_status,priority:2" json:"status"`
	CooldownUntil        *time.Time `gorm:"column:cooldown_until" json:"cooldown_until,omitempty"`
	LastUsedAt           *time.Time `gorm:"column:last_used_at" json:"last_used_at,omitempty"`
	LastError            *string    `gorm:"column:last_error;size:255" json:"last_error,omitempty"`
	LastTestAt           *time.Time `gorm:"column:last_test_at" json:"last_test_at,omitempty"`
	LastTestStatus       int8       `gorm:"column:last_test_status;not null;default:0" json:"last_test_status"`
	LastTestLatencyMs    int        `gorm:"column:last_test_latency_ms;not null;default:0" json:"last_test_latency_ms"`
	LastTestError        *string    `gorm:"column:last_test_error;size:255" json:"last_test_error,omitempty"`
	ErrorCount           int        `gorm:"column:error_count;not null;default:0" json:"error_count"`
	SuccessCount         uint64     `gorm:"column:success_count;not null;default:0" json:"success_count"`
	Remark               *string    `gorm:"column:remark;size:255" json:"remark,omitempty"`
	CreatedBy            *uint64    `gorm:"column:created_by" json:"created_by,omitempty"`
	CreatedAt            time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt            time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
	DeletedAt            *time.Time `gorm:"column:deleted_at;index" json:"-"`
}

// IsOAuth 判断是否 OAuth 账号（含 RT 刷新逻辑）。
func (a *Account) IsOAuth() bool {
	return a.AuthType == AuthTypeOAuth
}

// TableName 表名。
func (Account) TableName() string { return "account" }

// Available 是否处于可调度状态。
func (a *Account) Available(now time.Time) bool {
	if a.Status != AccountStatusEnabled {
		return false
	}
	if a.CooldownUntil != nil && now.Before(*a.CooldownUntil) {
		return false
	}
	return true
}

// AccountGroup 账号池分组。
type AccountGroup struct {
	ID        uint64     `gorm:"primaryKey;column:id" json:"id"`
	Provider  string     `gorm:"column:provider;size:32;not null" json:"provider"`
	Code      string     `gorm:"column:code;size:64;not null;uniqueIndex:uk_provider_code,priority:2" json:"code"`
	Name      string     `gorm:"column:name;size:128;not null" json:"name"`
	Strategy  string     `gorm:"column:strategy;size:32;not null;default:round_robin" json:"strategy"`
	Status    int8       `gorm:"column:status;not null;default:1" json:"status"`
	Remark    *string    `gorm:"column:remark;size:255" json:"remark,omitempty"`
	CreatedAt time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
	DeletedAt *time.Time `gorm:"column:deleted_at" json:"-"`
}

// TableName 表名。
func (AccountGroup) TableName() string { return "account_group" }

// AccountGroupMember 账号-分组成员。
type AccountGroupMember struct {
	GroupID   uint64    `gorm:"primaryKey;column:group_id" json:"group_id"`
	AccountID uint64    `gorm:"primaryKey;column:account_id" json:"account_id"`
	Weight    int       `gorm:"column:weight;not null;default:10" json:"weight"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

// TableName 表名。
func (AccountGroupMember) TableName() string { return "account_group_member" }
