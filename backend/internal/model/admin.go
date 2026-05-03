// Package model 管理后台实体。
package model

import "time"

// AdminUser 后台账号。
type AdminUser struct {
	ID          uint64     `gorm:"primaryKey;column:id" json:"id"`
	Username    string     `gorm:"column:username;size:64;not null;uniqueIndex" json:"username"`
	Password    string     `gorm:"column:password;size:72;not null" json:"-"`
	Nickname    *string    `gorm:"column:nickname;size:64" json:"nickname,omitempty"`
	Email       *string    `gorm:"column:email;size:128" json:"email,omitempty"`
	RoleID      uint64     `gorm:"column:role_id;not null;index" json:"role_id"`
	Status      int8       `gorm:"column:status;not null;default:1" json:"status"`
	LastLoginAt *time.Time `gorm:"column:last_login_at" json:"last_login_at,omitempty"`
	LastLoginIP *string    `gorm:"column:last_login_ip;size:45" json:"-"`
	CreatedAt   time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
	DeletedAt   *time.Time `gorm:"column:deleted_at" json:"-"`
}

// TableName 表名。
func (AdminUser) TableName() string { return "admin_user" }

// IsActive 是否可登录。
func (a *AdminUser) IsActive() bool { return a.Status == 1 }

// AdminRole 后台角色。
type AdminRole struct {
	ID        uint64    `gorm:"primaryKey;column:id" json:"id"`
	Name      string    `gorm:"column:name;size:64;not null" json:"name"`
	Code      string    `gorm:"column:code;size:32;not null;uniqueIndex" json:"code"`
	Remark    *string   `gorm:"column:remark;size:255" json:"remark,omitempty"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName 表名。
func (AdminRole) TableName() string { return "admin_role" }
