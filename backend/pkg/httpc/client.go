// Package httpc 是基于 resty 的统一 HTTP 客户端，所有外部 Provider 调用都必须经此包。
package httpc

import (
	"time"

	"github.com/go-resty/resty/v2"
)

// Options 客户端选项。
type Options struct {
	BaseURL string
	Timeout time.Duration
	Retry   int
	Headers map[string]string
}

// New 创建 resty 客户端。
func New(opt Options) *resty.Client {
	cli := resty.New()
	if opt.BaseURL != "" {
		cli.SetBaseURL(opt.BaseURL)
	}
	if opt.Timeout <= 0 {
		opt.Timeout = 60 * time.Second
	}
	cli.SetTimeout(opt.Timeout)
	if opt.Retry > 0 {
		cli.SetRetryCount(opt.Retry)
		cli.SetRetryWaitTime(500 * time.Millisecond)
		cli.SetRetryMaxWaitTime(5 * time.Second)
	}
	for k, v := range opt.Headers {
		cli.SetHeader(k, v)
	}
	cli.SetHeader("User-Agent", "KleinAI-Backend/1.0")
	return cli
}
