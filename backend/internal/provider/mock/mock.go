// Package mock 提供方 stub：直接返回示意性 URL，不发起真实第三方请求。
//
// 用途：
//   1. 本地开发未配真实账号时打通整链路；
//   2. 测试 / CI；
//   3. 演示。
package mock

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kleinai/backend/internal/provider"
)

// New 构造。kind 标识自身角色（gpt / grok）。
func New(name string) *Provider {
	return &Provider{name: name}
}

// Provider 实现 provider.Provider。
type Provider struct {
	name string
}

// Name impl。
func (p *Provider) Name() string { return p.name }

// Generate 模拟生成：sleep 一会儿，返回若干虚拟 URL。
func (p *Provider) Generate(ctx context.Context, req *provider.Request) (*provider.Result, error) {
	delay := 500 * time.Millisecond
	if req.Kind == provider.KindVideo {
		delay = 2 * time.Second
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(delay):
	}

	count := req.Count
	if count <= 0 {
		count = 1
	}
	assets := make([]provider.Asset, 0, count)
	for i := 0; i < count; i++ {
		switch req.Kind {
		case provider.KindImage:
			assets = append(assets, provider.Asset{
				URL:    mockImageURL(req.TaskID, i),
				Width:  1024,
				Height: 1024,
				Mime:   "image/png",
			})
		case provider.KindVideo:
			assets = append(assets, provider.Asset{
				URL:        mockVideoURL(req.TaskID, i),
				DurationMs: 4000,
				Width:      1280,
				Height:     720,
				Mime:       "video/mp4",
			})
		}
	}

	return &provider.Result{
		TaskID:  req.TaskID,
		Assets:  assets,
		Latency: delay,
	}, nil
}

func mockImageURL(taskID string, seq int) string {
	id := strings.ToLower(taskID)
	if id == "" {
		id = "demo"
	}
	return fmt.Sprintf("https://picsum.photos/seed/%s-%d/1024/1024", id, seq)
}

func mockVideoURL(taskID string, seq int) string {
	id := strings.ToLower(taskID)
	if id == "" {
		id = "demo"
	}
	return fmt.Sprintf("https://kleinai.dev/mock/video/%s-%d.mp4", id, seq)
}
