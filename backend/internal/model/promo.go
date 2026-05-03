// Package model 优惠码 / 兑换码模型。
package model

import "time"

// 优惠码 / CDK 状态。
const (
	PromoStatusEnabled  = 1
	PromoStatusDisabled = 0

	CDKStatusUnused   = 0
	CDKStatusUsed     = 1
	CDKStatusInvalid  = 2
)

// 优惠码折扣类型。
const (
	PromoTypeAmount   = 1 // 满减（CNY *100）
	PromoTypeDiscount = 2 // 折扣（百分比）
	PromoTypeGift     = 3 // 赠点
)

// PromoCode 优惠码。
type PromoCode struct {
	ID            uint64    `gorm:"primaryKey;column:id" json:"id"`
	Code          string    `gorm:"column:code;size:32;not null;uniqueIndex" json:"code"`
	Name          string    `gorm:"column:name;size:64;not null" json:"name"`
	DiscountType  int8      `gorm:"column:discount_type;not null" json:"discount_type"`
	DiscountVal   int64     `gorm:"column:discount_val;not null" json:"discount_val"`
	MinAmount     int64     `gorm:"column:min_amount;not null;default:0" json:"min_amount"`
	ApplyTo       string    `gorm:"column:apply_to;size:64;not null;default:all" json:"apply_to"`
	TotalQty      int       `gorm:"column:total_qty;not null;default:0" json:"total_qty"`
	UsedQty       int       `gorm:"column:used_qty;not null;default:0" json:"used_qty"`
	PerUserLimit  int       `gorm:"column:per_user_limit;not null;default:1" json:"per_user_limit"`
	StartAt       time.Time `gorm:"column:start_at;not null" json:"start_at"`
	EndAt         time.Time `gorm:"column:end_at;not null" json:"end_at"`
	Status        int8      `gorm:"column:status;not null;default:1" json:"status"`
	CreatedBy     *uint64   `gorm:"column:created_by" json:"created_by,omitempty"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName 表名。
func (PromoCode) TableName() string { return "promo_code" }

// Active 当前是否可用。
func (p *PromoCode) Active(now time.Time) bool {
	return p.Status == PromoStatusEnabled && now.After(p.StartAt) && now.Before(p.EndAt)
}

// PromoCodeUse 优惠码使用记录。
type PromoCodeUse struct {
	ID        uint64    `gorm:"primaryKey;column:id" json:"id"`
	PromoID   uint64    `gorm:"column:promo_id;not null;uniqueIndex:uk_promo_user_order,priority:1" json:"promo_id"`
	Code      string    `gorm:"column:code;size:32;not null" json:"code"`
	UserID    uint64    `gorm:"column:user_id;not null;uniqueIndex:uk_promo_user_order,priority:2" json:"user_id"`
	OrderNo   *string   `gorm:"column:order_no;size:32;uniqueIndex:uk_promo_user_order,priority:3" json:"order_no,omitempty"`
	Discount  int64     `gorm:"column:discount;not null" json:"discount"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

// TableName 表名。
func (PromoCodeUse) TableName() string { return "promo_code_use" }

// RedeemCodeBatch CDK 批次。
type RedeemCodeBatch struct {
	ID            uint64     `gorm:"primaryKey;column:id" json:"id"`
	BatchNo       string     `gorm:"column:batch_no;size:32;not null;uniqueIndex" json:"batch_no"`
	Name          string     `gorm:"column:name;size:64;not null" json:"name"`
	RewardType    string     `gorm:"column:reward_type;size:32;not null" json:"reward_type"`
	RewardValue   string     `gorm:"column:reward_value;type:json;not null" json:"reward_value"`
	TotalQty      int        `gorm:"column:total_qty;not null" json:"total_qty"`
	UsedQty       int        `gorm:"column:used_qty;not null;default:0" json:"used_qty"`
	PerUserLimit  int        `gorm:"column:per_user_limit;not null;default:1" json:"per_user_limit"`
	ExpireAt      *time.Time `gorm:"column:expire_at" json:"expire_at,omitempty"`
	Status        int8       `gorm:"column:status;not null;default:1" json:"status"`
	CreatedBy     *uint64    `gorm:"column:created_by" json:"created_by,omitempty"`
	CreatedAt     time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

// TableName 表名。
func (RedeemCodeBatch) TableName() string { return "redeem_code_batch" }

// RedeemCode CDK 单个码。
type RedeemCode struct {
	ID        uint64     `gorm:"primaryKey;column:id" json:"id"`
	BatchID   uint64     `gorm:"column:batch_id;not null;index:idx_batch_status,priority:1" json:"batch_id"`
	Code      string     `gorm:"column:code;size:32;not null;uniqueIndex" json:"code"`
	Status    int8       `gorm:"column:status;not null;default:0;index:idx_batch_status,priority:2" json:"status"`
	UsedBy    *uint64    `gorm:"column:used_by" json:"used_by,omitempty"`
	UsedAt    *time.Time `gorm:"column:used_at" json:"used_at,omitempty"`
	CreatedAt time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

// TableName 表名。
func (RedeemCode) TableName() string { return "redeem_code" }
