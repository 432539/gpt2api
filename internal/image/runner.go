package image

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/432539/gpt2api/internal/scheduler"
	"github.com/432539/gpt2api/internal/upstream/chatgpt"
	"github.com/432539/gpt2api/pkg/logger"
)

// Runner 单次/多次生图的执行器。封装完整的 chatgpt.com 协议链路:
//
//	ChatRequirements → PrepareFConversation → StreamFConversation (SSE) →
//	ParseImageSSE → (需要时) PollConversationForImages → ImageDownloadURL
//
// IMG2 已正式上线,不再做"灰度命中判定 / preview_only 换账号重试"这些节流操作,
// 拿到任意 file-service / sediment 引用即算成功,以速度和效率优先。
type Runner struct {
	sched   *scheduler.Scheduler
	dao     *DAO
	solver  chatgpt.TurnstileSolver
}

// NewRunner 构造 Runner。
func NewRunner(sched *scheduler.Scheduler, dao *DAO, solver chatgpt.TurnstileSolver) *Runner {
	return &Runner{sched: sched, dao: dao, solver: solver}
}

// ReferenceImage 是图生图/编辑的一张参考图输入。
// 只需要提供原始字节 + 可选的文件名,Runner 会在运行时调用 chatgpt Client 上传。
type ReferenceImage struct {
	Data     []byte
	FileName string // 可选,未填时按长度 + 嗅探扩展名生成
}

// RunOptions 是单次生图的输入。
type RunOptions struct {
	TaskID            string
	UserID            uint64
	KeyID             uint64
	ModelID           uint64
	UpstreamModel     string           // 默认 "auto"(由上游根据 system_hints 挑选图像模型)
	Prompt            string
	N                 int              // 期望返回的图片张数;够数 Poll 就立即返回(速度优先)
	MaxAttempts       int              // 跨账号重试次数,仅用于无账号/限流等硬错误,默认 1
	PerAttemptTimeout time.Duration    // 单次尝试总超时,默认 6min(覆盖 SSE + PollMaxWait + 缓冲)
	PollMaxWait       time.Duration    // SSE 没直出时,轮询 conversation 的最长等待,默认 300s
	References        []ReferenceImage // 图生图/编辑:参考图
}

// RunResult 是单次生图的输出。
type RunResult struct {
	Status         string   // success / failed
	ConversationID string
	AccountID      uint64
	FileIDs        []string // chatgpt.com 侧的原始 ref("sed:" 前缀表示 sediment)
	SignedURLs     []string // 直接可访问的签名 URL(15 分钟有效)
	ContentTypes   []string
	ErrorCode      string
	ErrorMessage   string
	Attempts       int // 跨账号尝试次数(runOnce 次数)
	DurationMs     int64
}

// Run 执行生图。会同步阻塞直到完成/失败;调用方自行做超时控制(传 ctx)。
func (r *Runner) Run(ctx context.Context, opt RunOptions) *RunResult {
	start := time.Now()
	if opt.MaxAttempts <= 0 {
		// 默认只跑 1 次,不为"没命中"做跨账号重试。
		// 仅当首轮因为没调度到账号 / 账号被硬限流时,才会用 MaxAttempts>1 做 1 次换账号重试。
		opt.MaxAttempts = 1
	}
	if opt.PerAttemptTimeout <= 0 {
		opt.PerAttemptTimeout = 10 * time.Minute
	}
	if opt.PollMaxWait <= 0 {
		opt.PollMaxWait = 480 * time.Second
	}
	if opt.UpstreamModel == "" {
		// 对齐浏览器抓包 + 参考实现:图像走 f/conversation 时 model 字段和
		// 普通 chat 一致用 "auto",通过 system_hints=["picture_v2"] 让上游知道
		// 这是图像任务。硬写 "gpt-5-3" 在免费/新账号上会直接 404。
		opt.UpstreamModel = "auto"
	}
	if opt.N <= 0 {
		opt.N = 1
	}

	result := &RunResult{Status: StatusFailed, ErrorCode: ErrUnknown}

	// 仅当有 DAO 和 taskID 时才落库
	if r.dao != nil && opt.TaskID != "" {
		_ = r.dao.MarkRunning(ctx, opt.TaskID, 0)
	}

	for attempt := 1; attempt <= opt.MaxAttempts; attempt++ {
		result.Attempts = attempt
		if err := ctx.Err(); err != nil {
			result.ErrorCode = ErrUnknown
			result.ErrorMessage = err.Error()
			break
		}

		attemptCtx, cancel := context.WithTimeout(ctx, opt.PerAttemptTimeout)
		ok, status, err := r.runOnce(attemptCtx, opt, result)
		cancel()

		if ok {
			result.Status = StatusSuccess
			result.ErrorCode = ""
			result.ErrorMessage = ""
			break
		}
		if err != nil {
			result.ErrorMessage = err.Error()
		}
		result.ErrorCode = status

		// 仅对"账号级硬错误"做一次跨账号重试:限流 / 无账号 / 鉴权失败。
		// 其他错误(poll 超时 / 上游 5xx / 网络错)直接抛给用户,不再悄悄吞掉时间。
		if attempt >= opt.MaxAttempts {
			break
		}
		if status != ErrRateLimited && status != ErrNoAccount && status != ErrAuthRequired {
			break
		}
		logger.L().Info("image runner retry with another account",
			zap.String("task_id", opt.TaskID),
			zap.String("reason", status),
			zap.Int("attempt", attempt))
	}

	result.DurationMs = time.Since(start).Milliseconds()

	// 落库
	if r.dao != nil && opt.TaskID != "" {
		if result.Status == StatusSuccess {
			_ = r.dao.MarkSuccess(ctx, opt.TaskID, result.ConversationID,
				result.FileIDs, result.SignedURLs, 0 /* credit_cost 由网关负责写 */)
		} else {
			_ = r.dao.MarkFailed(ctx, opt.TaskID, result.ErrorCode)
		}
	}
	return result
}

// runOnce 一次完整的尝试。返回 (ok, errorCode, err)。
// result 会被就地更新(ConversationID / FileIDs / SignedURLs / AccountID 等)。
func (r *Runner) runOnce(ctx context.Context, opt RunOptions, result *RunResult) (bool, string, error) {
	// 1) 调度账号
	lease, err := r.sched.Dispatch(ctx, "image")
	if err != nil {
		if errors.Is(err, scheduler.ErrNoAvailable) {
			return false, ErrNoAccount, err
		}
		return false, ErrUnknown, err
	}
	defer func() {
		_ = lease.Release(context.Background())
	}()
	result.AccountID = lease.Account.ID
	// 立刻把 account_id 写回 image_tasks,供后续图片代理端点按 task_id 解出 AT。
	// MarkRunning 在 status=running 时 WHERE 不命中,所以用专门的 SetAccount。
	if r.dao != nil && opt.TaskID != "" {
		_ = r.dao.SetAccount(ctx, opt.TaskID, lease.Account.ID)
	}

	// 2) 构造上游 client
	cli, err := chatgpt.New(chatgpt.Options{
		AuthToken:       lease.AuthToken,
		DeviceID:        lease.DeviceID,
		SessionID:       lease.SessionID,
		ProxyURL:        lease.ProxyURL,
		Cookies:         "", // 目前不从 oai_account_cookies 加载,后续 M3+ 再做
		TurnstileSolver: r.solver,
	})
	if err != nil {
		return false, ErrUnknown, fmt.Errorf("chatgpt client: %w", err)
	}

	// 3) ChatRequirements + POW(新两步 sentinel 流程,solver 未配置时内部自动
	// 回退到单步接口)
	cr, err := cli.ChatRequirementsV2(ctx)
	if err != nil {
		return false, r.classifyUpstream(err), err
	}
	var proofToken string
	if cr.Proofofwork.Required {
		proofCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		ch := make(chan string, 1)
		go func() { ch <- cr.SolveProof(chatgpt.DefaultUserAgent) }()
		select {
		case <-proofCtx.Done():
			cancel()
			r.sched.MarkWarned(context.Background(), lease.Account.ID)
			return false, ErrPOWTimeout, proofCtx.Err()
		case proofToken = <-ch:
			cancel()
		}
		if proofToken == "" {
			r.sched.MarkWarned(context.Background(), lease.Account.ID)
			return false, ErrPOWFailed, errors.New("pow solver returned empty")
		}
	}
	// Turnstile 是"建议性"信号:即使服务端声明 required,只要 chat_token + proof_token
	// 齐全,绝大多数账号的 f/conversation 仍然会正常下发图片结果。与 chat 流程(gateway/chat.go)
	// 保持一致——只打 warn,不阻断;真正拿不到 IMG2 终稿时由后续 poll 逻辑判定为失败。
	if cr.Turnstile.Required {
		logger.L().Warn("image turnstile required, continue anyway",
			zap.Uint64("account_id", lease.Account.ID))
	}

	// 4) 不再调用 /backend-api/conversation/init:
	// 浏览器实测路径是 prepare → chat-requirements → f/conversation 三步,init 是
	// 过时/冗余调用,在免费账号上还会返回 404 让整条链路 fail。system_hints=picture_v2
	// 会通过 f/conversation 的 payload 字段传达。

	// 4.5) 图生图:上传参考图。任何一张失败都直接整体 fail(上游后续会对不上 attachment)。
	var refs []*chatgpt.UploadedFile
	if len(opt.References) > 0 {
		for idx, r0 := range opt.References {
			upCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
			up, err := cli.UploadFile(upCtx, r0.Data, r0.FileName)
			cancel()
			if err != nil {
				logger.L().Warn("image runner upload reference failed",
					zap.Int("idx", idx), zap.Error(err))
				if ue, ok := err.(*chatgpt.UpstreamError); ok && ue.IsRateLimited() {
					r.sched.MarkRateLimited(context.Background(), lease.Account.ID)
					return false, ErrRateLimited, err
				}
				return false, ErrUpstream, fmt.Errorf("upload reference %d: %w", idx, err)
			}
			refs = append(refs, up)
		}
		logger.L().Info("image runner references uploaded",
			zap.String("task_id", opt.TaskID), zap.Int("count", len(refs)))
	}

	// 注意:新会话不要本地生成 conversation_id,上游会 404。
	// 真正的 conv_id 由 SSE 的 resume_conversation_token / sseResult.ConversationID 返回。
	var convID string
	parentID := uuid.NewString()
	messageID := uuid.NewString()

	// 统一把 model 强制为 "auto":对齐参考实现(只通过 system_hints=["picture_v2"]
	// 区分图像任务),避免 chatgpt-freeaccount / chatgpt-paid 之间的 model slug 差异。
	upstreamModel := "auto"
	if opt.UpstreamModel != "" && opt.UpstreamModel != "auto" && !cr.IsFreeAccount() {
		// 付费账号如果明确传了 upstream slug 且不是 auto,可以尊重用户传入
		// (但我们现有模型库没有 image 专用 slug,保留扩展点)
		upstreamModel = opt.UpstreamModel
	} else if cr.IsFreeAccount() && opt.UpstreamModel != "" && opt.UpstreamModel != "auto" {
		logger.L().Warn("image: free account requesting premium model, downgrade to auto",
			zap.Uint64("account_id", lease.Account.ID),
			zap.String("requested_model", opt.UpstreamModel))
	}

	// 5) 单轮 picture_v2:SSE 里直接给图就走 SSE 结果,没给就短轮询补一下。
	// IMG2 已正式上线,不再区分"终稿 / 预览",拿到就用,追求速度。
	convOpt := chatgpt.ImageConvOpts{
		Prompt:        opt.Prompt,
		UpstreamModel: upstreamModel,
		ConvID:        convID,
		ParentMsgID:   parentID,
		MessageID:     messageID,
		ChatToken:     cr.Token,
		ProofToken:    proofToken,
		References:    refs,
	}

	// Prepare(conduit_token;拿不到也能降级继续)
	if ct, err := cli.PrepareFConversation(ctx, convOpt); err == nil {
		convOpt.ConduitToken = ct
	} else if ue, ok := err.(*chatgpt.UpstreamError); ok && ue.IsRateLimited() {
		r.sched.MarkRateLimited(context.Background(), lease.Account.ID)
		return false, ErrRateLimited, err
	}

	// f/conversation SSE
	stream, err := cli.StreamFConversation(ctx, convOpt)
	if err != nil {
		code := r.classifyUpstream(err)
		if code == ErrRateLimited {
			r.sched.MarkRateLimited(context.Background(), lease.Account.ID)
		}
		return false, code, err
	}
	sseResult := chatgpt.ParseImageSSE(stream)
	if sseResult.ConversationID != "" {
		convID = sseResult.ConversationID
		result.ConversationID = convID
	}

	logger.L().Info("image runner SSE parsed",
		zap.String("task_id", opt.TaskID),
		zap.Uint64("account_id", lease.Account.ID),
		zap.String("conv_id", convID),
		zap.String("finish_type", sseResult.FinishType),
		zap.String("image_gen_task_id", sseResult.ImageGenTaskID),
		zap.Int("sse_fids", len(sseResult.FileIDs)),
		zap.Strings("sse_fids_list", sseResult.FileIDs),
		zap.Int("sse_sids", len(sseResult.SedimentIDs)),
		zap.Strings("sse_sids_list", sseResult.SedimentIDs),
	)

	// 聚合 SSE 阶段的所有引用:file-service 优先,sediment 补位
	var fileRefs []string
	fileRefs = append(fileRefs, sseResult.FileIDs...)
	for _, s := range sseResult.SedimentIDs {
		fileRefs = append(fileRefs, "sed:"+s)
	}

	// SSE 已经把期望数量的图带回来了 → 直接下载,跳过 Poll,省时间
	if len(fileRefs) >= opt.N {
		logger.L().Info("image runner enough refs from SSE, skip polling",
			zap.String("task_id", opt.TaskID),
			zap.Uint64("account_id", lease.Account.ID),
			zap.String("conv_id", convID),
			zap.Int("refs", len(fileRefs)),
			zap.Strings("refs_list", fileRefs),
		)
	} else {
		// SSE 没给够(常见于 IMG2 只走 tool 消息场景)→ 短轮询补齐。
		// 单轮新会话,不需要 baseline:conversation 里出现的每条 image_gen tool 消息
		// 都是本次请求的产物。
		pollOpt := chatgpt.PollOpts{
			ExpectedN: opt.N,
			MaxWait:   opt.PollMaxWait,
		}
		status, fids, sids := cli.PollConversationForImages(ctx, convID, pollOpt)
		logger.L().Info("image runner poll done",
			zap.String("task_id", opt.TaskID),
			zap.Uint64("account_id", lease.Account.ID),
			zap.String("conv_id", convID),
			zap.String("poll_status", string(status)),
			zap.Int("poll_fids", len(fids)),
			zap.Strings("poll_fids_list", fids),
			zap.Int("poll_sids", len(sids)),
			zap.Strings("poll_sids_list", sids),
		)
		switch status {
		case chatgpt.PollStatusSuccess:
			// 去重合并:SSE 捕获的 sediment 可能在 mapping 里再被 Poll 扫一次
			seen := make(map[string]struct{}, len(fileRefs))
			for _, r := range fileRefs {
				seen[r] = struct{}{}
			}
			for _, f := range fids {
				if _, ok := seen[f]; ok {
					continue
				}
				seen[f] = struct{}{}
				fileRefs = append(fileRefs, f)
			}
			for _, s := range sids {
				key := "sed:" + s
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				fileRefs = append(fileRefs, key)
			}
		case chatgpt.PollStatusTimeout:
			return false, ErrPollTimeout, errors.New("poll timeout without any image")
		default:
			return false, ErrUpstream, errors.New("poll error")
		}
	}

	if len(fileRefs) == 0 {
		return false, ErrUpstream, errors.New("no image ref produced")
	}

	// 6) 对每个 ref 取签名 URL
	var signedURLs []string
	var contentTypes []string
	for _, ref := range fileRefs {
		url, err := cli.ImageDownloadURL(ctx, convID, ref)
		if err != nil {
			logger.L().Warn("image runner download url failed",
				zap.String("ref", ref), zap.Error(err))
			continue
		}
		signedURLs = append(signedURLs, url)
		contentTypes = append(contentTypes, "image/png")
	}
	if len(signedURLs) == 0 {
		return false, ErrDownload, errors.New("all download urls failed")
	}

	logger.L().Info("image runner result summary",
		zap.String("task_id", opt.TaskID),
		zap.Uint64("account_id", lease.Account.ID),
		zap.String("conv_id", convID),
		zap.Int("refs", len(fileRefs)),
		zap.Strings("refs_list", fileRefs),
		zap.Int("signed_count", len(signedURLs)),
	)

	result.FileIDs = fileRefs
	result.SignedURLs = signedURLs
	result.ContentTypes = contentTypes
	return true, "", nil
}

// classifyUpstream 把上游错误转成内部 error code。
func (r *Runner) classifyUpstream(err error) string {
	if err == nil {
		return ""
	}
	var ue *chatgpt.UpstreamError
	if errors.As(err, &ue) {
		if ue.IsRateLimited() {
			return ErrRateLimited
		}
		if ue.IsUnauthorized() {
			return ErrAuthRequired
		}
		return ErrUpstream
	}
	if strings.Contains(err.Error(), "deadline exceeded") {
		return ErrPollTimeout
	}
	return ErrUpstream
}

// GenerateTaskID 生成对外 task_id。
func GenerateTaskID() string {
	return "img_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:24]
}
