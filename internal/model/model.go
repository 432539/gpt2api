package model

import (
	"database/sql"
	"time"
)

// 模型类型。
const (
	TypeChat  = "chat"
	TypeImage = "image"
)

// Provider 标识上游服务商。空字符串表示默认的 chatgpt.com 逆向路径。
const (
	ProviderChatGPT = ""         // 默认:chatgpt.com 逆向
	ProviderMiniMax = "minimax"  // MiniMax 官方 API
)

// Model 对应 models 表。
type Model struct {
	ID                    uint64       `db:"id" json:"id"`
	Slug                  string       `db:"slug" json:"slug"`
	Type                  string       `db:"type" json:"type"`
	Provider              string       `db:"provider" json:"provider"`
	UpstreamModelSlug     string       `db:"upstream_model_slug" json:"upstream_model_slug"`
	InputPricePer1M       int64        `db:"input_price_per_1m" json:"input_price_per_1m"`
	OutputPricePer1M      int64        `db:"output_price_per_1m" json:"output_price_per_1m"`
	CacheReadPricePer1M   int64        `db:"cache_read_price_per_1m" json:"cache_read_price_per_1m"`
	ImagePricePerCall     int64        `db:"image_price_per_call" json:"image_price_per_call"`
	Description           string       `db:"description" json:"description"`
	Enabled               bool         `db:"enabled" json:"enabled"`
	CreatedAt             time.Time    `db:"created_at" json:"created_at"`
	UpdatedAt             time.Time    `db:"updated_at" json:"updated_at"`
	DeletedAt             sql.NullTime `db:"deleted_at" json:"-"`
}
