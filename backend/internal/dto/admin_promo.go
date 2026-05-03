package dto

type AdminPromoListReq struct {
	Keyword      string `form:"keyword" binding:"omitempty,max=128"`
	Status       *int   `form:"status" binding:"omitempty,oneof=0 1"`
	DiscountType *int   `form:"discount_type" binding:"omitempty,oneof=1 2 3"`
	Page         int    `form:"page" binding:"omitempty,min=1"`
	PageSize     int    `form:"page_size" binding:"omitempty,min=1,max=200"`
}

type AdminPromoResp struct {
	ID           uint64 `json:"id"`
	Code         string `json:"code"`
	Name         string `json:"name"`
	DiscountType int8   `json:"discount_type"`
	DiscountVal  int64  `json:"discount_val"`
	MinAmount    int64  `json:"min_amount"`
	ApplyTo      string `json:"apply_to"`
	TotalQty     int    `json:"total_qty"`
	UsedQty      int    `json:"used_qty"`
	PerUserLimit int    `json:"per_user_limit"`
	StartAt      int64  `json:"start_at"`
	EndAt        int64  `json:"end_at"`
	Status       int8   `json:"status"`
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
}

type AdminPromoCreateReq struct {
	Code         string `json:"code" binding:"required,max=32"`
	Name         string `json:"name" binding:"required,max=64"`
	DiscountType int8   `json:"discount_type" binding:"required,oneof=1 2 3"`
	DiscountVal  int64  `json:"discount_val" binding:"required,min=1"`
	MinAmount    int64  `json:"min_amount" binding:"omitempty,min=0"`
	ApplyTo      string `json:"apply_to" binding:"omitempty,max=64"`
	TotalQty     int    `json:"total_qty" binding:"omitempty,min=0"`
	PerUserLimit int    `json:"per_user_limit" binding:"omitempty,min=0"`
	StartAt      int64  `json:"start_at" binding:"omitempty,min=0"`
	EndAt        int64  `json:"end_at" binding:"required,min=1"`
	Status       *int8  `json:"status" binding:"omitempty,oneof=0 1"`
}

type AdminPromoUpdateReq struct {
	Code         *string `json:"code" binding:"omitempty,max=32"`
	Name         *string `json:"name" binding:"omitempty,max=64"`
	DiscountType *int8   `json:"discount_type" binding:"omitempty,oneof=1 2 3"`
	DiscountVal  *int64  `json:"discount_val" binding:"omitempty,min=1"`
	MinAmount    *int64  `json:"min_amount" binding:"omitempty,min=0"`
	ApplyTo      *string `json:"apply_to" binding:"omitempty,max=64"`
	TotalQty     *int    `json:"total_qty" binding:"omitempty,min=0"`
	PerUserLimit *int    `json:"per_user_limit" binding:"omitempty,min=0"`
	StartAt      *int64  `json:"start_at" binding:"omitempty,min=0"`
	EndAt        *int64  `json:"end_at" binding:"omitempty,min=1"`
	Status       *int8   `json:"status" binding:"omitempty,oneof=0 1"`
}
