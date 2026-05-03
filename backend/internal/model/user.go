// Package model 定义 GORM 实体。字段必须带 gorm tag。
package model

import "time"

// User 用户实体。表 `user`。
type User struct {
	ID            uint64     `gorm:"primaryKey;column:id" json:"id"`
	UUID          string     `gorm:"column:uuid;size:36;uniqueIndex" json:"uuid"`
	Email         *string    `gorm:"column:email;size:128;uniqueIndex" json:"email,omitempty"`
	Phone         *string    `gorm:"column:phone;size:20;uniqueIndex" json:"phone,omitempty"`
	Username      *string    `gorm:"column:username;size:64" json:"username,omitempty"`
	Avatar        *string    `gorm:"column:avatar;size:255" json:"avatar,omitempty"`
	Password      string     `gorm:"column:password;size:72;not null" json:"-"`
	Points        int64      `gorm:"column:points;not null;default:0" json:"points"`
	FrozenPoints  int64      `gorm:"column:frozen_points;not null;default:0" json:"frozen_points"`
	TotalRecharge int64      `gorm:"column:total_recharge;not null;default:0" json:"total_recharge"`
	PlanCode      string     `gorm:"column:plan_code;size:32;not null;default:free" json:"plan_code"`
	PlanExpireAt  *time.Time `gorm:"column:plan_expire_at" json:"plan_expire_at,omitempty"`
	InviterID     *uint64    `gorm:"column:inviter_id" json:"inviter_id,omitempty"`
	InviteCode    string     `gorm:"column:invite_code;size:16;uniqueIndex" json:"invite_code"`
	Status        int8       `gorm:"column:status;not null;default:1" json:"status"`
	RegisterIP    *string    `gorm:"column:register_ip;size:45" json:"-"`
	LastLoginAt   *time.Time `gorm:"column:last_login_at" json:"last_login_at,omitempty"`
	LastLoginIP   *string    `gorm:"column:last_login_ip;size:45" json:"-"`
	CreatedAt     time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
	DeletedAt     *time.Time `gorm:"column:deleted_at;index" json:"-"`
}

// TableName 自定义表名。
func (User) TableName() string { return "user" }

// IsActive 用户是否可正常登录使用。
func (u *User) IsActive() bool { return u.Status == 1 }
