// Package proxyx 提供出站代理解析与 *http.Transport / *http.Client 构造。
//
// 设计原则（参考 sub2api）：
// - 协议白名单：http/https/socks5/socks5h；
// - 代理无效时返回 error，不回退直连（防止意外走真实出口 IP）；
// - 同一个进程内可多次调用，每次都会拷贝默认 Transport 字段，避免污染 http.DefaultTransport。
package proxyx

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	xproxy "golang.org/x/net/proxy"
)

// ErrInvalidProxy 代理 URL 无效或不支持。
var ErrInvalidProxy = errors.New("invalid proxy")

// Parse 校验代理 URL。空字符串视为不使用代理（返回 nil, nil）。
//
// 规则：
// - 必须有 scheme：http / https / socks5 / socks5h；
// - "socks5" 自动改写为 "socks5h"，让 DNS 在远端解析；
// - 必须有 host:port。
func Parse(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidProxy, err)
	}
	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "http", "https":
	case "socks5":
		u.Scheme = "socks5h"
	case "socks5h":
	default:
		return nil, fmt.Errorf("%w: unsupported scheme %q", ErrInvalidProxy, scheme)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("%w: missing host", ErrInvalidProxy)
	}
	return u, nil
}

// BuildTransport 基于代理 URL 构造一个新的 *http.Transport。proxyURL 为 nil 表示直连。
func BuildTransport(proxyURL *url.URL) (*http.Transport, error) {
	tr := &http.Transport{
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	if proxyURL == nil {
		tr.DialContext = (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext
		return tr, nil
	}
	scheme := strings.ToLower(proxyURL.Scheme)
	switch scheme {
	case "http", "https":
		// 通过 HTTP/HTTPS CONNECT 走代理。
		tr.Proxy = http.ProxyURL(proxyURL)
		tr.DialContext = (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	case "socks5", "socks5h":
		var auth *xproxy.Auth
		if proxyURL.User != nil {
			pw, _ := proxyURL.User.Password()
			auth = &xproxy.Auth{User: proxyURL.User.Username(), Password: pw}
		}
		dialer, err := xproxy.SOCKS5("tcp", proxyURL.Host, auth, &net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: 30 * time.Second,
		})
		if err != nil {
			return nil, fmt.Errorf("build socks5 dialer: %w", err)
		}
		ctxDialer, ok := dialer.(xproxy.ContextDialer)
		if !ok {
			return nil, fmt.Errorf("socks5 dialer does not support context")
		}
		tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return ctxDialer.DialContext(ctx, network, addr)
		}
	default:
		return nil, fmt.Errorf("%w: unsupported scheme %q", ErrInvalidProxy, scheme)
	}
	return tr, nil
}

// BuildClient 便捷方法：构造带代理的 http.Client，timeout 为请求超时。
func BuildClient(proxyURL *url.URL, timeout time.Duration) (*http.Client, error) {
	tr, err := BuildTransport(proxyURL)
	if err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &http.Client{Transport: tr, Timeout: timeout}, nil
}
