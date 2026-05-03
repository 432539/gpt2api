// Package model 生成任务相关实体。
package model

import "time"

// 生成任务状态。
const (
	GenStatusPending   = 0
	GenStatusRunning   = 1
	GenStatusSucceeded = 2
	GenStatusFailed    = 3
	GenStatusRefunded  = 4
)

// GenerationTask 生成任务（不区分 image / video，由 kind 区分）。
type GenerationTask struct {
	ID            uint64     `gorm:"primaryKey;column:id" json:"id"`
	TaskID        string     `gorm:"column:task_id;size:26;not null;uniqueIndex:uk_task_id" json:"task_id"`
	UserID        uint64     `gorm:"column:user_id;not null;uniqueIndex:uk_user_idem,priority:1;index:idx_user_kind_status,priority:1" json:"user_id"`
	Kind          string     `gorm:"column:kind;size:16;not null;index:idx_user_kind_status,priority:2" json:"kind"`
	Mode          string     `gorm:"column:mode;size:16;not null" json:"mode"`
	ModelCode     string     `gorm:"column:model_code;size:64;not null" json:"model_code"`
	Prompt        string     `gorm:"column:prompt;type:text;not null" json:"prompt"`
	NegPrompt     *string    `gorm:"column:neg_prompt;type:text" json:"neg_prompt,omitempty"`
	Params        string     `gorm:"column:params;type:json;not null" json:"params"`
	RefAssets     *string    `gorm:"column:ref_assets;type:json" json:"ref_assets,omitempty"`
	Count         int        `gorm:"column:count;not null;default:1" json:"count"`
	CostPoints    int64      `gorm:"column:cost_points;not null" json:"cost_points"`
	IdemKey       string     `gorm:"column:idem_key;size:64;not null;uniqueIndex:uk_user_idem,priority:2" json:"idem_key"`
	AccountID     *uint64    `gorm:"column:account_id" json:"account_id,omitempty"`
	Provider      string     `gorm:"column:provider;size:32;not null" json:"provider"`
	Status        int8       `gorm:"column:status;not null;default:0;index:idx_user_kind_status,priority:3;index:idx_status_created,priority:1" json:"status"`
	Progress      int8       `gorm:"column:progress;not null;default:0" json:"progress"`
	Error         *string    `gorm:"column:error;size:255" json:"error,omitempty"`
	StartedAt     *time.Time `gorm:"column:started_at" json:"started_at,omitempty"`
	FinishedAt    *time.Time `gorm:"column:finished_at" json:"finished_at,omitempty"`
	ClientIP      *string    `gorm:"column:client_ip;size:45" json:"-"`
	FromAPIKeyID  *uint64    `gorm:"column:from_api_key_id" json:"from_api_key_id,omitempty"`
	CreatedAt     time.Time  `gorm:"column:created_at;autoCreateTime;index:idx_status_created,priority:2" json:"created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
	DeletedAt     *time.Time `gorm:"column:deleted_at" json:"-"`
}

// TableName 表名。
func (GenerationTask) TableName() string { return "generation_task" }

// GenerationResult 生成结果。
type GenerationResult struct {
	ID         uint64    `gorm:"primaryKey;column:id" json:"id"`
	TaskID     string    `gorm:"column:task_id;size:26;not null;index" json:"task_id"`
	UserID     uint64    `gorm:"column:user_id;not null" json:"user_id"`
	Kind       string    `gorm:"column:kind;size:16;not null" json:"kind"`
	Seq        int8      `gorm:"column:seq;not null;default:0" json:"seq"`
	URL        string    `gorm:"column:url;size:512;not null" json:"url"`
	ThumbURL   *string   `gorm:"column:thumb_url;size:512" json:"thumb_url,omitempty"`
	Width      *int      `gorm:"column:width" json:"width,omitempty"`
	Height     *int      `gorm:"column:height" json:"height,omitempty"`
	DurationMs *int      `gorm:"column:duration_ms" json:"duration_ms,omitempty"`
	SizeBytes  *int64    `gorm:"column:size_bytes" json:"size_bytes,omitempty"`
	Meta       *string   `gorm:"column:meta;type:json" json:"meta,omitempty"`
	IsPublic   int8      `gorm:"column:is_public;not null;default:0" json:"is_public"`
	CreatedAt  time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

// TableName 表名。
func (GenerationResult) TableName() string { return "generation_result" }
