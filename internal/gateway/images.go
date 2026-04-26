package gateway

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/432539/gpt2api/internal/apikey"
	"github.com/432539/gpt2api/internal/billing"
	"github.com/432539/gpt2api/internal/image"
	modelpkg "github.com/432539/gpt2api/internal/model"
	"github.com/432539/gpt2api/internal/upstream/chatgpt"
	"github.com/432539/gpt2api/internal/usage"
	"github.com/432539/gpt2api/pkg/logger"
)

// 单张参考图的硬上限(字节)。chatgpt.com 的 /backend-api/files 实测上限大致 20MB。
const maxReferenceImageBytes = 20 * 1024 * 1024

// 同一次请求最多携带的参考图数量。
const maxReferenceImages = 4

// 异步 Runner 外层超时 7 分钟;查询端多留一点缓冲后把遗留 running 任务兜底置失败。
const imageTaskStaleAfter = 8 * time.Minute

// chatMsg 是 OpenAI chat message 的本地别名,便于 handleChatAsImage 内部表达。
type chatMsg = chatgpt.ChatMessage

// ImagesHandler 挂载在 /v1/images/* 下的处理器。
//
// 复用 Handler 的依赖(鉴权/模型/计费/限流/usage)加上专属的 image.Runner 和 DAO。
// 路由:
//
//	POST /v1/images/generations       同步生图(默认)
//	GET  /v1/images/tasks/:id         查询历史任务(按 task_id)
type ImagesHandler struct {
	*Handler
	Runner *image.Runner
	DAO    *image.DAO
	// ImageAccResolver 可选:代理下载上游图片时用于解出账号 AT/cookies/proxy。
	// 未注入时 /p/img 路径会返回 503。
	ImageAccResolver ImageAccountResolver
}

// ImageGenRequest OpenAI 兼容入参。
//
// 对 reference_images 的扩展:OpenAI 的 /images/generations 规范没有这个字段;
// 这里加一项非标准扩展,便于 Playground / Web UI 发起"图生图"走同一条 generations 路径。
// 每一项可以是:
//   - https:// URL       直接 HTTP GET
//   - data:<mime>;base64,xxxx   dataURL
//   - 纯 base64 字符串            兼容
type ImageGenRequest struct {
	Model           string   `json:"model"`
	Prompt          string   `json:"prompt"`
	N               int      `json:"n"`
	Size            string   `json:"size"`
	Quality         string   `json:"quality,omitempty"`
	Style           string   `json:"style,omitempty"`
	ResponseFormat  string   `json:"response_format,omitempty"` // url | b64_json(暂仅支持 url)
	User            string   `json:"user,omitempty"`
	Async           bool     `json:"async,omitempty"`            // true 时立即返回 task_id,后台继续生成
	ReferenceImages []string `json:"reference_images,omitempty"` // 非标准扩展,见注释
	// Upscale 非标准扩展:控制"本服务对原图做本地高清放大"的目标档位。
	// 可选值:""(原图直出,默认)/ "2k"(长边 2560) / "4k"(长边 3840)。
	// 算法:golang.org/x/image/draw.CatmullRom(传统插值,不是 AI 超分)。
	// 生效时机:图片代理 URL 首次请求时做一次 decode+放大+PNG 编码,之后进程内
	// LRU 缓存命中毫秒级返回。仅影响 /v1/images/proxy/... 的出口字节,不改原图。
	Upscale string `json:"upscale,omitempty"`
}

// ImageGenData 单张图响应。
type ImageGenData struct {
	URL           string `json:"url,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
	FileID        string `json:"file_id,omitempty"` // chatgpt.com 侧原始 id(用于对账)
}

// ImageGenResponse OpenAI 兼容返回。
type ImageGenResponse struct {
	Created int64          `json:"created"`
	Data    []ImageGenData `json:"data"`
	TaskID  string         `json:"task_id,omitempty"`
}

// ImageGenerations POST /v1/images/generations。
func (h *ImagesHandler) ImageGenerations(c *gin.Context) {
	startAt := time.Now()
	ak, ok := apikey.FromCtx(c)
	if !ok {
		openAIError(c, http.StatusUnauthorized, "missing_api_key", "缺少 API Key")
		return
	}

	var req ImageGenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		openAIError(c, http.StatusBadRequest, "invalid_request_error", "请求参数错误:"+err.Error())
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		openAIError(c, http.StatusBadRequest, "invalid_request_error", "prompt 不能为空")
		return
	}
	if req.Model == "" {
		req.Model = "gpt-image-2"
	}
	if req.N <= 0 {
		req.N = 1
	}
	if req.N > 4 {
		req.N = 4 // 目前 IMG2 终稿单轮稳定产出 1-4 张,保守上限
	}
	if req.Size == "" {
		req.Size = "1024x1024"
	}
	req.Upscale = image.ValidateUpscale(req.Upscale)
	async := req.Async

	refID := uuid.NewString()
	rec := &usage.Log{
		UserID:    ak.UserID,
		KeyID:     ak.ID,
		RequestID: refID,
		Type:      usage.TypeImage,
		IP:        c.ClientIP(),
		UA:        c.Request.UserAgent(),
	}
	writeUsage := true
	defer func() {
		if !writeUsage {
			return
		}
		rec.DurationMs = int(time.Since(startAt).Milliseconds())
		if rec.Status == "" {
			rec.Status = usage.StatusFailed
		}
		if h.Usage != nil {
			h.Usage.Write(rec)
		}
	}()
	fail := func(code string) { rec.Status = usage.StatusFailed; rec.ErrorCode = code }

	// 1) 模型白名单
	if !ak.ModelAllowed(req.Model) {
		fail("model_not_allowed")
		openAIError(c, http.StatusForbidden, "model_not_allowed",
			fmt.Sprintf("当前 API Key 无权调用模型 %q", req.Model))
		return
	}
	m, err := h.Models.BySlug(c.Request.Context(), req.Model)
	if err != nil || m == nil || !m.Enabled {
		fail("model_not_found")
		openAIError(c, http.StatusBadRequest, "model_not_found",
			fmt.Sprintf("模型 %q 不存在或已下架", req.Model))
		return
	}
	if m.Type != modelpkg.TypeImage {
		fail("model_type_mismatch")
		openAIError(c, http.StatusBadRequest, "model_type_mismatch",
			fmt.Sprintf("模型 %q 不是图像模型,不能用于 /v1/images/generations", req.Model))
		return
	}
	rec.ModelID = m.ID

	// 2) 分组倍率 + RPM 限流(图像不走 TPM)
	ratio := 1.0
	rpmCap := ak.RPM
	if h.Groups != nil {
		if g, err := h.Groups.OfUser(c.Request.Context(), ak.UserID); err == nil && g != nil {
			ratio = g.Ratio
			if rpmCap == 0 {
				rpmCap = g.RPMLimit
			}
		}
	}
	if h.Limiter != nil {
		if ok, _, err := h.Limiter.AllowRPM(c.Request.Context(), ak.ID, rpmCap); err == nil && !ok {
			fail("rate_limit_rpm")
			openAIError(c, http.StatusTooManyRequests, "rate_limit_rpm",
				"触发每分钟请求数限制 (RPM),请稍后再试")
			return
		}
	}

	// 若本地模型配置了外置渠道(OpenAI DALL·E / Gemini imagen 等),优先走渠道。
	// 参考图场景(reference_images)仍走原 ChatGPT 账号池 Runner。
	// async=true 需要落 image_tasks 并通过 /v1/images/tasks/:id 查询,因此走内置 Runner。
	if h.Channels != nil && !async {
		if handled := h.dispatchImageToChannel(c, ak, m, &req, rec, ratio); handled {
			return
		}
	}
	if async {
		if h.DAO == nil {
			fail("not_configured")
			openAIError(c, http.StatusInternalServerError, "not_configured", "图片任务存储未初始化,无法提交异步任务")
			return
		}
		if h.Runner == nil {
			fail("not_configured")
			openAIError(c, http.StatusInternalServerError, "not_configured", "图片 Runner 未初始化,请联系管理员")
			return
		}
	}

	// 3) 预扣(图像按定价,est = actual)
	cost := billing.ComputeImageCost(m, req.N, ratio)
	if cost > 0 {
		if err := h.Billing.PreDeduct(c.Request.Context(), ak.UserID, ak.ID, cost, refID, "image prepay"); err != nil {
			if errors.Is(err, billing.ErrInsufficient) {
				fail("insufficient_balance")
				openAIError(c, http.StatusPaymentRequired, "insufficient_balance",
					"积分不足,请前往「账单与充值」充值后再试")
				return
			}
			fail("billing_error")
			openAIError(c, http.StatusInternalServerError, "billing_error", "计费异常:"+err.Error())
			return
		}
	}
	refunded := false
	refund := func(code string) {
		fail(code)
		if refunded || cost == 0 {
			return
		}
		refunded = true
		_ = h.Billing.Refund(context.Background(), ak.UserID, ak.ID, cost, refID, "image refund")
	}

	// 4) 落任务
	taskID := image.GenerateTaskID()
	taskStatus := image.StatusDispatched
	if async {
		taskStatus = image.StatusQueued
	}
	task := &image.Task{
		TaskID:          taskID,
		UserID:          ak.UserID,
		KeyID:           ak.ID,
		ModelID:         m.ID,
		Prompt:          req.Prompt,
		N:               req.N,
		Size:            req.Size,
		Upscale:         req.Upscale,
		Status:          taskStatus,
		EstimatedCredit: cost,
	}
	if h.DAO != nil {
		if err := h.DAO.Create(c.Request.Context(), task); err != nil {
			refund("billing_error")
			openAIError(c, http.StatusInternalServerError, "internal_error", "创建任务失败:"+err.Error())
			return
		}
	}

	// 4.5) 解析 reference_images(图生图 / 图像编辑入口都走到这里)
	refs, err := decodeReferenceInputs(c.Request.Context(), req.ReferenceImages)
	if err != nil {
		refund("invalid_request_error")
		openAIError(c, http.StatusBadRequest, "invalid_reference_image", "参考图解析失败:"+err.Error())
		return
	}

	// async=true:请求只负责提交任务;后台 goroutine 继续跑完整 Runner + 计费闭环。
	if async {
		writeUsage = false
		maxAttempts := 2
		if len(refs) > 0 {
			maxAttempts = 1
		}
		h.startAsyncImageRun(imageAsyncJob{
			TaskID:        taskID,
			RefID:         refID,
			UserID:        ak.UserID,
			KeyID:         ak.ID,
			ModelID:       m.ID,
			UpstreamModel: m.UpstreamModelSlug,
			Prompt:        maybeAppendClaritySuffix(req.Prompt),
			N:             req.N,
			MaxAttempts:   maxAttempts,
			Timeout:       7 * time.Minute,
			Cost:          cost,
			Refs:          refs,
			StartAt:       startAt,
			IP:            c.ClientIP(),
			UA:            c.Request.UserAgent(),
		})
		c.JSON(http.StatusAccepted, gin.H{
			"created": time.Now().Unix(),
			"task_id": taskID,
			"status":  image.StatusQueued,
		})
		return
	}

	// 5) 执行(同步阻塞)
	//
	// 单请求硬上限 7 分钟:Runner 默认 per-attempt 6 分钟
	// (SSE ~60s + PollMaxWait 300s + 缓冲),外层再留 1 分钟
	// 给账号调度 + 签名 URL 换取等周边耗时。IMG2 已正式上线,不再做 preview_only 重试。
	runCtx, cancel := context.WithTimeout(c.Request.Context(), 7*time.Minute)
	defer cancel()

	// 带参考图时,多轮重试没什么意义(反而会重复上传参考图),只留 1 次尝试。
	maxAttempts := 2
	if len(refs) > 0 {
		maxAttempts = 1
	}

	res := h.Runner.Run(runCtx, image.RunOptions{
		TaskID:        taskID,
		UserID:        ak.UserID,
		KeyID:         ak.ID,
		ModelID:       m.ID,
		UpstreamModel: m.UpstreamModelSlug,
		Prompt:        maybeAppendClaritySuffix(req.Prompt),
		N:             req.N,
		MaxAttempts:   maxAttempts,
		References:    refs,
	})
	rec.AccountID = res.AccountID

	if res.Status != image.StatusSuccess {
		refund(ifEmpty(res.ErrorCode, "upstream_error"))
		httpStatus := http.StatusBadGateway
		if res.ErrorCode == image.ErrNoAccount {
			httpStatus = http.StatusServiceUnavailable
		}
		if res.ErrorCode == image.ErrRateLimited {
			httpStatus = http.StatusServiceUnavailable
		}
		openAIError(c, httpStatus, ifEmpty(res.ErrorCode, "upstream_error"),
			localizeImageErr(res.ErrorCode, res.ErrorMessage))
		return
	}

	// 6) 结算
	if cost > 0 {
		if err := h.Billing.Settle(context.Background(), ak.UserID, ak.ID, cost, cost, refID, "image settle"); err != nil {
			logger.L().Error("billing settle image", zap.Error(err), zap.String("ref", refID))
		}
	}
	_ = h.Keys.DAO().TouchUsage(context.Background(), ak.ID, c.ClientIP(), cost)

	// 7) usage
	rec.Status = usage.StatusSuccess
	rec.CreditCost = cost
	// 实际产出张数:优先按 SignedURLs 计数,空时回落到请求张数,
	// 兜底再回落到 1 —— 旧版只写 0 会让"图片张数"统计长期偏小。
	rec.ImageCount = len(res.SignedURLs)
	if rec.ImageCount <= 0 {
		rec.ImageCount = req.N
	}
	if rec.ImageCount <= 0 {
		rec.ImageCount = 1
	}

	// 8) DAO 回写 credit_cost(Runner 已经 MarkSuccess,这里只补 credit_cost)
	if h.DAO != nil {
		_ = h.DAO.UpdateCost(c.Request.Context(), taskID, cost)
	}

	// 9) 响应:URL 统一走自家代理,防止 chatgpt.com estuary/content 防盗链
	out := ImageGenResponse{
		Created: time.Now().Unix(),
		TaskID:  taskID,
		Data:    make([]ImageGenData, 0, len(res.SignedURLs)),
	}
	for i := range res.SignedURLs {
		d := ImageGenData{URL: BuildImageProxyURL(taskID, i, ImageProxyTTL)}
		if i < len(res.FileIDs) {
			d.FileID = strings.TrimPrefix(res.FileIDs[i], "sed:")
		}
		out.Data = append(out.Data, d)
	}
	c.JSON(http.StatusOK, out)
}

type imageAsyncJob struct {
	TaskID        string
	RefID         string
	UserID        uint64
	KeyID         uint64
	ModelID       uint64
	UpstreamModel string
	Prompt        string
	N             int
	MaxAttempts   int
	Timeout       time.Duration
	Cost          int64
	Refs          []image.ReferenceImage
	StartAt       time.Time
	IP            string
	UA            string
}

func (h *ImagesHandler) startAsyncImageRun(job imageAsyncJob) {
	go func() {
		rec := &usage.Log{
			UserID:    job.UserID,
			KeyID:     job.KeyID,
			ModelID:   job.ModelID,
			RequestID: job.RefID,
			Type:      usage.TypeImage,
			IP:        job.IP,
			UA:        job.UA,
		}
		refunded := false
		settled := false
		refund := func(code string, detail ...string) {
			rec.Status = usage.StatusFailed
			rec.ErrorCode = code
			if h.DAO != nil {
				msg := ""
				if len(detail) > 0 {
					msg = detail[0]
				}
				_ = h.DAO.MarkFailedWithMessage(context.Background(), job.TaskID, code, msg)
			}
			if refunded || settled || job.Cost == 0 {
				return
			}
			refunded = true
			if err := h.Billing.Refund(context.Background(), job.UserID, job.KeyID, job.Cost, job.RefID, "image async refund"); err != nil {
				logger.L().Error("billing refund async image",
					zap.Error(err), zap.String("ref", job.RefID), zap.String("task_id", job.TaskID))
			}
		}
		defer func() {
			if p := recover(); p != nil {
				logger.L().Error("async image runner panic",
					zap.Any("panic", p), zap.String("task_id", job.TaskID))
				if rec.Status != usage.StatusSuccess {
					refund("panic")
				}
			}
			rec.DurationMs = int(time.Since(job.StartAt).Milliseconds())
			if rec.Status == "" {
				rec.Status = usage.StatusFailed
			}
			if h.Usage != nil {
				h.Usage.Write(rec)
			}
		}()

		if h.Runner == nil {
			refund("not_configured")
			return
		}
		if job.Timeout <= 0 {
			job.Timeout = 7 * time.Minute
		}
		runCtx, cancel := context.WithTimeout(context.Background(), job.Timeout)
		defer cancel()

		res := h.Runner.Run(runCtx, image.RunOptions{
			TaskID:        job.TaskID,
			UserID:        job.UserID,
			KeyID:         job.KeyID,
			ModelID:       job.ModelID,
			UpstreamModel: job.UpstreamModel,
			Prompt:        job.Prompt,
			N:             job.N,
			MaxAttempts:   job.MaxAttempts,
			References:    job.Refs,
		})
		rec.AccountID = res.AccountID

		if res.Status != image.StatusSuccess {
			refund(ifEmpty(res.ErrorCode, "upstream_error"), res.ErrorMessage)
			return
		}

		if job.Cost > 0 {
			if err := h.Billing.Settle(context.Background(), job.UserID, job.KeyID, job.Cost, job.Cost, job.RefID, "image async settle"); err != nil {
				logger.L().Error("billing settle async image",
					zap.Error(err), zap.String("ref", job.RefID), zap.String("task_id", job.TaskID))
			} else {
				settled = true
			}
		} else {
			settled = true
		}

		rec.Status = usage.StatusSuccess
		rec.CreditCost = job.Cost
		rec.ImageCount = len(res.SignedURLs)
		if rec.ImageCount <= 0 {
			rec.ImageCount = job.N
		}
		if rec.ImageCount <= 0 {
			rec.ImageCount = 1
		}
		_ = h.Keys.DAO().TouchUsage(context.Background(), job.KeyID, job.IP, job.Cost)
		if h.DAO != nil {
			_ = h.DAO.UpdateCost(context.Background(), job.TaskID, job.Cost)
		}
	}()
}

// ImageTask GET /v1/images/tasks/:id。
func (h *ImagesHandler) ImageTask(c *gin.Context) {
	ak, ok := apikey.FromCtx(c)
	if !ok {
		openAIError(c, http.StatusUnauthorized, "missing_api_key", "缺少 API Key")
		return
	}
	id := c.Param("id")
	if id == "" {
		openAIError(c, http.StatusBadRequest, "invalid_request_error", "task id 不能为空")
		return
	}
	if h.DAO == nil {
		openAIError(c, http.StatusInternalServerError, "not_configured", "图片任务存储未初始化,请联系管理员")
		return
	}
	t, err := h.DAO.Get(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, image.ErrNotFound) {
			openAIError(c, http.StatusNotFound, "not_found", "任务不存在")
			return
		}
		openAIError(c, http.StatusInternalServerError, "internal_error", "查询任务失败:"+err.Error())
		return
	}
	if t.UserID != ak.UserID {
		openAIError(c, http.StatusNotFound, "not_found", "任务不存在")
		return
	}
	now := time.Now()
	if isStaleImageTask(t, now) {
		updated, err := h.DAO.MarkStaleFailed(c.Request.Context(), t.TaskID,
			now.Add(-imageTaskStaleAfter),
			image.ErrPollTimeout,
			"image task exceeded async runner timeout; worker may have exited before writing final status")
		if err != nil {
			openAIError(c, http.StatusInternalServerError, "internal_error", "更新超时任务失败:"+err.Error())
			return
		}
		if updated {
			t, err = h.DAO.Get(c.Request.Context(), id)
			if err != nil {
				openAIError(c, http.StatusInternalServerError, "internal_error", "查询任务失败:"+err.Error())
				return
			}
		}
	}

	urls := t.DecodeResultURLs()
	data := make([]ImageGenData, 0, len(urls))
	fileIDs := t.DecodeFileIDs()
	for i := range urls {
		d := ImageGenData{URL: BuildImageProxyURL(t.TaskID, i, ImageProxyTTL)}
		if i < len(fileIDs) {
			d.FileID = strings.TrimPrefix(fileIDs[i], "sed:")
		}
		data = append(data, d)
	}

	c.JSON(http.StatusOK, gin.H{
		"task_id":         t.TaskID,
		"status":          t.Status,
		"conversation_id": t.ConversationID,
		"created":         t.CreatedAt.Unix(),
		"finished_at":     nullableUnix(t.FinishedAt),
		"error":           t.Error,
		"credit_cost":     t.CreditCost,
		"data":            data,
	})
}

// handleChatAsImage 是 /v1/chat/completions 发现 model.type=image 时的转派点。
// 行为:
//   - 取最后一条 user message 作为 prompt
//   - 走完整图像链路(同 /v1/images/generations)
//   - 以 assistant message(含 markdown 图片链接)的 OpenAI chat 响应返回
//
// 这样前端只要调用一个端点 /v1/chat/completions,切换 model=gpt-image-2 就能出图。
func (h *ImagesHandler) handleChatAsImage(c *gin.Context, rec *usage.Log, ak *apikey.APIKey,
	m *modelpkg.Model, req *ChatCompletionsRequest, startAt time.Time) {
	rec.ModelID = m.ID
	rec.Type = usage.TypeImage

	prompt := extractLastUserPrompt(req.Messages)
	if strings.TrimSpace(prompt) == "" {
		rec.Status = usage.StatusFailed
		rec.ErrorCode = "invalid_request_error"
		openAIError(c, http.StatusBadRequest, "invalid_request_error",
			"图像模型需要用户消息作为 prompt,请检查 messages 内容")
		return
	}

	refID := uuid.NewString()

	// 倍率 + RPM
	ratio := 1.0
	rpmCap := ak.RPM
	if h.Groups != nil {
		if g, err := h.Groups.OfUser(c.Request.Context(), ak.UserID); err == nil && g != nil {
			ratio = g.Ratio
			if rpmCap == 0 {
				rpmCap = g.RPMLimit
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
	cost := billing.ComputeImageCost(m, 1, ratio)
	if cost > 0 {
		if err := h.Billing.PreDeduct(c.Request.Context(), ak.UserID, ak.ID, cost, refID, "chat->image prepay"); err != nil {
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
	}
	refunded := false
	refund := func(code string) {
		rec.Status = usage.StatusFailed
		rec.ErrorCode = code
		if refunded || cost == 0 {
			return
		}
		refunded = true
		_ = h.Billing.Refund(context.Background(), ak.UserID, ak.ID, cost, refID, "chat->image refund")
	}

	taskID := image.GenerateTaskID()
	if h.DAO != nil {
		_ = h.DAO.Create(c.Request.Context(), &image.Task{
			TaskID:          taskID,
			UserID:          ak.UserID,
			KeyID:           ak.ID,
			ModelID:         m.ID,
			Prompt:          prompt,
			N:               1,
			Size:            "1024x1024",
			Status:          image.StatusDispatched,
			EstimatedCredit: cost,
		})
	}

	runCtx, cancel := context.WithTimeout(c.Request.Context(), 7*time.Minute)
	defer cancel()

	res := h.Runner.Run(runCtx, image.RunOptions{
		TaskID:        taskID,
		UserID:        ak.UserID,
		KeyID:         ak.ID,
		ModelID:       m.ID,
		UpstreamModel: m.UpstreamModelSlug,
		Prompt:        maybeAppendClaritySuffix(prompt),
		N:             1,
		MaxAttempts:   2,
	})
	rec.AccountID = res.AccountID

	if res.Status != image.StatusSuccess {
		refund(ifEmpty(res.ErrorCode, "upstream_error"))
		httpStatus := http.StatusBadGateway
		if res.ErrorCode == image.ErrNoAccount || res.ErrorCode == image.ErrRateLimited {
			httpStatus = http.StatusServiceUnavailable
		}
		openAIError(c, httpStatus, ifEmpty(res.ErrorCode, "upstream_error"),
			localizeImageErr(res.ErrorCode, res.ErrorMessage))
		return
	}

	if cost > 0 {
		_ = h.Billing.Settle(context.Background(), ak.UserID, ak.ID, cost, cost, refID, "chat->image settle")
	}
	_ = h.Keys.DAO().TouchUsage(context.Background(), ak.ID, c.ClientIP(), cost)
	if h.DAO != nil {
		_ = h.DAO.UpdateCost(c.Request.Context(), taskID, cost)
	}

	rec.Status = usage.StatusSuccess
	rec.CreditCost = cost
	rec.DurationMs = int(time.Since(startAt).Milliseconds())
	// chat-as-image 单轮固定 N=1,这里也按 SignedURLs 兜底,避免 0 张统计漂移。
	rec.ImageCount = len(res.SignedURLs)
	if rec.ImageCount <= 0 {
		rec.ImageCount = 1
	}

	// 以 chat 响应返回(content 里内嵌 markdown 图片)。
	var sb strings.Builder
	for i := range res.SignedURLs {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(fmt.Sprintf("![generated](%s)", BuildImageProxyURL(taskID, i, ImageProxyTTL)))
	}
	resp := ChatCompletionResponse{
		ID:      "chatcmpl-" + uuid.NewString(),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   m.Slug,
		Choices: []ChatCompletionChoice{{
			Index: 0,
			Message: chatMsg{
				Role:    "assistant",
				Content: sb.String(),
			},
			FinishReason: "stop",
		}},
		Usage: ChatCompletionUsage{
			PromptTokens:     0,
			CompletionTokens: 0,
			TotalTokens:      0,
		},
	}
	c.JSON(http.StatusOK, resp)
}

// extractLastUserPrompt 从 messages 中拿最后一条 user 消息的 content。
func extractLastUserPrompt(msgs []chatMsg) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" && strings.TrimSpace(msgs[i].Content) != "" {
			return msgs[i].Content
		}
	}
	return ""
}

// --- helpers ---

func ifEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// localizeImageErr 把 runner 返回的英文错误码 + 原始 err.Error() 压成一段中文提示,
// 方便前端 / SDK 用户直接看懂。原始英文 message 作为后缀保留以便排障。
func localizeImageErr(code, raw string) string {
	var zh string
	switch code {
	case image.ErrNoAccount:
		zh = "No available accounts in the pool, please try again later"
	case image.ErrRateLimited:
		zh = "Upstream rate limited, please try again later"
	case image.ErrUpstreamRejected:
		zh = "Upstream rejected the image generation request"
	case image.ErrUnknown, "":
		zh = "Image generation failed"
	case "upstream_error":
		zh = "Upstream returned an error"
	default:
		zh = "Image generation failed (" + code + ")"
	}
	if raw != "" && raw != code {
		return zh + ":" + raw
	}
	return zh
}

func nullableUnix(t *time.Time) int64 {
	if t == nil || t.IsZero() {
		return 0
	}
	return t.Unix()
}

func isStaleImageTask(t *image.Task, now time.Time) bool {
	if t == nil {
		return false
	}
	switch t.Status {
	case image.StatusQueued, image.StatusDispatched, image.StatusRunning:
	default:
		return false
	}
	base := t.CreatedAt
	if t.StartedAt != nil && !t.StartedAt.IsZero() {
		base = *t.StartedAt
	}
	return now.Sub(base) > imageTaskStaleAfter
}

// 含这些关键字时,追加中英双约束让上游出字更清楚(迁移自 gen_image.py)。
var textHintKeywords = []string{
	"文字", "对话", "台词", "旁白", "标语", "字幕", "标题", "文案",
	"招牌", "横幅", "海报文字", "弹幕", "气泡", "字体",
	"text:", "caption", "subtitle", "title:", "label", "banner", "poster text",
}

const claritySuffix = "\n\nclean readable Chinese text, prioritize text clarity over image details"

// ImageEdits 实现 POST /v1/images/edits,严格按 OpenAI 规范接 multipart/form-data。
//
// 表单字段(与 OpenAI 官方一致):
//
//	image            (file)      单张主图,必填
//	image[]          (file)      多张,可重复(2025 起官方支持)
//	mask             (file)      可选,透明区域为编辑区;当前协议下直接一并上传(上游暂不区分)
//	prompt           (string)    必填
//	model            (string)    模型 slug,默认 gpt-image-2
//	n                (int)       默认 1
//	size             (string)    默认 1024x1024
//	response_format  (string)    url | b64_json,当前仅 url
//	user             (string)
//
// 实际走的上游协议和 /v1/images/generations + reference_images 完全相同。
// 行为等价于"把 multipart 文件读成字节 + prompt,交给 ImageGenerations 的主流程"。
func (h *ImagesHandler) ImageEdits(c *gin.Context) {
	startAt := time.Now()
	ak, ok := apikey.FromCtx(c)
	if !ok {
		openAIError(c, http.StatusUnauthorized, "missing_api_key", "缺少 API Key")
		return
	}

	// multipart 上限:单文件 20MB * 最多 4 张 + 冗余。
	if err := c.Request.ParseMultipartForm(int64(maxReferenceImageBytes) * int64(maxReferenceImages+1)); err != nil {
		openAIError(c, http.StatusBadRequest, "invalid_request_error", "解析 multipart 失败:"+err.Error())
		return
	}

	prompt := strings.TrimSpace(c.Request.FormValue("prompt"))
	if prompt == "" {
		openAIError(c, http.StatusBadRequest, "invalid_request_error", "prompt 不能为空")
		return
	}
	model := c.Request.FormValue("model")
	if model == "" {
		model = "gpt-image-2"
	}
	n := 1
	if s := c.Request.FormValue("n"); s != "" {
		if v, err := parseIntClamp(s, 1, 4); err == nil {
			n = v
		}
	}
	size := c.Request.FormValue("size")
	if size == "" {
		size = "1024x1024"
	}
	upscale := image.ValidateUpscale(c.Request.FormValue("upscale"))

	// 主图 + 可能的多张
	files, err := collectEditFiles(c.Request.MultipartForm)
	if err != nil {
		openAIError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if len(files) == 0 {
		openAIError(c, http.StatusBadRequest, "invalid_request_error", "至少需要上传一张 image 作为参考图")
		return
	}
	if len(files) > maxReferenceImages {
		openAIError(c, http.StatusBadRequest, "invalid_request_error",
			fmt.Sprintf("最多支持 %d 张参考图", maxReferenceImages))
		return
	}
	refs := make([]image.ReferenceImage, 0, len(files))
	for _, fh := range files {
		data, err := readMultipart(fh)
		if err != nil {
			openAIError(c, http.StatusBadRequest, "invalid_reference_image",
				fmt.Sprintf("读取 %q 失败:%s", fh.Filename, err.Error()))
			return
		}
		if len(data) == 0 {
			openAIError(c, http.StatusBadRequest, "invalid_reference_image",
				fmt.Sprintf("参考图 %q 为空", fh.Filename))
			return
		}
		if len(data) > maxReferenceImageBytes {
			openAIError(c, http.StatusBadRequest, "invalid_reference_image",
				fmt.Sprintf("参考图 %q 超过 %dMB 上限", fh.Filename, maxReferenceImageBytes/1024/1024))
			return
		}
		refs = append(refs, image.ReferenceImage{Data: data, FileName: filepath.Base(fh.Filename)})
	}

	// usage 记录
	refID := uuid.NewString()
	rec := &usage.Log{
		UserID:    ak.UserID,
		KeyID:     ak.ID,
		RequestID: refID,
		Type:      usage.TypeImage,
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

	if !ak.ModelAllowed(model) {
		fail("model_not_allowed")
		openAIError(c, http.StatusForbidden, "model_not_allowed",
			fmt.Sprintf("当前 API Key 无权调用模型 %q", model))
		return
	}
	m, err := h.Models.BySlug(c.Request.Context(), model)
	if err != nil || m == nil || !m.Enabled {
		fail("model_not_found")
		openAIError(c, http.StatusBadRequest, "model_not_found",
			fmt.Sprintf("模型 %q 不存在或已下架", model))
		return
	}
	if m.Type != modelpkg.TypeImage {
		fail("model_type_mismatch")
		openAIError(c, http.StatusBadRequest, "model_type_mismatch",
			fmt.Sprintf("模型 %q 不是图像模型,不能用于 /v1/images/edits", model))
		return
	}
	rec.ModelID = m.ID

	ratio := 1.0
	rpmCap := ak.RPM
	if h.Groups != nil {
		if g, err := h.Groups.OfUser(c.Request.Context(), ak.UserID); err == nil && g != nil {
			ratio = g.Ratio
			if rpmCap == 0 {
				rpmCap = g.RPMLimit
			}
		}
	}
	if h.Limiter != nil {
		if ok, _, err := h.Limiter.AllowRPM(c.Request.Context(), ak.ID, rpmCap); err == nil && !ok {
			fail("rate_limit_rpm")
			openAIError(c, http.StatusTooManyRequests, "rate_limit_rpm",
				"触发每分钟请求数限制 (RPM),请稍后再试")
			return
		}
	}

	cost := billing.ComputeImageCost(m, n, ratio)
	if cost > 0 {
		if err := h.Billing.PreDeduct(c.Request.Context(), ak.UserID, ak.ID, cost, refID, "image-edit prepay"); err != nil {
			if errors.Is(err, billing.ErrInsufficient) {
				fail("insufficient_balance")
				openAIError(c, http.StatusPaymentRequired, "insufficient_balance",
					"积分不足,请前往「账单与充值」充值后再试")
				return
			}
			fail("billing_error")
			openAIError(c, http.StatusInternalServerError, "billing_error", "计费异常:"+err.Error())
			return
		}
	}
	refunded := false
	refund := func(code string) {
		fail(code)
		if refunded || cost == 0 {
			return
		}
		refunded = true
		_ = h.Billing.Refund(context.Background(), ak.UserID, ak.ID, cost, refID, "image-edit refund")
	}

	taskID := image.GenerateTaskID()
	if h.DAO != nil {
		_ = h.DAO.Create(c.Request.Context(), &image.Task{
			TaskID:          taskID,
			UserID:          ak.UserID,
			KeyID:           ak.ID,
			ModelID:         m.ID,
			Prompt:          prompt,
			N:               n,
			Size:            size,
			Upscale:         upscale,
			Status:          image.StatusDispatched,
			EstimatedCredit: cost,
		})
	}

	runCtx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Minute)
	defer cancel()

	res := h.Runner.Run(runCtx, image.RunOptions{
		TaskID:        taskID,
		UserID:        ak.UserID,
		KeyID:         ak.ID,
		ModelID:       m.ID,
		UpstreamModel: m.UpstreamModelSlug,
		Prompt:        maybeAppendClaritySuffix(prompt),
		N:             n,
		MaxAttempts:   1, // 带参考图时只跑一次,避免重复上传
		References:    refs,
	})
	rec.AccountID = res.AccountID

	if res.Status != image.StatusSuccess {
		refund(ifEmpty(res.ErrorCode, "upstream_error"))
		httpStatus := http.StatusBadGateway
		if res.ErrorCode == image.ErrNoAccount || res.ErrorCode == image.ErrRateLimited {
			httpStatus = http.StatusServiceUnavailable
		}
		openAIError(c, httpStatus, ifEmpty(res.ErrorCode, "upstream_error"),
			localizeImageErr(res.ErrorCode, res.ErrorMessage))
		return
	}

	if cost > 0 {
		if err := h.Billing.Settle(context.Background(), ak.UserID, ak.ID, cost, cost, refID, "image-edit settle"); err != nil {
			logger.L().Error("billing settle image-edit", zap.Error(err), zap.String("ref", refID))
		}
	}
	_ = h.Keys.DAO().TouchUsage(context.Background(), ak.ID, c.ClientIP(), cost)

	rec.Status = usage.StatusSuccess
	rec.CreditCost = cost
	rec.ImageCount = len(res.SignedURLs)
	if rec.ImageCount <= 0 {
		rec.ImageCount = n
	}
	if rec.ImageCount <= 0 {
		rec.ImageCount = 1
	}
	if h.DAO != nil {
		_ = h.DAO.UpdateCost(c.Request.Context(), taskID, cost)
	}

	out := ImageGenResponse{
		Created: time.Now().Unix(),
		TaskID:  taskID,
		Data:    make([]ImageGenData, 0, len(res.SignedURLs)),
	}
	for i := range res.SignedURLs {
		d := ImageGenData{URL: BuildImageProxyURL(taskID, i, ImageProxyTTL)}
		if i < len(res.FileIDs) {
			d.FileID = strings.TrimPrefix(res.FileIDs[i], "sed:")
		}
		out.Data = append(out.Data, d)
	}
	c.JSON(http.StatusOK, out)
}

// collectEditFiles 把 multipart 里"可能作为参考图"的字段一次性收拢。
// 兼容 OpenAI 的几种写法:
//   - image      : 单文件
//   - image[]    : 多文件
//   - mask       : 可选,按参考图一并喂给上游(上游暂不区分 mask)
func collectEditFiles(form *multipart.Form) ([]*multipart.FileHeader, error) {
	if form == nil {
		return nil, errors.New("empty multipart form")
	}
	var out []*multipart.FileHeader
	seen := map[string]bool{}
	add := func(fhs []*multipart.FileHeader) {
		for _, fh := range fhs {
			if fh == nil {
				continue
			}
			key := fh.Filename + "|" + fmt.Sprint(fh.Size)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, fh)
		}
	}
	for _, key := range []string{"image", "image[]", "images", "images[]", "mask"} {
		if fhs := form.File[key]; len(fhs) > 0 {
			add(fhs)
		}
	}
	// 也兼容 image_1 / image_2 / ... 的写法
	for k, fhs := range form.File {
		if strings.HasPrefix(k, "image_") {
			add(fhs)
		}
	}
	return out, nil
}

func readMultipart(fh *multipart.FileHeader) ([]byte, error) {
	f, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

// decodeReferenceInputs 把 JSON 里 reference_images(url/data-url/base64 混合)下载/解码成字节。
// 超出条数上限直接报错;单张尺寸上限 maxReferenceImageBytes。
func decodeReferenceInputs(ctx context.Context, inputs []string) ([]image.ReferenceImage, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	if len(inputs) > maxReferenceImages {
		return nil, fmt.Errorf("最多支持 %d 张参考图", maxReferenceImages)
	}
	out := make([]image.ReferenceImage, 0, len(inputs))
	for i, s := range inputs {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, fmt.Errorf("第 %d 张参考图为空", i+1)
		}
		data, name, err := fetchReferenceBytes(ctx, s)
		if err != nil {
			return nil, fmt.Errorf("第 %d 张参考图:%w", i+1, err)
		}
		if len(data) == 0 {
			return nil, fmt.Errorf("第 %d 张参考图解码后为空", i+1)
		}
		if len(data) > maxReferenceImageBytes {
			return nil, fmt.Errorf("第 %d 张参考图超过 %dMB 上限", i+1, maxReferenceImageBytes/1024/1024)
		}
		out = append(out, image.ReferenceImage{Data: data, FileName: name})
	}
	return out, nil
}

// fetchReferenceBytes 支持 http(s) / data URL / 裸 base64 三种输入。
func fetchReferenceBytes(ctx context.Context, s string) ([]byte, string, error) {
	low := strings.ToLower(s)
	switch {
	case strings.HasPrefix(low, "data:"):
		// data:<mime>[;base64],<payload>
		comma := strings.IndexByte(s, ',')
		if comma < 0 {
			return nil, "", errors.New("无效 data URL")
		}
		meta := s[5:comma]
		payload := s[comma+1:]
		if strings.Contains(strings.ToLower(meta), ";base64") {
			b, err := base64.StdEncoding.DecodeString(payload)
			if err != nil {
				// 兼容 URL-safe
				if b2, err2 := base64.URLEncoding.DecodeString(payload); err2 == nil {
					b = b2
				} else {
					return nil, "", fmt.Errorf("base64 解码失败:%w", err)
				}
			}
			return b, "", nil
		}
		return []byte(payload), "", nil
	case strings.HasPrefix(low, "http://"), strings.HasPrefix(low, "https://"):
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, s, nil)
		if err != nil {
			return nil, "", err
		}
		// 15s 基本能覆盖 OSS / CDN / presigned URL
		hc := &http.Client{Timeout: 15 * time.Second}
		res, err := hc.Do(req)
		if err != nil {
			return nil, "", err
		}
		defer res.Body.Close()
		if res.StatusCode >= 400 {
			return nil, "", fmt.Errorf("下载失败 HTTP %d", res.StatusCode)
		}
		body, err := io.ReadAll(io.LimitReader(res.Body, int64(maxReferenceImageBytes)+1))
		if err != nil {
			return nil, "", err
		}
		name := filepath.Base(req.URL.Path)
		return body, name, nil
	default:
		// 当成裸 base64 处理
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			if b2, err2 := base64.URLEncoding.DecodeString(s); err2 == nil {
				return b2, "", nil
			}
			return nil, "", fmt.Errorf("既非 URL 也非可解析的 base64:%w", err)
		}
		return b, "", nil
	}
}

func parseIntClamp(s string, min, max int) (int, error) {
	var v int
	if _, err := fmt.Sscanf(s, "%d", &v); err != nil {
		return 0, err
	}
	if v < min {
		v = min
	}
	if v > max {
		v = max
	}
	return v, nil
}

func maybeAppendClaritySuffix(prompt string) string {
	lower := strings.ToLower(prompt)
	need := false
	for _, kw := range textHintKeywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			need = true
			break
		}
	}
	if !need {
		// 检测中文/英文引号内容 ≥ 2 个字
		for _, pair := range [][2]string{
			{"\"", "\""}, {"'", "'"},
			{"“", "”"}, {"‘", "’"},
			{"「", "」"}, {"『", "』"},
		} {
			if idx := strings.Index(prompt, pair[0]); idx >= 0 {
				rest := prompt[idx+len(pair[0]):]
				if end := strings.Index(rest, pair[1]); end >= 2 {
					need = true
					break
				}
			}
		}
	}
	if need && !strings.Contains(prompt, strings.TrimSpace(claritySuffix)) {
		return prompt + claritySuffix
	}
	return prompt
}
