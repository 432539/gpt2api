// Package gateway 实现 OpenAI 兼容的 /v1/* 入口。
//
// 职责:
//   1. 鉴权(API Key,IP/模型白名单)
//   2. 查模型 → 预扣积分
//   3. 通过调度器拿账号 Lease
//   4. 转译请求体 → 调用 chatgpt.com 上游
//   5. 转译响应(流式 or 聚合) → OpenAI 协议
//   6. 结算(真实 tokens) / 失败退款 / 释放账号锁 / 更新风控状态
package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/432539/gpt2api/internal/apikey"
	"github.com/432539/gpt2api/internal/billing"
	modelpkg "github.com/432539/gpt2api/internal/model"
	"github.com/432539/gpt2api/internal/ratelimit"
	"github.com/432539/gpt2api/internal/scheduler"
	"github.com/432539/gpt2api/internal/upstream/chatgpt"
	"github.com/432539/gpt2api/internal/upstream/minimax"
	"github.com/432539/gpt2api/internal/usage"
	"github.com/432539/gpt2api/internal/user"
	"github.com/432539/gpt2api/pkg/logger"
)

// Handler 聚合网关需要的所有依赖。
type Handler struct {
	Models    *modelpkg.Registry
	Keys      *apikey.Service
	Billing   *billing.Engine
	Scheduler *scheduler.Scheduler
	Groups    *user.GroupCache
	Limiter   *ratelimit.Limiter
	Usage     *usage.Logger
	AccSvc    interface {
		DecryptCookies(ctx context.Context, accountID uint64) (string, error)
	}
	// Images 可选:若挂载,chat/completions 里指定图像模型会自动转派。
	Images *ImagesHandler

	// MiniMax 可选:若注入则 provider=minimax 的模型走 MiniMax 官方 API。
	MiniMax *minimax.Client

	// Settings 可选:若注入则在构造上游 client 时应用动态超时。
	Settings interface {
		GatewayUpstreamTimeoutSec() int
		GatewaySSEReadTimeoutSec() int
	}
}

// upstreamTimeout 返回当前应使用的上游非流式超时。未注入时回退 60s。
func (h *Handler) upstreamTimeout() time.Duration {
	if h.Settings != nil {
		if n := h.Settings.GatewayUpstreamTimeoutSec(); n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return 60 * time.Second
}

// mapUpstreamModelSlug 把品牌名映射到 chatgpt.com 后端实际认的灰度 slug。
//
// 背景:chatgpt.com 的 /f/conversation payload 里 model 字段不是品牌名
// (gpt-5 / gpt-4o),而是"灰度构建版本号"(例如 gpt-5-3)。浏览器在打开
// 页面时从 /backend-api/models 响应里拿到真实 slug,发请求时原样回传。
// 后台直接发 "gpt-5" 会被上游判为非标准客户端,下发一条空 system message
// (is_visually_hidden_from_conversation=true)静默拒绝,看起来就是"没输出"。
//
// 本映射只收录 HAR 抓包实证过的条目;未覆盖的 slug 原样透传,管理员如果
// 遇到 silent rejection,可以:
//  1. 抓包拿到真实 slug 后在 models.upstream_model_slug 里直接填带版本号的值;
//  2. 或临时改成 "auto",让上游自己路由(代价是付费账号可能被降级到免费模型)。
func mapUpstreamModelSlug(s string) string {
	switch s {
	case "gpt-5":
		// HAR 抓包(2026-04,paid 账号,Edge 143)实证
		return "gpt-5-3"
	default:
		return s
	}
}

// roughEstimateTokens 估算 messages prompt tokens(无 tiktoken,简单 len/4)。
func roughEstimateTokens(msgs []chatgpt.ChatMessage) int {
	n := 0
	for _, m := range msgs {
		n += (len(m.Content) + 3) / 4
		n += 4 // role/overhead
	}
	return n + 2
}

// ChatCompletions 是 POST /v1/chat/completions 入口。
func (h *Handler) ChatCompletions(c *gin.Context) {
	startAt := time.Now()
	ak, ok := apikey.FromCtx(c)
	if !ok {
		openAIError(c, http.StatusUnauthorized, "missing_api_key", "缺少 API Key")
		return
	}

	var req ChatCompletionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		openAIError(c, http.StatusBadRequest, "invalid_request_error", "请求参数错误:"+err.Error())
		return
	}

	refID := uuid.NewString()

	// 请求全生命周期的上下文,用于最终写 usage_logs。
	rec := &usage.Log{
		UserID:    ak.UserID,
		KeyID:     ak.ID,
		RequestID: refID,
		Type:      usage.TypeChat,
		IP:        c.ClientIP(),
		UA:        c.Request.UserAgent(),
	}
	defer func() {
		rec.DurationMs = int(time.Since(startAt).Milliseconds())
		if rec.Status == "" {
			rec.Status = usage.StatusFailed
		}
		if h.Usage != nil {
			h.Usage.Write(rec)
		}
	}()
	fail := func(code string) { rec.Status = usage.StatusFailed; rec.ErrorCode = code }

	// 1) 白名单 + 模型
	if !ak.ModelAllowed(req.Model) {
		fail("model_not_allowed")
		openAIError(c, http.StatusForbidden, "model_not_allowed",
			fmt.Sprintf("当前 API Key 无权调用模型 %q", req.Model))
		return
	}
	m, err := h.Models.BySlug(c.Request.Context(), req.Model)
	if err != nil || !m.Enabled {
		fail("model_not_found")
		openAIError(c, http.StatusBadRequest, "model_not_found",
			fmt.Sprintf("模型 %q 不存在或已下架", req.Model))
		return
	}
	// Chat 入口收到图像模型时,转派给图像分支(便于客户端只用 /v1/chat/completions)。
	if m.Type == modelpkg.TypeImage {
		if h.Images == nil {
			fail("image_not_wired")
			openAIError(c, http.StatusNotImplemented, "image_not_wired",
				"图片生成能力未开启,请联系管理员")
			return
		}
		// 借用当前已鉴权/白名单通过的 ak + 模型,走图像流程并以 OpenAI chat 响应格式返回。
		h.Images.handleChatAsImage(c, rec, ak, m, &req, startAt)
		return
	}
	// MiniMax provider:直接调官方 API,不经过 chatgpt.com 账号调度器。
	if m.Provider == modelpkg.ProviderMiniMax {
		h.handleMiniMaxChat(c, rec, ak, m, &req, startAt)
		return
	}
	rec.ModelID = m.ID

	// 2) 分组倍率 + RPM/TPM
	ratio := 1.0
	rpmCap, tpmCap := ak.RPM, ak.TPM
	if h.Groups != nil {
		if g, err := h.Groups.OfUser(c.Request.Context(), ak.UserID); err == nil && g != nil {
			ratio = g.Ratio
			if rpmCap == 0 {
				rpmCap = g.RPMLimit
			}
			if tpmCap == 0 {
				tpmCap = g.TPMLimit
			}
		}
	}

	// 2a) RPM
	if h.Limiter != nil {
		if ok, _, err := h.Limiter.AllowRPM(c.Request.Context(), ak.ID, rpmCap); err == nil && !ok {
			fail("rate_limit_rpm")
			openAIError(c, http.StatusTooManyRequests, "rate_limit_rpm",
				"触发每分钟请求数限制 (RPM),请稍后再试")
			return
		}
	}

	// 3) 预扣(按 max_tokens 或 2048 估算)
	promptTokens := roughEstimateTokens(req.Messages)
	estTokens := req.MaxTokens
	if estTokens <= 0 {
		estTokens = 2048
	}
	estCost := billing.EstimateChat(m, promptTokens, req.MaxTokens, ratio)

	// 2b) TPM(按估算 tokens 预扣,结算时按差额 adjust)
	if h.Limiter != nil {
		if ok, _, err := h.Limiter.AllowTPM(c.Request.Context(), ak.ID, tpmCap,
			int64(promptTokens+estTokens)); err == nil && !ok {
			fail("rate_limit_tpm")
			openAIError(c, http.StatusTooManyRequests, "rate_limit_tpm",
				"触发每分钟 Token 限制 (TPM),请稍后再试")
			return
		}
	}

	if err := h.Billing.PreDeduct(c.Request.Context(), ak.UserID, ak.ID, estCost, refID, "chat prepay"); err != nil {
		if errors.Is(err, billing.ErrInsufficient) {
			fail("insufficient_balance")
			openAIError(c, http.StatusPaymentRequired, "insufficient_balance", "积分不足,请前往「账单与充值」充值后再试")
			return
		}
		fail("billing_error")
		openAIError(c, http.StatusInternalServerError, "billing_error", "计费异常:"+err.Error())
		return
	}

	refunded := false
	refund := func(code string) {
		fail(code)
		if refunded {
			return
		}
		refunded = true
		_ = h.Billing.Refund(context.Background(), ak.UserID, ak.ID, estCost, refID, "chat refund")
	}

	// 4) 调度账号
	lease, err := h.Scheduler.Dispatch(c.Request.Context(), modelpkg.TypeChat)
	if err != nil {
		refund("no_account_available")
		openAIError(c, http.StatusServiceUnavailable, "no_account_available", "账号池暂无可用账号,请稍后重试")
		return
	}
	rec.AccountID = lease.Account.ID
	defer func() { _ = lease.Release(context.Background()) }()

	// 5) 构造上游 client
	cookies, _ := h.AccSvc.DecryptCookies(c.Request.Context(), lease.Account.ID)
	cli, err := chatgpt.New(chatgpt.Options{
		AuthToken: lease.AuthToken,
		DeviceID:  lease.DeviceID,
		SessionID: lease.SessionID,
		ProxyURL:  lease.ProxyURL,
		Cookies:   cookies,
		Timeout:   h.upstreamTimeout(),
	})
	if err != nil {
		refund("upstream_init_error")
		openAIError(c, http.StatusInternalServerError, "upstream_init_error", "上游客户端初始化失败:"+err.Error())
		return
	}

	upstreamModel := m.UpstreamModelSlug
	if upstreamModel == "" {
		upstreamModel = "auto"
	}
	// Model slug 兜底映射:chatgpt.com 后端识别的是"灰度构建版本号",不是
	// 通用品牌名。HAR 抓包(2026-04 paid 账号)显示浏览器实际发送的 slug:
	//   品牌名 "GPT-5"   →  真实 slug "gpt-5-3"
	//   品牌名 "GPT-5 Thinking" → "gpt-5-t-3" (待证实)
	// 直接发裸的 "gpt-5" 会被上游识别为非标准客户端,下发一条
	// is_visually_hidden_from_conversation=true 的空 system message(silent
	// rejection)。这里做一次自动改写,避免运维每次灰度版本号变动都要改表。
	// 管理员若在 models.upstream_model_slug 直接填了带版本号的 slug(如
	// "gpt-5-3"),本映射是 no-op。
	upstreamModel = mapUpstreamModelSlug(upstreamModel)

	// 对齐 Python 参考实现(gen_image.py,已验证可用)的真实顺序:
	//   (a) Bootstrap GET /                  —— 拿 __cf_bm / oai-did / _cfuvid cookie
	//   (b) sentinel/chat-requirements       —— 拿 chat_token + proofofwork 描述
	//   (c) f/conversation/prepare           —— 带 chat_token(!) + proof_token,拿 conduit_token
	//   (d) f/conversation                   —— 带 chat_token + proof_token + conduit_token 发 SSE
	//
	// Python 参考实现 gen_image.py 的 prepare_fconversation 明确要 chat_token,
	// 且不带 sentinel header 会让 prepare 返回空 conduit_token。

	// (a) Bootstrap
	bootCtx, cancelBoot := context.WithTimeout(c.Request.Context(), 15*time.Second)
	_ = cli.Bootstrap(bootCtx)
	cancelBoot()

	// (b) chat-requirements —— 优先走新两步协议(prepare + finalize),solver 未配置
	// 或失败时会自动回退到单步老接口(V2 内部实现)。
	reqCtx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	cr, err := cli.ChatRequirementsV2(reqCtx)
	if err != nil {
		h.handleUpstreamErr(c, lease, err, func() { refund("upstream_error") })
		return
	}

	// POW(异步,5s 超时)
	var proofToken string
	if cr.Proofofwork.Required {
		proofCtx, cancelProof := context.WithTimeout(c.Request.Context(), 5*time.Second)
		proofCh := make(chan string, 1)
		go func() { proofCh <- cr.SolveProof("") }()
		select {
		case <-proofCtx.Done():
			cancelProof()
			h.Scheduler.MarkWarned(c.Request.Context(), lease.Account.ID)
			refund("pow_timeout")
			openAIError(c, http.StatusServiceUnavailable, "pow_timeout",
				"上游风控(PoW)未在规定时间内完成,请重试")
			return
		case proofToken = <-proofCh:
			cancelProof()
		}
		if proofToken == "" {
			h.Scheduler.MarkWarned(c.Request.Context(), lease.Account.ID)
			refund("pow_failed")
			openAIError(c, http.StatusServiceUnavailable, "pow_failed",
				"上游风控(PoW)校验失败,请稍后重试")
			return
		}
	}
	// Turnstile 在新账号 / 新 device 场景几乎必现,但它实际上是"建议",
	// 大多数情况下直接继续发 /conversation 也能被上游接受。这里只打 warn
	// 日志,不阻断(参考 gen_image.py / chat2api 的通用做法)。
	if cr.Turnstile.Required {
		logger.L().Warn("chat turnstile required, continue anyway",
			zap.Uint64("account_id", lease.Account.ID))
	}

	// 免费账号(persona=chatgpt-freeaccount)对高级模型(如 gpt-5)会静默不生成,
	// SSE 只会下发一条 hidden system preamble 就结束。chatgpt.com 浏览器端对免费账号
	// 实际发的 model 就是 "auto",由上游自己选。我们强制降级,避免"哑巴失败"。
	if cr.IsFreeAccount() && upstreamModel != "auto" {
		logger.L().Warn("free account requesting premium model, downgrade to auto",
			zap.Uint64("account_id", lease.Account.ID),
			zap.String("requested_model", upstreamModel))
		upstreamModel = "auto"
	}

	chatOpt := chatgpt.FChatOpts{
		UpstreamModel: upstreamModel,
		Messages:      req.Messages,
		ChatToken:     cr.Token,
		ProofToken:    proofToken,
	}

	// (c) f/conversation/prepare(必须在 chat-requirements 之后,且带 sentinel header)
	prepCtx, cancelPrep := context.WithTimeout(c.Request.Context(), 30*time.Second)
	conduit, err := cli.PrepareFChat(prepCtx, chatOpt)
	cancelPrep()
	if err != nil {
		logger.L().Warn("f/conversation/prepare failed, continue without conduit",
			zap.Uint64("account_id", lease.Account.ID),
			zap.String("upstream_model", upstreamModel),
			zap.Error(err))
		conduit = ""
	}
	chatOpt.ConduitToken = conduit

	logger.L().Info("chat f/conversation send",
		zap.Uint64("account_id", lease.Account.ID),
		zap.String("upstream_model", upstreamModel),
		zap.Int("chat_token_len", len(cr.Token)),
		zap.Int("proof_token_len", len(proofToken)),
		zap.Int("conduit_len", len(conduit)),
		zap.Bool("turnstile_required", cr.Turnstile.Required),
		zap.String("persona", cr.Persona),
	)

	// (d) f/conversation SSE
	stream, err := cli.StreamFChat(c.Request.Context(), chatOpt)
	if err != nil {
		h.handleUpstreamErr(c, lease, err, func() { refund("upstream_error") })
		return
	}

	// 8) 转发响应
	id := "chatcmpl-" + uuid.NewString()
	if req.Stream {
		h.streamOpenAI(c, id, req.Model, stream, cr.IsFreeAccount())
	} else {
		h.collectOpenAI(c, id, req.Model, stream, cr.IsFreeAccount())
	}

	// 9) 结算
	completionTokens := h.lastCompletionTokens(c)
	actual := billing.ComputeChatCost(m, promptTokens, completionTokens, ratio)
	if err := h.Billing.Settle(context.Background(), ak.UserID, ak.ID, estCost, actual, refID, "chat settle"); err != nil {
		logger.L().Error("billing settle", zap.Error(err), zap.String("ref", refID))
	}
	_ = h.Keys.DAO().TouchUsage(context.Background(), ak.ID, c.ClientIP(), actual)

	// 10) TPM 差额补偿:真实 tokens 可能低于估算,这里可以 adjust 还桶。
	if h.Limiter != nil {
		real := int64(promptTokens + completionTokens)
		est := int64(promptTokens + estTokens)
		if diff := real - est; diff > 0 {
			h.Limiter.AdjustTPM(context.Background(), ak.ID, tpmCap, diff)
		}
	}

	// 11) usage 记录
	rec.Status = usage.StatusSuccess
	rec.InputTokens = promptTokens
	rec.OutputTokens = completionTokens
	rec.CreditCost = actual
}

// streamOpenAI 将上游 SSE 事件转为 OpenAI 风格流式响应。
// freeAccount 标记上游 persona 是 chatgpt-freeaccount,用于在"没拿到任何内容"
// 的兜底分支给出更精准的错误消息(免费账号会被上游静默拒绝)。
func (h *Handler) streamOpenAI(c *gin.Context, id, model string, stream <-chan chatgpt.SSEEvent, freeAccount bool) {
	w := c.Writer
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)

	// 先发一个 role=assistant 的空 delta(OpenAI 规范起始)
	writeChunk(w, flusher, id, model, DeltaMsg{Role: "assistant"}, nil)

	var extr deltaExtractor
	var total strings.Builder

	evCount := 0
	silentlyRejected := false
	for ev := range stream {
		if ev.Err != nil {
			logger.L().Warn("upstream stream err", zap.Error(ev.Err))
			break
		}
		if len(ev.Data) == 0 {
			continue
		}
		evCount++
		// 对前若干帧开 Info 级别日志,方便线上快速定位 "SSE 有事件但正文为空" 的协议级问题。
		// truncate 2048:大多数关键元数据事件(含 error/moderation_result)一帧就能看全。
		// 稳定后可改回 Debug。
		if evCount <= 16 {
			logger.L().Info("chat sse raw", zap.Int("n", evCount),
				zap.String("event", ev.Event),
				zap.String("data", truncate(string(ev.Data), 2048)))
		}
		if !silentlyRejected && isSilentRejection(ev.Data) {
			silentlyRejected = true
		}
		delta, final, err := extr.Extract(ev.Data)
		if err != nil {
			continue
		}
		if delta != "" {
			total.WriteString(delta)
			writeChunk(w, flusher, id, model, DeltaMsg{Content: delta}, nil)
		}
		if final {
			break
		}
	}
	logger.L().Info("chat sse done", zap.Int("events", evCount),
		zap.Int("content_len", total.Len()),
		zap.Bool("silently_rejected", silentlyRejected))

	// 兜底:上游接受了请求但没产出任何可见文本
	if total.Len() == 0 && evCount > 0 {
		writeChunk(w, flusher, id, model, DeltaMsg{Content: emptyReplyMessage(freeAccount, silentlyRejected)}, nil)
	}

	stop := "stop"
	writeChunk(w, flusher, id, model, DeltaMsg{}, &stop)
	fmt.Fprintf(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}

	// 把内容长度记录到 ctx 供结算使用。
	c.Set("completion_tokens", (total.Len()+3)/4)
}

func (h *Handler) collectOpenAI(c *gin.Context, id, model string, stream <-chan chatgpt.SSEEvent, freeAccount bool) {
	var extr deltaExtractor
	var content strings.Builder
	evCount := 0
	silentlyRejected := false
	for ev := range stream {
		if ev.Err != nil {
			logger.L().Warn("upstream collect err", zap.Error(ev.Err))
			break
		}
		if len(ev.Data) == 0 {
			continue
		}
		evCount++
		if evCount <= 16 {
			logger.L().Info("chat collect raw", zap.Int("n", evCount),
				zap.String("event", ev.Event),
				zap.String("data", truncate(string(ev.Data), 2048)))
		}
		if !silentlyRejected && isSilentRejection(ev.Data) {
			silentlyRejected = true
		}
		delta, final, _ := extr.Extract(ev.Data)
		if delta != "" {
			content.WriteString(delta)
		}
		if final {
			break
		}
	}
	logger.L().Info("chat collect done", zap.Int("events", evCount),
		zap.Int("content_len", content.Len()),
		zap.Bool("silently_rejected", silentlyRejected))

	// 兜底:上游接受了请求但没产出任何可见文本(见 streamOpenAI 同名兜底的说明)
	if content.Len() == 0 && evCount > 0 {
		content.WriteString(emptyReplyMessage(freeAccount, silentlyRejected))
	}

	completionTokens := (content.Len() + 3) / 4
	c.Set("completion_tokens", completionTokens)

	resp := ChatCompletionResponse{
		ID:      id,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []ChatCompletionChoice{{
			Index:        0,
			Message:      chatgpt.ChatMessage{Role: "assistant", Content: content.String()},
			FinishReason: "stop",
		}},
		Usage: ChatCompletionUsage{
			PromptTokens:     0, // 由外层填
			CompletionTokens: completionTokens,
			TotalTokens:      completionTokens,
		},
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) lastCompletionTokens(c *gin.Context) int {
	if v, ok := c.Get("completion_tokens"); ok {
		if i, ok := v.(int); ok {
			return i
		}
	}
	return 0
}

// handleUpstreamErr 根据上游错误降级账号并回传 OpenAI 错误。
func (h *Handler) handleUpstreamErr(c *gin.Context, lease *scheduler.Lease, err error, refund func()) {
	var ue *chatgpt.UpstreamError
	if errors.As(err, &ue) {
		switch {
		case ue.IsRateLimited():
			h.Scheduler.MarkRateLimited(c.Request.Context(), lease.Account.ID)
		case ue.IsUnauthorized():
			h.Scheduler.MarkDead(c.Request.Context(), lease.Account.ID)
		}
		refund()
		logger.L().Error("chat upstream error",
			zap.Int("status", ue.Status),
			zap.Uint64("account_id", lease.Account.ID),
			zap.String("body", truncate(ue.Body, 1500)))
		openAIError(c, http.StatusBadGateway, "upstream_error",
			fmt.Sprintf("上游返回错误(HTTP %d):%s", ue.Status, truncate(ue.Body, 200)))
		return
	}
	refund()
	openAIError(c, http.StatusBadGateway, "upstream_error", "上游请求失败:"+err.Error())
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// isSilentRejection 识别 ChatGPT 对免费账号/高限流账号的"静默拒绝"特征:
// 上游下发一条 author=system + is_visually_hidden_from_conversation=true
// + end_turn=true + parts=[""] 的 delta 事件,让前端看起来"什么都没发生"就终止。
// 这种 pattern 和 payload 是否合规完全无关,是上游策略层的硬门槛。
func isSilentRejection(data []byte) bool {
	s := string(data)
	// 用字符串快速判定,避免每帧都做完整 JSON 反序列化。
	// 三个字段同时出现才算,防止误判正常 assistant 消息。
	return strings.Contains(s, `"is_visually_hidden_from_conversation": true`) &&
		strings.Contains(s, `"role": "system"`) &&
		strings.Contains(s, `"end_turn": true`)
}

// emptyReplyMessage 根据账号类型和上游信号,返回给最终用户看的兜底文案。
func emptyReplyMessage(freeAccount, silentlyRejected bool) string {
	switch {
	case silentlyRejected && freeAccount:
		return "上游检测到当前账号为免费版(chatgpt-freeaccount),已静默拒绝本次请求。" +
			"请联系管理员更换 ChatGPT Plus / Team 账号后再试。"
	case silentlyRejected:
		return "上游已接受请求但静默终止对话(常见于账号被限流或触发内容审核)," +
			"请稍后重试,若仍失败请更换模型或账号。"
	case freeAccount:
		return "当前账号为 ChatGPT 免费版,上游未产出内容。请更换 Plus/Team 账号后再试。"
	default:
		return "上游未产出回答内容,可能触发了内容审核或账号被临时限流,请稍后重试。"
	}
}

func writeChunk(w io.Writer, f http.Flusher, id, model string, delta DeltaMsg, finish *string) {
	chunk := ChatCompletionChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []ChatCompletionChunkChoice{{Index: 0, Delta: delta, FinishReason: finish}},
	}
	b, _ := json.Marshal(chunk)
	fmt.Fprintf(w, "data: %s\n\n", b)
	if f != nil {
		f.Flush()
	}
}

// openAIError 按 OpenAI 规范返回错误。
func openAIError(c *gin.Context, httpStatus int, code, msg string) {
	c.AbortWithStatusJSON(httpStatus, gin.H{
		"error": gin.H{
			"message": msg,
			"type":    "invalid_request_error",
			"code":    code,
		},
	})
}

// ListModels GET /v1/models
func (h *Handler) ListModels(c *gin.Context) {
	list, err := h.Models.ListEnabled(c.Request.Context())
	if err != nil {
		openAIError(c, http.StatusInternalServerError, "list_models_error", "获取模型列表失败:"+err.Error())
		return
	}
	data := make([]gin.H, 0, len(list))
	for _, m := range list {
		ownedBy := "chatgpt"
		if m.Provider != "" {
			ownedBy = m.Provider
		}
		data = append(data, gin.H{
			"id":       m.Slug,
			"object":   "model",
			"created":  m.CreatedAt.Unix(),
			"owned_by": ownedBy,
		})
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": data})
}

// handleMiniMaxChat 处理 provider=minimax 的文字对话请求。
//
// 与 chatgpt 路径的主要区别:
//   - 不需要账号调度器,直接用 MiniMax 官方 API Key;
//   - SSE 格式是标准 OpenAI delta chunk,而非 chatgpt.com 的 JSON-Patch 格式;
//   - 计费按实际 tokens(从响应体拿到)结算;
//   - 没有"免费账号降级"等逻辑。
func (h *Handler) handleMiniMaxChat(c *gin.Context, rec *usage.Log, ak *apikey.APIKey,
	m *modelpkg.Model, req *ChatCompletionsRequest, startAt time.Time) {

	rec.ModelID = m.ID

	if h.MiniMax == nil {
		rec.Status = usage.StatusFailed
		rec.ErrorCode = "minimax_not_configured"
		openAIError(c, http.StatusServiceUnavailable, "minimax_not_configured",
			"MiniMax API 未配置,请在 config.yaml 的 minimax.api_key 中填入 API Key")
		return
	}

	refID := uuid.NewString()

	// 倍率 + RPM
	ratio := 1.0
	rpmCap, tpmCap := ak.RPM, ak.TPM
	if h.Groups != nil {
		if g, err := h.Groups.OfUser(c.Request.Context(), ak.UserID); err == nil && g != nil {
			ratio = g.Ratio
			if rpmCap == 0 {
				rpmCap = g.RPMLimit
			}
			if tpmCap == 0 {
				tpmCap = g.TPMLimit
			}
		}
	}
	if h.Limiter != nil {
		if ok, _, err := h.Limiter.AllowRPM(c.Request.Context(), ak.ID, rpmCap); err == nil && !ok {
			rec.Status = usage.StatusFailed
			rec.ErrorCode = "rate_limit_rpm"
			openAIError(c, http.StatusTooManyRequests, "rate_limit_rpm",
				"触发每分钟请求数限制 (RPM),请稍后再试")
			return
		}
	}

	// 预扣
	promptTokens := roughEstimateTokens(req.Messages)
	estTokens := req.MaxTokens
	if estTokens <= 0 {
		estTokens = 2048
	}
	estCost := billing.EstimateChat(m, promptTokens, estTokens, ratio)

	if h.Limiter != nil {
		if ok, _, err := h.Limiter.AllowTPM(c.Request.Context(), ak.ID, tpmCap,
			int64(promptTokens+estTokens)); err == nil && !ok {
			rec.Status = usage.StatusFailed
			rec.ErrorCode = "rate_limit_tpm"
			openAIError(c, http.StatusTooManyRequests, "rate_limit_tpm",
				"触发每分钟 Token 限制 (TPM),请稍后再试")
			return
		}
	}

	if err := h.Billing.PreDeduct(c.Request.Context(), ak.UserID, ak.ID, estCost, refID, "minimax chat prepay"); err != nil {
		rec.Status = usage.StatusFailed
		if errors.Is(err, billing.ErrInsufficient) {
			rec.ErrorCode = "insufficient_balance"
			openAIError(c, http.StatusPaymentRequired, "insufficient_balance",
				"积分不足,请前往「账单与充值」充值后再试")
			return
		}
		rec.ErrorCode = "billing_error"
		openAIError(c, http.StatusInternalServerError, "billing_error", "计费异常:"+err.Error())
		return
	}

	refunded := false
	refund := func(code string) {
		rec.Status = usage.StatusFailed
		rec.ErrorCode = code
		if refunded {
			return
		}
		refunded = true
		_ = h.Billing.Refund(context.Background(), ak.UserID, ak.ID, estCost, refID, "minimax chat refund")
	}

	upstreamModel := m.UpstreamModelSlug
	if upstreamModel == "" {
		upstreamModel = m.Slug
	}

	id := "chatcmpl-" + uuid.NewString()

	if req.Stream {
		stream, err := h.MiniMax.StreamChat(c.Request.Context(), upstreamModel, req.Messages, req.MaxTokens)
		if err != nil {
			refund("upstream_error")
			openAIError(c, http.StatusBadGateway, "upstream_error", "MiniMax 请求失败:"+err.Error())
			return
		}
		h.streamMiniMax(c, id, req.Model, stream)
	} else {
		content, promptTok, completionTok, err := h.MiniMax.Chat(c.Request.Context(), upstreamModel, req.Messages, req.MaxTokens)
		if err != nil {
			refund("upstream_error")
			openAIError(c, http.StatusBadGateway, "upstream_error", "MiniMax 请求失败:"+err.Error())
			return
		}
		if promptTok > 0 {
			promptTokens = promptTok
		}
		completionTokens := completionTok
		if completionTokens == 0 {
			completionTokens = (len(content) + 3) / 4
		}

		actual := billing.ComputeChatCost(m, promptTokens, completionTokens, ratio)
		if err := h.Billing.Settle(context.Background(), ak.UserID, ak.ID, estCost, actual, refID, "minimax chat settle"); err != nil {
			logger.L().Error("minimax billing settle", zap.Error(err), zap.String("ref", refID))
		}
		_ = h.Keys.DAO().TouchUsage(context.Background(), ak.ID, c.ClientIP(), actual)

		rec.Status = usage.StatusSuccess
		rec.InputTokens = promptTokens
		rec.OutputTokens = completionTokens
		rec.CreditCost = actual
		rec.DurationMs = int(time.Since(startAt).Milliseconds())

		resp := ChatCompletionResponse{
			ID:      id,
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   req.Model,
			Choices: []ChatCompletionChoice{{
				Index:        0,
				Message:      chatgpt.ChatMessage{Role: "assistant", Content: content},
				FinishReason: "stop",
			}},
			Usage: ChatCompletionUsage{
				PromptTokens:     promptTokens,
				CompletionTokens: completionTokens,
				TotalTokens:      promptTokens + completionTokens,
			},
		}
		c.JSON(http.StatusOK, resp)
		return
	}

	// 流式时结算(从 ctx 里读 completion_tokens)
	completionTokens := h.lastCompletionTokens(c)
	actual := billing.ComputeChatCost(m, promptTokens, completionTokens, ratio)
	if err := h.Billing.Settle(context.Background(), ak.UserID, ak.ID, estCost, actual, refID, "minimax chat settle"); err != nil {
		logger.L().Error("minimax billing settle", zap.Error(err), zap.String("ref", refID))
	}
	_ = h.Keys.DAO().TouchUsage(context.Background(), ak.ID, c.ClientIP(), actual)
	if h.Limiter != nil {
		real := int64(promptTokens + completionTokens)
		est := int64(promptTokens + estTokens)
		if diff := real - est; diff > 0 {
			h.Limiter.AdjustTPM(context.Background(), ak.ID, tpmCap, diff)
		}
	}

	rec.Status = usage.StatusSuccess
	rec.InputTokens = promptTokens
	rec.OutputTokens = completionTokens
	rec.CreditCost = actual
}

// streamMiniMax 将 MiniMax 标准 OpenAI-format SSE 转为 OpenAI 流式响应。
//
// MiniMax 使用标准格式:
//
//	data: {"choices":[{"delta":{"content":"..."},"finish_reason":null}]}
//	data: {"choices":[{"delta":{},"finish_reason":"stop"}]}
//	data: [DONE]
func (h *Handler) streamMiniMax(c *gin.Context, id, model string, stream <-chan chatgpt.SSEEvent) {
	w := c.Writer
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)

	writeChunk(w, flusher, id, model, DeltaMsg{Role: "assistant"}, nil)

	var total strings.Builder

	for ev := range stream {
		if ev.Err != nil {
			logger.L().Warn("minimax stream err", zap.Error(ev.Err))
			break
		}
		if len(ev.Data) == 0 {
			continue
		}
		delta, done := extractMiniMaxDelta(ev.Data)
		if delta != "" {
			total.WriteString(delta)
			writeChunk(w, flusher, id, model, DeltaMsg{Content: delta}, nil)
		}
		if done {
			break
		}
	}

	stop := "stop"
	writeChunk(w, flusher, id, model, DeltaMsg{}, &stop)
	fmt.Fprintf(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
	c.Set("completion_tokens", (total.Len()+3)/4)
}

// extractMiniMaxDelta 从标准 OpenAI delta chunk 中提取增量文本。
// 返回 (增量内容, 是否结束)。
func extractMiniMaxDelta(data []byte) (string, bool) {
	var chunk struct {
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
			FinishReason *string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &chunk); err != nil {
		return "", false
	}
	if len(chunk.Choices) == 0 {
		return "", false
	}
	choice := chunk.Choices[0]
	done := choice.FinishReason != nil && *choice.FinishReason == "stop"
	return choice.Delta.Content, done
}
