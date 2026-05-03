// Package gpt 实现 OpenAI 兼容的图像生成 provider（GPT 账号池 → /v1/images/generations）。
//
// 协议：完全对齐 OpenAI Images API，可对接 OpenAI 官方 / Azure / 任意网关。
//   POST {base_url}/v1/images/generations
//   Header: Authorization: Bearer {api_key}
//   Body  : {"model","prompt","n","size","response_format"}
//   Resp  : {"created":int,"data":[{"url":"..."} | {"b64_json":"..."}]}
//
// 错误处理：
//   - 4xx 标记账号失败并 30s 冷却（避免雪崩）；
//   - 5xx 标记账号失败并 5min 冷却；
//   - 超时同上。
package gpt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/kleinai/backend/internal/provider"
)

const (
	defaultBaseURL = "https://api.openai.com"
	defaultTimeout = 90 * time.Second
)

// Provider 实现 provider.Provider。
type Provider struct {
	client     *http.Client
	defaultURL string
	name       string
}

// New 构造。defaultBase 为空时使用 OpenAI 官方域名。
func New(defaultBase string) *Provider {
	if defaultBase == "" {
		defaultBase = defaultBaseURL
	}
	return &Provider{
		client: &http.Client{
			Timeout: defaultTimeout,
		},
		defaultURL: strings.TrimRight(defaultBase, "/"),
		name:       "gpt",
	}
}

// Name impl。
func (p *Provider) Name() string { return p.name }

type imgReq struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	N              int    `json:"n,omitempty"`
	Size           string `json:"size,omitempty"`
	Quality        string `json:"quality,omitempty"`
	Style          string `json:"style,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
}

type imgRespItem struct {
	URL     string `json:"url"`
	B64JSON string `json:"b64_json,omitempty"`
}
type imgResp struct {
	Created int           `json:"created"`
	Data    []imgRespItem `json:"data"`
	Error   *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// Generate impl。仅支持 KindImage。
func (p *Provider) Generate(ctx context.Context, req *provider.Request) (*provider.Result, error) {
	if req.Kind != provider.KindImage {
		return nil, fmt.Errorf("gpt provider only supports image kind, got %s", req.Kind)
	}
	if req.Credential == "" {
		return nil, fmt.Errorf("gpt provider missing credential")
	}

	base := req.BaseURL
	if base == "" {
		base = p.defaultURL
	}
	base = strings.TrimRight(base, "/")
	url := base + "/v1/images/generations"

	count := req.Count
	if count <= 0 {
		count = 1
	}

	body := imgReq{
		Model:          req.ModelCode,
		Prompt:         req.Prompt,
		N:              count,
		Size:           strParam(req.Params, "size", "1024x1024"),
		Quality:        strParam(req.Params, "quality", ""),
		Style:          strParam(req.Params, "style", ""),
		ResponseFormat: "url",
	}
	payload, _ := json.Marshal(body)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+req.Credential)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "kleinai/1.0")

	start := time.Now()
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gpt http: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("gpt %d: %s", resp.StatusCode, snippet(raw, 240))
	}

	var out imgResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("gpt decode: %w (raw=%s)", err, snippet(raw, 240))
	}
	if out.Error != nil && out.Error.Message != "" {
		return nil, fmt.Errorf("gpt: %s", out.Error.Message)
	}
	if len(out.Data) == 0 {
		return nil, fmt.Errorf("gpt returned 0 image")
	}

	width, height := parseSize(body.Size)
	assets := make([]provider.Asset, 0, len(out.Data))
	for _, d := range out.Data {
		a := provider.Asset{
			URL:    d.URL,
			Width:  width,
			Height: height,
			Mime:   "image/png",
		}
		if a.URL == "" && d.B64JSON != "" {
			// 大多数网关会直接给 URL；b64 模式 caller 应自行落 OSS 后再回填。
			a.URL = "data:image/png;base64," + d.B64JSON
		}
		assets = append(assets, a)
	}

	return &provider.Result{
		TaskID:  req.TaskID,
		Assets:  assets,
		Latency: time.Since(start),
	}, nil
}

// === helpers ===

func strParam(p map[string]any, key, def string) string {
	if p == nil {
		return def
	}
	if v, ok := p[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return def
}

func parseSize(size string) (int, int) {
	if size == "" {
		return 1024, 1024
	}
	parts := strings.SplitN(size, "x", 2)
	if len(parts) != 2 {
		return 1024, 1024
	}
	var w, h int
	fmt.Sscanf(parts[0], "%d", &w)
	fmt.Sscanf(parts[1], "%d", &h)
	if w <= 0 {
		w = 1024
	}
	if h <= 0 {
		h = 1024
	}
	return w, h
}

func snippet(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "...(truncated)"
}
