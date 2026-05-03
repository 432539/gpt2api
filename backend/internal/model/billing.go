// Package model 计费相关实体（钱包流水 / 充值 / 消费 / 退款）。
package model

import "time"

// 业务类型常量。
const (
	BizRecharge = "recharge"
	BizConsume  = "consume"
	BizRefund   = "refund"
	BizCDK      = "cdk"
	BizPromo    = "promo"
	BizInvite   = "invite_reward"
	BizGift     = "gift"
)

// WalletLog 点数流水（总账）。
type WalletLog struct {
	ID            uint64    `gorm:"primaryKey;column:id" json:"id"`
	UserID        uint64    `gorm:"column:user_id;not null;index:idx_user_created,priority:1" json:"user_id"`
	Direction     int8      `gorm:"column:direction;not null" json:"direction"` // 1 收入 -1 支出
	BizType       string    `gorm:"column:biz_type;size:32;not null;index:idx_biz,priority:1" json:"biz_type"`
	BizID         string    `gorm:"column:biz_id;size:64;not null;index:idx_biz,priority:2" json:"biz_id"`
	Points        int64     `gorm:"column:points;not null" json:"points"`
	PointsBefore  int64     `gorm:"column:points_before;not null" json:"points_before"`
	PointsAfter   int64     `gorm:"column:points_after;not null" json:"points_after"`
	Remark        *string   `gorm:"column:remark;size:255" json:"remark,omitempty"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime;index:idx_user_created,priority:2" json:"created_at"`
}

// TableName 表名。
func (WalletLog) TableName() string { return "wallet_log" }

// 消费状态。
const (
	ConsumeStatusFrozen   = 0
	ConsumeStatusSettled  = 1
	ConsumeStatusRefunded = 2
)

// ConsumeRecord 消费记录。
type ConsumeRecord struct {
	ID          uint64    `gorm:"primaryKey;column:id" json:"id"`
	TaskID      string    `gorm:"column:task_id;size:26;not null;uniqueIndex:uk_task" json:"task_id"`
	UserID      uint64    `gorm:"column:user_id;not null;index:idx_user_created,priority:1" json:"user_id"`
	Kind        string    `gorm:"column:kind;size:16;not null" json:"kind"`
	ModelCode   string    `gorm:"column:model_code;size:64;not null" json:"model_code"`
	Count       int       `gorm:"column:count;not null" json:"count"`
	UnitPoints  int64     `gorm:"column:unit_points;not null" json:"unit_points"`
	TotalPoints int64     `gorm:"column:total_points;not null" json:"total_points"`
	Status      int8      `gorm:"column:status;not null" json:"status"`
	AccountID   *uint64   `gorm:"column:account_id" json:"account_id,omitempty"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime;index:idx_user_created,priority:2" json:"created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName 表名。
func (ConsumeRecord) TableName() string { return "consume_record" }

// RefundRecord 退款记录。
type RefundRecord struct {
	ID        uint64    `gorm:"primaryKey;column:id" json:"id"`
	TaskID    string    `gorm:"column:task_id;size:26;not null;index" json:"task_id"`
	UserID    uint64    `gorm:"column:user_id;not null;index" json:"user_id"`
	Points    int64     `gorm:"column:points;not null" json:"points"`
	Reason    string    `gorm:"column:reason;size:255;not null" json:"reason"`
	Operator  string    `gorm:"column:operator;size:64;not null;default:system" json:"operator"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

// TableName 表名。
func (RefundRecord) TableName() string { return "refund_record" }
