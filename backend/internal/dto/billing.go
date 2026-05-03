// Package dto 计费相关 DTO。
package dto

// CDKRedeemReq 兑换 CDK。
type CDKRedeemReq struct {
	Code string `json:"code" binding:"required,min=4,max=32"`
}

// WalletLogResp 钱包流水响应（一行）。
type WalletLogResp struct {
	ID           uint64 `json:"id"`
	Direction    int8   `json:"direction"`
	BizType      string `json:"biz_type"`
	BizID        string `json:"biz_id"`
	Points       int64  `json:"points"`
	PointsBefore int64  `json:"points_before"`
	PointsAfter  int64  `json:"points_after"`
	Remark       string `json:"remark,omitempty"`
	CreatedAt    int64  `json:"created_at"`
}

// CDKBatchCreateReq 管理后台创建 CDK 批次。
type CDKBatchCreateReq struct {
	BatchNo      string `json:"batch_no"       binding:"required,min=4,max=32"`
	Name         string `json:"name"           binding:"required,min=1,max=64"`
	Points       int64  `json:"points"         binding:"required,min=1"`
	Qty          int    `json:"qty"            binding:"required,min=1,max=100000"`
	PerUserLimit int    `json:"per_user_limit" binding:"omitempty,min=0"`
	ExpireAt     int64  `json:"expire_at"      binding:"omitempty,min=0"` // unix
}
