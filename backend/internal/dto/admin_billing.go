package dto

type AdminWalletLogListReq struct {
	Keyword   string `form:"keyword" binding:"omitempty,max=128"`
	UserID    uint64 `form:"user_id" binding:"omitempty,min=1"`
	BizType   string `form:"biz_type" binding:"omitempty,max=32"`
	Direction *int   `form:"direction" binding:"omitempty,oneof=-1 1"`
	Page      int    `form:"page" binding:"omitempty,min=1"`
	PageSize  int    `form:"page_size" binding:"omitempty,min=1,max=200"`
}

type AdminWalletLogResp struct {
	ID           uint64 `json:"id"`
	CreatedAt    int64  `json:"created_at"`
	UserID       uint64 `json:"user_id"`
	UserLabel    string `json:"user_label"`
	Direction    int8   `json:"direction"`
	BizType      string `json:"biz_type"`
	BizID        string `json:"biz_id"`
	Points       int64  `json:"points"`
	PointsBefore int64  `json:"points_before"`
	PointsAfter  int64  `json:"points_after"`
	Remark       string `json:"remark,omitempty"`
}
