package dto

type AdminGenerationLogListReq struct {
	Keyword  string `form:"keyword" binding:"omitempty,max=128"`
	Kind     string `form:"kind" binding:"omitempty,oneof=image video chat text"`
	Status   *int   `form:"status" binding:"omitempty,oneof=0 1 2 3 4"`
	Page     int    `form:"page" binding:"omitempty,min=1"`
	PageSize int    `form:"page_size" binding:"omitempty,min=1,max=200"`
}

type AdminGenerationLogResp struct {
	TaskID     string `json:"task_id"`
	CreatedAt  int64  `json:"created_at"`
	UserID     uint64 `json:"user_id"`
	UserLabel  string `json:"user_label"`
	APIKeyID   uint64 `json:"api_key_id,omitempty"`
	KeyLabel   string `json:"key_label,omitempty"`
	Kind       string `json:"kind"`
	ModelCode  string `json:"model_code"`
	Prompt     string `json:"prompt"`
	Status     int8   `json:"status"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	CostPoints int64  `json:"cost_points"`
	PreviewURL string `json:"preview_url,omitempty"`
	Error      string `json:"error,omitempty"`
}

type AdminGenerationLogPurgeReq struct {
	Days int `json:"days" binding:"required,min=1,max=3650"`
}

type AdminGenerationLogPurgeResp struct {
	Deleted int64 `json:"deleted"`
}

type AdminGenerationUpstreamLogResp struct {
	ID              uint64  `json:"id"`
	TaskID          string  `json:"task_id"`
	Provider        string  `json:"provider"`
	AccountID       *uint64 `json:"account_id,omitempty"`
	Stage           string  `json:"stage"`
	Method          string  `json:"method,omitempty"`
	URL             string  `json:"url,omitempty"`
	StatusCode      int     `json:"status_code"`
	DurationMs      int64   `json:"duration_ms"`
	RequestExcerpt  string  `json:"request_excerpt,omitempty"`
	ResponseExcerpt string  `json:"response_excerpt,omitempty"`
	Error           string  `json:"error,omitempty"`
	Meta            string  `json:"meta,omitempty"`
	CreatedAt       int64   `json:"created_at"`
}
