// Package chatgpt 封装 chatgpt.com 的反向工程调用。
//
// 本包不关心调度与计费,只负责一次 HTTP 往返。
// 调用方(网关)负责:调度器拿 Lease -> 构造 Client -> 发请求 -> 转译响应。
package chatgpt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/net/publicsuffix"

	"github.com/432539/gpt2api/pkg/logger"
)

func loggerL() *zap.Logger { return logger.L() }

// 固定请求头(模拟 Chrome 124;客户端版本号可按需更新)。
const (
	// UA 对齐 HAR 抓包(Edge 143 / Chromium 143 on Windows 11);必须与
	// commonHeaders 里的 Sec-Ch-Ua-* 完整套件自洽,否则上游认为指纹冲突。
	DefaultUserAgent      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36 Edg/143.0.0.0"
	DefaultClientVersion  = "prod-be885abbfcfe7b1f511e88b3003d9ee44757fbad"
	DefaultClientBuildNum = "5955942"
	DefaultLanguage       = "zh-CN"
	BaseURL               = "https://chatgpt.com"
)

// Options 构造 Client 的参数。
type Options struct {
	BaseURL       string
	AuthToken     string // 完整 Bearer token(已解密)
	DeviceID      string
	SessionID     string
	ProxyURL      string        // http(s)://user:pass@host:port,为空则直连
	Timeout       time.Duration // HTTP 总超时,默认 120s
	SSETimeout    time.Duration // SSE 首 byte 超时,默认 60s
	Cookies       string        // JSON 字符串(可选),格式 [{"name":"x","value":"y","domain":".chatgpt.com"}]
	UserAgent     string
	ClientVersion string
	Language      string

	// TurnstileSolver 可选。为 nil 时 ChatRequirementsV2 会回退到单步
	// chat-requirements 流程(TurnstileRequired=true 时直接忽略)。
	TurnstileSolver TurnstileSolver
}

// Client 一个账号/代理/device 一次请求的上游客户端。可多次复用(建议 1 次请求 1 个)。
type Client struct {
	opts Options
	hc   *http.Client
}

// New 构造客户端。
func New(opt Options) (*Client, error) {
	if opt.AuthToken == "" {
		return nil, errors.New("auth_token required")
	}
	if opt.DeviceID == "" {
		return nil, errors.New("device_id required")
	}
	if opt.BaseURL == "" {
		opt.BaseURL = BaseURL
	}
	if opt.Timeout == 0 {
		opt.Timeout = 120 * time.Second
	}
	if opt.SSETimeout == 0 {
		opt.SSETimeout = 60 * time.Second
	}
	if opt.UserAgent == "" {
		opt.UserAgent = DefaultUserAgent
	}
	if opt.ClientVersion == "" {
		opt.ClientVersion = DefaultClientVersion
	}
	if opt.Language == "" {
		opt.Language = DefaultLanguage
	}

	// 直接用标准 net/http 会被 Cloudflare 按 JA3/JA4 指纹识别出不是浏览器(403 拦截页);
	// 这里换成 utls-based RoundTripper,ClientHello 伪装成 Chrome 120。
	// Proxy(HTTP / HTTPS CONNECT)在 transport 内部处理,不再走 http.ProxyURL。
	tr, err := NewUTLSTransport(opt.ProxyURL, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("init utls transport: %w", err)
	}

	jar, _ := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	hc := &http.Client{
		Transport: tr,
		Timeout:   opt.Timeout, // SSE 场景会关闭该 timeout(见 StreamConversation)
		Jar:       jar,
	}
	if opt.Cookies != "" {
		_ = loadCookies(jar, opt.BaseURL, opt.Cookies)
	}
	return &Client{opts: opt, hc: hc}, nil
}

// loadCookies 把 JSON cookies 加载到 jar。
func loadCookies(jar http.CookieJar, base, raw string) error {
	var list []struct {
		Name   string `json:"name"`
		Value  string `json:"value"`
		Domain string `json:"domain"`
		Path   string `json:"path"`
	}
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return err
	}
	u, err := url.Parse(base)
	if err != nil {
		return err
	}
	cs := make([]*http.Cookie, 0, len(list))
	for _, c := range list {
		if c.Name == "" || c.Value == "" {
			continue
		}
		path := c.Path
		if path == "" {
			path = "/"
		}
		cs = append(cs, &http.Cookie{Name: c.Name, Value: c.Value, Domain: c.Domain, Path: path})
	}
	jar.SetCookies(u, cs)
	return nil
}

// commonHeaders 设置所有 chatgpt.com 请求通用的头。
//
// 对齐真实浏览器(Edge 143 @ Windows)抓包:除了 PoW/turnstile 这类 sentinel 头
// 由具体端点自己加,其他客户端指纹头、Oai-* 头、sec-ch-ua 完整套件、
// X-Openai-Target-Path/Route 都在这里统一设置。X-Oai-Turn-Trace-Id 是"每 turn 一个"
// 的 UUID,由具体发送函数(StreamFChat / StreamFConversation)自己随机生成,
// 这里只填固定的。
//
// 真正的指纹差异在 HTTP/2 SETTINGS frame(JA4H),已在 utls_transport.go 中用
// forceH1 强制走 http/1.1 规避。
func (c *Client) commonHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.opts.AuthToken)
	req.Header.Set("User-Agent", c.opts.UserAgent)
	req.Header.Set("Origin", c.opts.BaseURL)
	req.Header.Set("Referer", c.opts.BaseURL+"/")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8,en-GB;q=0.7,en-US;q=0.6")
	// 不设置 Accept-Encoding:Go net/http 会自动加 `Accept-Encoding: gzip` 并透明解压。
	// 若主动声明 br/zstd,Go 不会解压,body 会是压缩字节,SSE / JSON 解析全坏。
	// sec-ch-ua 完整套件(Edge 143 on Windows 11):真实浏览器每次都会带这整套,
	// 少其中任何一项都可能触发 bot 指纹识别。保持与 DefaultUserAgent 对齐。
	req.Header.Set("Sec-Ch-Ua", `"Microsoft Edge";v="143", "Chromium";v="143", "Not A(Brand";v="24"`)
	req.Header.Set("Sec-Ch-Ua-Arch", `"x86"`)
	req.Header.Set("Sec-Ch-Ua-Bitness", `"64"`)
	req.Header.Set("Sec-Ch-Ua-Full-Version", `"143.0.3650.96"`)
	req.Header.Set("Sec-Ch-Ua-Full-Version-List",
		`"Microsoft Edge";v="143.0.3650.96", "Chromium";v="143.0.7499.147", "Not A(Brand";v="24.0.0.0"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Model", `""`)
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	req.Header.Set("Sec-Ch-Ua-Platform-Version", `"19.0.0"`)
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Priority", "u=1, i")
	// Oai-* 头:真实浏览器每请求必带。
	req.Header.Set("Oai-Device-Id", c.opts.DeviceID)
	if c.opts.SessionID != "" {
		req.Header.Set("Oai-Session-Id", c.opts.SessionID)
	}
	req.Header.Set("Oai-Language", c.opts.Language)
	req.Header.Set("Oai-Client-Version", c.opts.ClientVersion)
	req.Header.Set("Oai-Client-Build-Number", DefaultClientBuildNum)
	// X-Openai-Target-Path / Route:真实浏览器每请求必带,值就是请求 URL 的 path
	// (不含 query)。Route 通常是带占位符的形式(例如 /files/download/{file_id}),
	// 但后端对两个字段都是相等比较,填成 Path 也不触发 400;先统一 Path,以后
	// 发现特定端点报错再单独覆盖。
	if p := req.URL.Path; p != "" {
		req.Header.Set("X-Openai-Target-Path", p)
		req.Header.Set("X-Openai-Target-Route", p)
	}
	// Accept 默认值在各 endpoint 函数里会覆盖(比如 SSE 设成 text/event-stream)。
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "*/*")
	}
}

// UpstreamError 是一次 chatgpt.com 请求失败的结构化错误。
type UpstreamError struct {
	Status  int
	Message string
	Body    string
	Header  http.Header
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("chatgpt upstream %d: %s", e.Status, e.Message)
}

// ConversationID 尽量从上游错误响应里取出已创建的 conversation_id。
// 某些 f/conversation 响应会返回 skipped_mainline,但图片工具链路仍在会话里继续跑。
func (e *UpstreamError) ConversationID() string {
	if e == nil {
		return ""
	}
	for _, key := range []string{
		"Openai-Conversation-Id",
		"Oai-Conversation-Id",
		"X-Openai-Conversation-Id",
		"X-Oai-Conversation-Id",
		"X-Conversation-Id",
		"Conversation-Id",
	} {
		if v := strings.TrimSpace(e.Header.Get(key)); v != "" {
			return v
		}
	}
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(e.Body)), &obj); err != nil {
		return ""
	}
	for _, key := range []string{"conversation_id", "conversationId", "conversationID"} {
		if v, _ := obj[key].(string); strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// IsRateLimited 对应 HTTP 429 / 资源耗尽。
func (e *UpstreamError) IsRateLimited() bool { return e.Status == 429 }

// IsUnauthorized 对应 401 / 403(token 失效 / 风控封号)。
func (e *UpstreamError) IsUnauthorized() bool { return e.Status == 401 || e.Status == 403 }

// ChatRequirementsResp 对应响应(仅摘取关键字段)。
type ChatRequirementsResp struct {
	Token string `json:"token"` // chat_token
	// Persona 常见取值:"chatgpt-freeaccount"(免费号)/ "chatgpt-paid"(Plus/Team)
	//              / "chatgpt-noauth"(未登录)
	// 免费号对高级模型(gpt-5 等)会静默不生成,必须把 upstream model 退化到 "auto"
	// 由上游自己挑,否则 SSE 只会拿到一条 hidden system preamble 就结束。
	Persona     string `json:"persona"`
	Proofofwork struct {
		Required   bool   `json:"required"`
		Seed       string `json:"seed"`
		Difficulty string `json:"difficulty"`
	} `json:"proofofwork"`
	Turnstile struct {
		Required bool `json:"required"`
	} `json:"turnstile"`
}

// IsFreeAccount 判断当前账号是否为免费号(persona=chatgpt-freeaccount)。
func (r *ChatRequirementsResp) IsFreeAccount() bool {
	return r.Persona == "chatgpt-freeaccount"
}

// SolveProof 求解 POW,返回要放进 `Openai-Sentinel-Proof-Token` 的字符串。
// 若 Proofofwork.Required=false,返回空串。
func (r *ChatRequirementsResp) SolveProof(userAgent string) string {
	if !r.Proofofwork.Required {
		return ""
	}
	return SolveProofToken(r.Proofofwork.Seed, r.Proofofwork.Difficulty, userAgent)
}

// Bootstrap 模拟浏览器首次打开 chatgpt.com 的 GET /,让 cookie jar 收下
// Cloudflare 的 `__cf_bm` / `_cfuvid` 与 OpenAI 的 `oai-did` 等 cookie。
//
// 关键作用:没有这些 cookie 时,chat-requirements 会直接要求 Turnstile(即使
// Bearer 合法),所以建议在每次新建 Client 后先调一次 Bootstrap,或者在
// ChatRequirements 内部第一次请求前自动调一次。
// 多次调用是幂等的(HTTP 200/3xx 均视为成功)。
func (c *Client) Bootstrap(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.opts.BaseURL+"/", nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.opts.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8,en-GB;q=0.7,en-US;q=0.6")
	req.Header.Set("Sec-Ch-Ua", `"Microsoft Edge";v="143", "Chromium";v="143", "Not A(Brand";v="24"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	res, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("bootstrap GET /: %w", err)
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, res.Body)
	if res.StatusCode >= 500 {
		return &UpstreamError{Status: res.StatusCode, Message: "bootstrap failed"}
	}
	return nil
}

// ChatRequirements 取 chat_token。
// 请求体必须带 `p` = 客户端预算的 requirements_token(前缀 gAAAAAC,固定难度 0fffff)。
// 否则上游会返回空 token 或 403。
func (c *Client) ChatRequirements(ctx context.Context) (*ChatRequirementsResp, error) {
	// 首次请求前顺手做一次浏览器首访,拿 __cf_bm / oai-did,避免 Turnstile。
	// jar 已经持有过则这次 GET 其实是 200,代价就是一个 RTT,可以接受。
	if err := c.Bootstrap(ctx); err != nil {
		// 拿不到 cookie 不致命,继续往下走;真不行再让 chat-requirements 自己报错。
		_ = err
	}
	reqToken := NewPOWConfig(c.opts.UserAgent).RequirementsToken()
	body, _ := json.Marshal(map[string]string{"p": reqToken})
	req, err := http.NewRequestWithContext(ctx,
		http.MethodPost,
		c.opts.BaseURL+"/backend-api/sentinel/chat-requirements",
		strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	c.commonHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	res, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	buf, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		return nil, &UpstreamError{Status: res.StatusCode, Message: "chat-requirements failed", Body: string(buf)}
	}
	var out ChatRequirementsResp
	if err := json.Unmarshal(buf, &out); err != nil {
		return nil, fmt.Errorf("decode chat-requirements: %w", err)
	}
	// 诊断用:打印完整 body(含 turnstile / proofofwork / arkose 字段)。
	// 稳定后可改成 Debug 或删除。
	if logger := loggerL(); logger != nil {
		bodyStr := string(buf)
		if len(bodyStr) > 800 {
			bodyStr = bodyStr[:800] + "..."
		}
		logger.Info("chat-requirements raw body",
			zap.String("body", bodyStr),
			zap.Bool("turnstile_required", out.Turnstile.Required),
			zap.Bool("pow_required", out.Proofofwork.Required),
			zap.String("token_prefix", truncatePrefix(out.Token, 16)))
	}
	return &out, nil
}

func truncatePrefix(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ChatRequirementsPrepareResp 是 /sentinel/chat-requirements/prepare 的响应。
//
// 浏览器在每个 turn 前先调 prepare 拿到 challenge(turnstile.dx + pow.seed/difficulty),
// 本地计算后再调 finalize 拿最终 chat-requirements-token。我们 Go 端没法解
// turnstile(Cloudflare 混淆 JS + WASM),所以 solver 未配置时需要回退到老的
// 单步 /sentinel/chat-requirements 端点(见 ChatRequirementsV2)。
type ChatRequirementsPrepareResp struct {
	Persona      string `json:"persona"`
	PrepareToken string `json:"prepare_token"`
	Turnstile    struct {
		Required bool   `json:"required"`
		DX       string `json:"dx"`
	} `json:"turnstile"`
	Proofofwork struct {
		Required   bool   `json:"required"`
		Seed       string `json:"seed"`
		Difficulty string `json:"difficulty"`
	} `json:"proofofwork"`
}

// ChatRequirementsPrepare 调 /backend-api/sentinel/chat-requirements/prepare。
// 请求体里的 `p` 字段仍是 18 元素 PoW(前缀 gAAAAAC),和老的单步接口同款。
func (c *Client) ChatRequirementsPrepare(ctx context.Context) (*ChatRequirementsPrepareResp, error) {
	reqToken := NewPOWConfig(c.opts.UserAgent).RequirementsToken()
	body, _ := json.Marshal(map[string]string{"p": reqToken})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.opts.BaseURL+"/backend-api/sentinel/chat-requirements/prepare",
		strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	c.commonHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	res, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	buf, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		return nil, &UpstreamError{Status: res.StatusCode, Message: "chat-requirements/prepare failed", Body: string(buf)}
	}
	var out ChatRequirementsPrepareResp
	if err := json.Unmarshal(buf, &out); err != nil {
		return nil, fmt.Errorf("decode chat-requirements/prepare: %w", err)
	}
	return &out, nil
}

// ChatRequirementsFinalize 调 /backend-api/sentinel/chat-requirements/finalize。
// 入参:prepare_token(来自 Prepare),proofofwork(本地解,13 元素),
// turnstileResp(通常由 TurnstileSolver 提供;没有则传空串,上游可能拒绝)。
// 返回最终的 chat-requirements-token。
func (c *Client) ChatRequirementsFinalize(
	ctx context.Context,
	prepareToken, proofofwork, turnstileResp string,
) (string, string, error) {
	payload := map[string]interface{}{
		"prepare_token": prepareToken,
	}
	if proofofwork != "" {
		payload["proofofwork"] = proofofwork
	}
	if turnstileResp != "" {
		payload["turnstile"] = turnstileResp
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.opts.BaseURL+"/backend-api/sentinel/chat-requirements/finalize",
		strings.NewReader(string(body)))
	if err != nil {
		return "", "", err
	}
	c.commonHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	res, err := c.hc.Do(req)
	if err != nil {
		return "", "", err
	}
	defer res.Body.Close()
	buf, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		return "", "", &UpstreamError{Status: res.StatusCode, Message: "chat-requirements/finalize failed", Body: string(buf)}
	}
	var out struct {
		Persona string `json:"persona"`
		Token   string `json:"token"`
	}
	if err := json.Unmarshal(buf, &out); err != nil {
		return "", "", fmt.Errorf("decode chat-requirements/finalize: %w", err)
	}
	return out.Token, out.Persona, nil
}

// ChatRequirementsV2 是 sentinel 协议的新统一入口。
//
// 路由逻辑:
//  1. 先调 /prepare 拿 challenge;
//  2. 若返回 turnstile.required=false,直接把 prepare_token 走 finalize 拿最终 token;
//  3. 若 turnstile.required=true:
//     a. 当 opts.TurnstileSolver != nil → solver.Solve(dx),然后走 finalize;
//     b. 当 solver 为 nil → **回退到老的单步 chat-requirements**,保持向后兼容。
//
// 返回值与老的 ChatRequirements 保持一致,方便调用方无痛切换。
//
// 调用前请先 Bootstrap()。
func (c *Client) ChatRequirementsV2(ctx context.Context) (*ChatRequirementsResp, error) {
	prep, err := c.ChatRequirementsPrepare(ctx)
	if err != nil {
		// prepare 本身失败时,不再尝试 finalize,直接回退到单步接口。
		// 这样新协议上游未开启时也不会阻塞业务。
		if logger := loggerL(); logger != nil {
			logger.Warn("chat-requirements/prepare failed, fallback to single-step",
				zap.Error(err))
		}
		return c.ChatRequirements(ctx)
	}

	// 组装回退用的伪 Resp(即使后面走 finalize,也要把这些字段透传出去)
	resp := &ChatRequirementsResp{Persona: prep.Persona}
	resp.Turnstile.Required = prep.Turnstile.Required
	resp.Proofofwork.Required = prep.Proofofwork.Required
	resp.Proofofwork.Seed = prep.Proofofwork.Seed
	resp.Proofofwork.Difficulty = prep.Proofofwork.Difficulty

	// 本地解 PoW(header 里用的 proof_token 与 finalize 里的 proofofwork 同款)
	var proof string
	if prep.Proofofwork.Required {
		proof = SolveProofToken(prep.Proofofwork.Seed, prep.Proofofwork.Difficulty, c.opts.UserAgent)
	}

	// Turnstile 路由
	var turnstileResp string
	if prep.Turnstile.Required {
		if c.opts.TurnstileSolver != nil {
			sCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			out, solveErr := c.opts.TurnstileSolver.Solve(sCtx, prep.Turnstile.DX)
			if solveErr != nil || out == "" {
				if logger := loggerL(); logger != nil {
					logger.Warn("turnstile solver failed, fallback to single-step chat-requirements",
						zap.Error(solveErr))
				}
				return c.ChatRequirements(ctx)
			}
			turnstileResp = out
		} else {
			// 没配 solver,直接回退单步
			if logger := loggerL(); logger != nil {
				logger.Info("turnstile required but no solver configured, fallback to single-step")
			}
			return c.ChatRequirements(ctx)
		}
	}

	// finalize 拿真正 token
	token, persona, err := c.ChatRequirementsFinalize(ctx, prep.PrepareToken, proof, turnstileResp)
	if err != nil {
		if logger := loggerL(); logger != nil {
			logger.Warn("chat-requirements/finalize failed, fallback to single-step",
				zap.Error(err))
		}
		return c.ChatRequirements(ctx)
	}
	resp.Token = token
	if persona != "" {
		resp.Persona = persona
	}
	if logger := loggerL(); logger != nil {
		logger.Info("chat-requirements two-step ok",
			zap.String("persona", resp.Persona),
			zap.Bool("turnstile_required", prep.Turnstile.Required),
			zap.Bool("pow_required", prep.Proofofwork.Required),
			zap.Int("token_len", len(token)),
		)
	}
	return resp, nil
}

// SSEEvent 单条 SSE 数据。
type SSEEvent struct {
	Event string
	Data  []byte
	Err   error
}
