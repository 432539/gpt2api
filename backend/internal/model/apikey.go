// Package model API Key 实体。
package model

import "time"

// APIKey 用户 API Key。表 `api_key`。
//
// DB 中只保存：prefix（前缀展示）、hash（SHA256(plaintext+salt)）、salt、last4。
// 明文仅在创建时返回一次。
type APIKey struct {
	ID         uint64     `gorm:"primaryKey;column:id" json:"id"`
	UserID     uint64     `gorm:"column:user_id;not null;index:idx_user_status,priority:1" json:"user_id"`
	Name       string     `gorm:"column:name;size:64;not null" json:"name"`
	Prefix     string     `gorm:"column:prefix;size:16;not null" json:"prefix"`
	Hash       string     `gorm:"column:hash;size:64;not null;uniqueIndex" json:"-"`
	Salt       string     `gorm:"column:salt;size:32;not null" json:"-"`
	Last4      string     `gorm:"column:last4;size:4;not null" json:"last4"`
	Scope      string     `gorm:"column:scope;size:255;not null;default:chat,image,video" json:"scope"`
	RPMLimit   int        `gorm:"column:rpm_limit;not null;default:60" json:"rpm_limit"`
	DailyQuota int        `gorm:"column:daily_quota;not null;default:0" json:"daily_quota"`
	ExpireAt   *time.Time `gorm:"column:expire_at" json:"expire_at,omitempty"`
	LastUsedAt *time.Time `gorm:"column:last_used_at" json:"last_used_at,omitempty"`
	Status     int8       `gorm:"column:status;not null;default:1;index:idx_user_status,priority:2" json:"status"`
	CreatedAt  time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
	DeletedAt  *time.Time `gorm:"column:deleted_at" json:"-"`
}

// TableName 表名。
func (APIKey) TableName() string { return "api_key" }

// IsActive Key 当前是否可用。
func (k *APIKey) IsActive(now time.Time) bool {
	if k.Status != 1 {
		return false
	}
	if k.ExpireAt != nil && now.After(*k.ExpireAt) {
		return false
	}
	return true
}
