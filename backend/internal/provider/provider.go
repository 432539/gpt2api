// Package provider 第三方生成提供方抽象（GPT 生图 / GROK 生视频 等）。
//
// 真正的协议适配在子包内（gpt / grok / mock），调度器只依赖接口。
package provider

import (
	"context"
	"time"

	"github.com/kleinai/backend/internal/model"
)

// Kind 生成类型。
type Kind string

const (
	KindChat  Kind = "chat"
	KindImage Kind = "image"
	KindVideo Kind = "video"
)

// Mode 生成模式。
type Mode string

const (
	ModeT2I Mode = "t2i"
	ModeI2I Mode = "i2i"
	ModeT2V Mode = "t2v"
	ModeI2V Mode = "i2v"
)

// Request 通用生成请求。
type Request struct {
	TaskID    string
	Kind      Kind
	Mode      Mode
	ModelCode string
	Prompt    string
	NegPrompt string
	Params    map[string]any
	RefAssets []string
	Count     int
	Account   *model.Account
	// Credential 是 Account.CredentialEnc 解密后的明文（API Key / Cookie / OAuth Token）。
	// 调用方负责解密，provider 不再持有 AESGCM。
	Credential string
	// BaseURL 优先级：account.base_url > provider 默认。
	BaseURL  string
	ProxyURL string
	// UpstreamLog records provider stage diagnostics for admin troubleshooting.
	UpstreamLog UpstreamLogger
}

type UpstreamLogEntry struct {
	Provider        string
	Stage           string
	Method          string
	URL             string
	StatusCode      int
	DurationMs      int64
	RequestExcerpt  string
	ResponseExcerpt string
	Error           string
	Meta            map[string]any
}

type UpstreamLogger func(ctx context.Context, entry UpstreamLogEntry)

// Asset 单个生成资产（一张图 / 一段视频）。
type Asset struct {
	URL        string
	ThumbURL   string
	Width      int
	Height     int
	DurationMs int
	SizeBytes  int64
	Mime       string
	Meta       map[string]any
}

// Result 通用生成结果。
type Result struct {
	TaskID  string
	Assets  []Asset
	Latency time.Duration
}

// Provider 提供方接口。
type Provider interface {
	// Name 返回 provider 标识，例如 "gpt" / "grok"。
	Name() string
	// Generate 同步发起一次生成（worker 内部使用）。
	Generate(ctx context.Context, req *Request) (*Result, error)
}
