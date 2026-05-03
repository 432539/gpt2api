package dto

// AdminUserListReq is the admin-side user list query.
type AdminUserListReq struct {
	Keyword  string `form:"keyword" binding:"omitempty,max=128"`
	Status   *int   `form:"status" binding:"omitempty,oneof=0 1"`
	Page     int    `form:"page" binding:"omitempty,min=1"`
	PageSize int    `form:"page_size" binding:"omitempty,min=1,max=200"`
}

type AdminUserResp struct {
	ID            uint64  `json:"id"`
	UUID          string  `json:"uuid"`
	Email         string  `json:"email,omitempty"`
	Phone         string  `json:"phone,omitempty"`
	Username      string  `json:"username,omitempty"`
	Avatar        string  `json:"avatar,omitempty"`
	Points        int64   `json:"points"`
	FrozenPoints  int64   `json:"frozen_points"`
	TotalRecharge int64   `json:"total_recharge"`
	PlanCode      string  `json:"plan_code"`
	PlanExpireAt  int64   `json:"plan_expire_at,omitempty"`
	InviterID     *uint64 `json:"inviter_id,omitempty"`
	InviteCode    string  `json:"invite_code"`
	Status        int8    `json:"status"`
	RegisterIP    string  `json:"register_ip,omitempty"`
	LastLoginAt   int64   `json:"last_login_at,omitempty"`
	LastLoginIP   string  `json:"last_login_ip,omitempty"`
	CreatedAt     int64   `json:"created_at"`
	UpdatedAt     int64   `json:"updated_at"`
}

type AdminUserCreateReq struct {
	Account  string `json:"account" binding:"required,max=128"`
	Password string `json:"password" binding:"required,min=6,max=72"`
	Username string `json:"username" binding:"omitempty,max=64"`
	Points   int64  `json:"points" binding:"omitempty,min=0"`
	Status   *int8  `json:"status" binding:"omitempty,oneof=0 1"`
}

type AdminUserUpdateReq struct {
	Email        *string `json:"email" binding:"omitempty,max=128"`
	Phone        *string `json:"phone" binding:"omitempty,max=20"`
	Username     *string `json:"username" binding:"omitempty,max=64"`
	Avatar       *string `json:"avatar" binding:"omitempty,max=255"`
	Password     *string `json:"password" binding:"omitempty,min=6,max=72"`
	Status       *int8   `json:"status" binding:"omitempty,oneof=0 1"`
	PlanCode     *string `json:"plan_code" binding:"omitempty,max=32"`
	PlanExpireAt *int64  `json:"plan_expire_at" binding:"omitempty,min=0"`
}

type AdminUserAdjustPointsReq struct {
	Action string `json:"action" binding:"required,oneof=recharge deduct"`
	Points int64  `json:"points" binding:"required,min=1"`
	Remark string `json:"remark" binding:"omitempty,max=255"`
}

type AdminUserAdjustPointsResp struct {
	PointsBefore int64 `json:"points_before"`
	PointsAfter  int64 `json:"points_after"`
}
