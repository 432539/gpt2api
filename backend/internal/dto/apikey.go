// Package dto API Key 入参 / 出参。
package dto

// APIKeyCreateReq 创建 Key。
type APIKeyCreateReq struct {
	Name       string `json:"name"        binding:"required,min=1,max=64"`
	Scope      string `json:"scope"       binding:"omitempty,max=255"`
	RPMLimit   int    `json:"rpm_limit"   binding:"omitempty,min=0,max=10000"`
	DailyQuota int    `json:"daily_quota" binding:"omitempty,min=0"`
	ExpireDays int    `json:"expire_days" binding:"omitempty,min=0,max=3650"`
}

// APIKeyCreateResp 创建返回（含明文，仅一次）。
type APIKeyCreateResp struct {
	ID        uint64 `json:"id"`
	Name      string `json:"name"`
	Plain     string `json:"plain"`
	Prefix    string `json:"prefix"`
	Last4     string `json:"last4"`
	Scope     string `json:"scope"`
	CreatedAt int64  `json:"created_at"`
}

// APIKeyResp 列表 / 详情返回（已脱敏）。
type APIKeyResp struct {
	ID         uint64 `json:"id"`
	Name       string `json:"name"`
	Prefix     string `json:"prefix"`
	Last4      string `json:"last4"`
	Mask       string `json:"mask"`
	Scope      string `json:"scope"`
	RPMLimit   int    `json:"rpm_limit"`
	DailyQuota int    `json:"daily_quota"`
	Status     int8   `json:"status"`
	ExpireAt   int64  `json:"expire_at,omitempty"`
	LastUsedAt int64  `json:"last_used_at,omitempty"`
	CreatedAt  int64  `json:"created_at"`
}
