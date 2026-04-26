// Package image 异步生图任务的数据模型、DAO 以及同步 Runner。
//
// M3 首版采用「同步直出 + 异步查询」混合路线:
//
//   - /v1/images/generations 默认是同步(wait_for_result=true),请求直接
//     阻塞到图片生成完成再返回。由网关层的 goroutine pool 承接并发(目标
//     1000 并发),每个任务落库 + 走一次完整的上游协议链路。
//   - /v1/images/tasks/:id 可作为异步查询入口,客户端也可设
//     wait_for_result=false 拿到 task_id 后自行轮询(适合移动端/脚本)。
//
// 这样能复用现有的 Account + Proxy + 计费 + 限流路径,不引入额外的 Redis
// Stream 基础设施;等并发压力上来后再把 Runner 接到 Stream 消费者即可。
package image

import "time"

// 任务状态。
const (
	StatusQueued     = "queued"     // 已入库,等调度
	StatusDispatched = "dispatched" // 已拿到 lease,未开始跑上游
	StatusRunning    = "running"    // 上游 SSE 已发起
	StatusSuccess    = "success"
	StatusFailed     = "failed"
)

// 错误码(短字符串,便于排查 & 计费对账)。
const (
	ErrUnknown          = "unknown"
	ErrNoAccount        = "no_available_account"
	ErrAuthRequired     = "auth_required"
	ErrRateLimited      = "rate_limited"
	ErrNetworkTransient = "network_transient" // 瞬态网络错误(EOF / reset),可自动重试
	ErrPOWTimeout       = "pow_timeout"
	ErrPOWFailed        = "pow_failed"
	ErrTurnstile        = "turnstile_required"
	ErrUpstream         = "upstream_error"
	ErrUpstreamRejected = "upstream_rejected"
	ErrPollTimeout      = "poll_timeout"
	ErrDownload         = "download_failed"
	ErrInvalidResponse  = "invalid_response"
)

// Task 对应 image_tasks 表。
type Task struct {
	ID              uint64     `db:"id"               json:"id"`
	TaskID          string     `db:"task_id"          json:"task_id"`
	UserID          uint64     `db:"user_id"          json:"user_id"`
	KeyID           uint64     `db:"key_id"           json:"key_id"`
	ModelID         uint64     `db:"model_id"         json:"model_id"`
	AccountID       uint64     `db:"account_id"       json:"account_id"`
	Prompt          string     `db:"prompt"           json:"prompt"`
	N               int        `db:"n"                json:"n"`
	Size            string     `db:"size"             json:"size"`
	Upscale         string     `db:"upscale"          json:"upscale"`
	Status          string     `db:"status"           json:"status"`
	ConversationID  string     `db:"conversation_id"  json:"conversation_id"`
	FileIDs         []byte     `db:"file_ids"         json:"-"`
	ResultURLs      []byte     `db:"result_urls"      json:"-"`
	Error           string     `db:"error"            json:"error"`
	EstimatedCredit int64      `db:"estimated_credit" json:"estimated_credit"`
	CreditCost      int64      `db:"credit_cost"      json:"credit_cost"`
	CreatedAt       time.Time  `db:"created_at"       json:"created_at"`
	StartedAt       *time.Time `db:"started_at"       json:"started_at"`
	FinishedAt      *time.Time `db:"finished_at"      json:"finished_at"`
}

// Result 是 Runner 返回给网关/客户端的生图结果。
type Result struct {
	TaskID         string        `json:"task_id"`
	Status         string        `json:"status"`
	ConversationID string        `json:"conversation_id,omitempty"`
	Images         []ResultImage `json:"images,omitempty"`
	ErrorCode      string        `json:"error_code,omitempty"`
	ErrorMessage   string        `json:"error_message,omitempty"`
	CreditCost     int64         `json:"credit_cost"`
}

// ResultImage 单张生图。
type ResultImage struct {
	URL         string `json:"url"`     // 上游签名直链(短期有效,通常 15 分钟)
	FileID      string `json:"file_id"` // chatgpt.com file-service id(纯 id,不含 sed:)
	IsSediment  bool   `json:"is_sediment,omitempty"`
	ContentType string `json:"content_type,omitempty"`
}
