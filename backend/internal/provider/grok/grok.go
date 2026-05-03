// Package grok 实现 GROK 风格的视频生成 provider。
//
// GROK 公开 API 仍在演进中，本 provider 采用一个通用的"异步任务 + 轮询"协议，
// 你可以把 base_url 指到任意兼容网关（kleinai-gateway / FAL / Runway 风格）：
//
//	POST {base_url}/v1/videos/generations
//	     Authorization: Bearer {api_key}
//	     Body: {"model","prompt","duration","aspect_ratio","ref_images":[]}
//	     Resp 200 either:
//	       A. {"task_id":"abc","status":"queued"}            // 异步
//	       B. {"data":[{"url":"https://..."}], "duration_ms":... } // 同步直返
//
//	GET {base_url}/v1/videos/tasks/{task_id}
//	     Resp: {"task_id","status":"queued|running|succeeded|failed",
//	            "data":[{"url":"...","duration_ms":...}], "error":""}
//
// 调度器内置超时：默认 12min，单次轮询间隔 3s（指数 backoff 上限 10s）。
package grok

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
	defaultBaseURL    = webBaseURL
	httpTimeout       = 30 * time.Second
	pollMaxDur        = 12 * time.Minute
	pollInitialPeriod = 3 * time.Second
	pollMaxPeriod     = 10 * time.Second
)

// Provider 实现 provider.Provider。
type Provider struct {
	client     *http.Client
	defaultURL string
	name       string
	web        *WebClient
}

// New 构造。
func New(defaultBase string) *Provider {
	if defaultBase == "" {
		defaultBase = defaultBaseURL
	}
	return &Provider{
		client: &http.Client{
			Timeout: httpTimeout,
		},
		defaultURL: strings.TrimRight(defaultBase, "/"),
		name:       "grok",
		web:        NewWebClient(defaultBase),
	}
}

// Name impl。
func (p *Provider) Name() string { return p.name }

type vidCreateReq struct {
	Model       string   `json:"model"`
	Prompt      string   `json:"prompt"`
	NegPrompt   string   `json:"negative_prompt,omitempty"`
	Duration    int      `json:"duration,omitempty"`
	AspectRatio string   `json:"aspect_ratio,omitempty"`
	N           int      `json:"n,omitempty"`
	RefImages   []string `json:"ref_images,omitempty"`
}

type vidAsset struct {
	URL        string `json:"url"`
	ThumbURL   string `json:"thumb_url,omitempty"`
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
	DurationMs int    `json:"duration_ms,omitempty"`
	Mime       string `json:"mime,omitempty"`
}

type vidResp struct {
	TaskID string     `json:"task_id,omitempty"`
	Status string     `json:"status,omitempty"`
	Data   []vidAsset `json:"data,omitempty"`
	Error  string     `json:"error,omitempty"`
}

// Generate 视频生成；自动识别同步 / 异步响应。
func (p *Provider) Generate(ctx context.Context, req *provider.Request) (*provider.Result, error) {
	if req.Kind != provider.KindVideo {
		return nil, fmt.Errorf("grok provider only supports video kind, got %s", req.Kind)
	}
	if req.Credential == "" {
		return nil, fmt.Errorf("grok provider missing credential")
	}
	base := req.BaseURL
	if base == "" {
		base = p.defaultURL
	}
	if base == "" || strings.Contains(base, "grok.com") || strings.Contains(base, "api.x.ai") {
		web := p.web
		if req.ProxyURL != "" || req.BaseURL != "" {
			web = NewWebClientWithProxy(req.BaseURL, req.ProxyURL)
		}
		web = web.WithUpstreamLogger(req.UpstreamLog)
		count := req.Count
		if count <= 0 {
			count = 1
		}
		assets := make([]provider.Asset, 0, count)
		for i := 0; i < count; i++ {
			items, err := web.GenerateVideo(ctx, req.Credential, VideoRequest{
				ModelCode:   NormalizeVideoModel(req.ModelCode),
				Prompt:      req.Prompt,
				Refs:        req.RefAssets,
				DurationSec: intParam(req.Params, "duration", 6),
				Size:        strParam(req.Params, "size", ""),
				AspectRatio: strParam(req.Params, "aspect_ratio", ""),
				Quality:     strParam(req.Params, "quality", ""),
				Count:       1,
			})
			if err != nil {
				return nil, err
			}
			for _, it := range items {
				assets = append(assets, provider.Asset{
					URL:        it.URL,
					ThumbURL:   it.ThumbURL,
					Width:      it.Width,
					Height:     it.Height,
					DurationMs: it.DurationMs,
					Mime:       "video/mp4",
				})
			}
		}
		return &provider.Result{TaskID: req.TaskID, Assets: assets}, nil
	}

	base = strings.TrimRight(base, "/")

	count := req.Count
	if count <= 0 {
		count = 1
	}
	dur := normalizeVideoDuration(intParam(req.Params, "duration", 6))
	aspect := strParam(req.Params, "aspect_ratio", "16:9")

	body := vidCreateReq{
		Model:       req.ModelCode,
		Prompt:      req.Prompt,
		NegPrompt:   req.NegPrompt,
		Duration:    dur,
		AspectRatio: aspect,
		N:           count,
		RefImages:   req.RefAssets,
	}
	payload, _ := json.Marshal(body)

	start := time.Now()
	createResp, err := p.do(ctx, http.MethodPost, base+"/v1/videos/generations", payload, req.Credential)
	if err != nil {
		return nil, err
	}

	// 同步直返
	if len(createResp.Data) > 0 {
		return &provider.Result{
			TaskID:  req.TaskID,
			Assets:  toAssets(createResp.Data, dur),
			Latency: time.Since(start),
		}, nil
	}
	if createResp.TaskID == "" {
		return nil, fmt.Errorf("grok empty task_id and empty data")
	}

	// 异步：内部轮询
	period := pollInitialPeriod
	deadline := time.Now().Add(pollMaxDur)
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(period):
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("grok task %s timeout", createResp.TaskID)
		}
		st, err := p.do(ctx, http.MethodGet, base+"/v1/videos/tasks/"+createResp.TaskID, nil, req.Credential)
		if err != nil {
			return nil, err
		}
		switch strings.ToLower(st.Status) {
		case "succeeded", "success", "completed", "done":
			if len(st.Data) == 0 {
				return nil, fmt.Errorf("grok task %s succeeded but empty data", createResp.TaskID)
			}
			return &provider.Result{
				TaskID:  req.TaskID,
				Assets:  toAssets(st.Data, dur),
				Latency: time.Since(start),
			}, nil
		case "failed", "error", "cancelled":
			msg := st.Error
			if msg == "" {
				msg = "grok task failed"
			}
			return nil, fmt.Errorf("grok task %s: %s", createResp.TaskID, msg)
		}
		// queued / running / processing → 继续轮询
		period *= 2
		if period > pollMaxPeriod {
			period = pollMaxPeriod
		}
	}
}

func (p *Provider) do(ctx context.Context, method, url string, payload []byte, key string) (*vidResp, error) {
	var rdr io.Reader
	if payload != nil {
		rdr = bytes.NewReader(payload)
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, url, rdr)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+key)
	if payload != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	httpReq.Header.Set("User-Agent", "kleinai/1.0")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("grok http: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("grok %d: %s", resp.StatusCode, snippet(raw, 240))
	}
	var out vidResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("grok decode: %w (raw=%s)", err, snippet(raw, 240))
	}
	return &out, nil
}

func toAssets(items []vidAsset, durSec int) []provider.Asset {
	out := make([]provider.Asset, 0, len(items))
	for _, it := range items {
		a := provider.Asset{
			URL:        it.URL,
			ThumbURL:   it.ThumbURL,
			Width:      it.Width,
			Height:     it.Height,
			DurationMs: it.DurationMs,
			Mime:       it.Mime,
		}
		if a.DurationMs == 0 && durSec > 0 {
			a.DurationMs = durSec * 1000
		}
		if a.Mime == "" {
			a.Mime = "video/mp4"
		}
		if a.Width == 0 || a.Height == 0 {
			a.Width, a.Height = 1280, 720
		}
		out = append(out, a)
	}
	return out
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

func intParam(p map[string]any, key string, def int) int {
	if p == nil {
		return def
	}
	if v, ok := p[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		case int64:
			return int(n)
		}
	}
	return def
}

func snippet(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "...(truncated)"
}
