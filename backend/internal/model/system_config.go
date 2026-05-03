package model

import "time"

// SystemConfig 系统全局 KV 配置。表 `system_config`。
// `value` 字段为 JSON 文本（存任意 JSON 标量 / 对象）。
type SystemConfig struct {
	Key       string    `gorm:"primaryKey;column:key;size:64" json:"key"`
	Value     string    `gorm:"column:value;type:json;not null" json:"value"`
	Remark    *string   `gorm:"column:remark;size:255" json:"remark,omitempty"`
	UpdatedBy *uint64   `gorm:"column:updated_by" json:"updated_by,omitempty"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName 表名。
func (SystemConfig) TableName() string { return "system_config" }
