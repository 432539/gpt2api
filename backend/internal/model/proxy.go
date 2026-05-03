package model

import "time"

// Proxy 协议常量。
const (
	ProxyProtoHTTP    = "http"
	ProxyProtoHTTPS   = "https"
	ProxyProtoSOCKS5  = "socks5"
	ProxyProtoSOCKS5H = "socks5h"
)

// Proxy 状态。
const (
	ProxyStatusEnabled  = 1
	ProxyStatusDisabled = 0
)

// 测试结果。
const (
	ProxyCheckUnknown = 0
	ProxyCheckOK      = 1
	ProxyCheckFail    = 2
)

// Proxy 出站代理实体。表 `proxy`。
type Proxy struct {
	ID           uint64     `gorm:"primaryKey;column:id" json:"id"`
	Name         string     `gorm:"column:name;size:128;not null" json:"name"`
	Protocol     string     `gorm:"column:protocol;size:16;not null" json:"protocol"`
	Host         string     `gorm:"column:host;size:255;not null" json:"host"`
	Port         uint16     `gorm:"column:port;not null" json:"port"`
	Username     *string    `gorm:"column:username;size:255" json:"username,omitempty"`
	PasswordEnc  []byte     `gorm:"column:password_enc;type:blob" json:"-"`
	Status       int8       `gorm:"column:status;not null;default:1" json:"status"`
	LastCheckAt  *time.Time `gorm:"column:last_check_at" json:"last_check_at,omitempty"`
	LastCheckOK  int8       `gorm:"column:last_check_ok;not null;default:0" json:"last_check_ok"`
	LastCheckMs  int        `gorm:"column:last_check_ms;not null;default:0" json:"last_check_ms"`
	LastError    *string    `gorm:"column:last_error;size:255" json:"last_error,omitempty"`
	Remark       *string    `gorm:"column:remark;size:255" json:"remark,omitempty"`
	CreatedBy    *uint64    `gorm:"column:created_by" json:"created_by,omitempty"`
	CreatedAt    time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
	DeletedAt    *time.Time `gorm:"column:deleted_at;index" json:"-"`
}

// TableName 表名。
func (Proxy) TableName() string { return "proxy" }
