package image

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/432539/gpt2api/internal/scheduler"
	"github.com/432539/gpt2api/internal/upstream/chatgpt"
	"github.com/432539/gpt2api/pkg/logger"
)

// QuotaDecrementor 允许 Runner 在生图成功后立即扣减账号剩余额度,
// 无需等待下一次后台探测即可在前端看到正确数字。
type QuotaDecrementor interface {
	DecrQuota(ctx context.Context, accountID uint64, n int) error
}

// Runner 单次/多次生图的执行器。封装完整的 chatgpt.com 协议链路:
//
//	ChatRequirements → PrepareFConversation → StreamFConversation (SSE) →
//	ParseImageSSE → (需要时) PollConversationForImages → ImageDownloadURL
//
// IMG2 已正式上线,不再做"灰度命中判定 / preview_only 换账号重试"这些节流操作,
// 拿到任意 file-service / sediment 引用即算成功,以速度和效率优先。
type Runner struct {
	sched     *scheduler.Scheduler
	dao       *DAO
	quotaDecr QuotaDecrementor // 生图成功后立即扣减账号额度(可空,空时跳过)
}

// NewRunner 构造 Runner。
func NewRunner(sched *scheduler.Scheduler, dao *DAO) *Runner {
	return &Runner{sched: sched, dao: dao}
}

// SetQuotaDecrementor 注入额度扣减器。
func (r *Runner) SetQuotaDecrementor(qd QuotaDecrementor) { r.quotaDecr = qd }

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
	UpstreamModel     string // 默认 "auto"(由上游根据 system_hints 挑选图像模型)
	Prompt            string
	N                 int              // 期望返回的图片张数;够数 Poll 就立即返回(速度优先)
	MaxAttempts       int              // 跨账号重试次数,仅用于无账号/限流等硬错误,默认 1
	PerAttemptTimeout time.Duration    // 单次尝试总超时,默认 6min(覆盖 SSE + PollMaxWait + 缓冲)
	PollMaxWait       time.Duration    // SSE 没直出时,轮询 conversation 的最长等待,默认 300s
	References        []ReferenceImage // 图生图/编辑:参考图
}

// RunResult 是单次生图的输出。
type RunResult struct {
	Status         string // success / failed
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
//
// N > 1 时并发启动 N 个独立 goroutine,每个各自走完整链路出 1 张图,
// 最终合并结果——比向单一会话请求 N 张快得多(ChatGPT f/conversation 每轮只产 1 张)。
func (r *Runner) Run(ctx context.Context, opt RunOptions) *RunResult {
	start := time.Now()
	if opt.MaxAttempts <= 0 {
		opt.MaxAttempts = 1
	}
	if opt.PerAttemptTimeout <= 0 {
		opt.PerAttemptTimeout = 6 * time.Minute
	}
	if opt.PollMaxWait <= 0 {
		opt.PollMaxWait = 300 * time.Second
	}
	if opt.UpstreamModel == "" {
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

	if opt.N > 1 {
		// 并发模式:N 个 goroutine 各独立出 1 张
		r.runParallel(ctx, opt, start, result)
	} else {
		// 串行模式(原逻辑):带跨账号重试
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
				result.ErrorMessage = runnerErrorMessage(err)
			}
			result.ErrorCode = status

			if attempt >= opt.MaxAttempts {
				break
			}
			retryable := status == ErrRateLimited || status == ErrNoAccount ||
				status == ErrAuthRequired || status == ErrNetworkTransient
			if !retryable {
				break
			}
			logger.L().Info("image runner retry with another account",
				zap.String("task_id", opt.TaskID),
				zap.String("reason", status),
				zap.Int("attempt", attempt))
		}
	}

	result.DurationMs = time.Since(start).Milliseconds()

	// 落库
	if r.dao != nil && opt.TaskID != "" {
		if result.Status == StatusSuccess {
			_ = r.dao.MarkSuccess(ctx, opt.TaskID, result.ConversationID,
				result.FileIDs, result.SignedURLs, 0 /* credit_cost 由网关负责写 */)
			if r.quotaDecr != nil && result.AccountID > 0 {
				n := len(result.FileIDs)
				if n == 0 {
					n = opt.N
				}
				_ = r.quotaDecr.DecrQuota(context.Background(), result.AccountID, n)
			}
		} else {
			_ = r.dao.MarkFailedWithMessage(ctx, opt.TaskID, result.ErrorCode, result.ErrorMessage)
		}
	}
	return result
}

// runParallel 并发启动 opt.N 个独立请求,每个各出 1 张图,最终合并到 result。
// 只要有 ≥1 张成功就算整体成功;全部失败才返回失败。
// 各 goroutine 不写 DAO(TaskID 置空),写库由外层 Run 统一完成。
func (r *Runner) runParallel(ctx context.Context, opt RunOptions, start time.Time, result *RunResult) {
	type subResult struct {
		ok         bool
		fileIDs    []string
		signedURLs []string
		convID     string
		accountID  uint64
		errCode    string
		errMsg     string
	}

	n := opt.N
	ch := make(chan subResult, n)

	// 子任务:单张、不写 DAO
	subOpt := opt
	subOpt.N = 1
	subOpt.TaskID = "" // 禁用 DAO,避免多 goroutine 互相覆盖

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sub := &RunResult{Status: StatusFailed, ErrorCode: ErrUnknown}
			attemptCtx, cancel := context.WithTimeout(ctx, opt.PerAttemptTimeout)
			defer cancel()
			ok, status, err := r.runOnce(attemptCtx, subOpt, sub)
			msg := ""
			if err != nil {
				msg = runnerErrorMessage(err)
			}
			if !ok && status == "" {
				status = sub.ErrorCode
			}
			ch <- subResult{
				ok:         ok,
				fileIDs:    sub.FileIDs,
				signedURLs: sub.SignedURLs,
				convID:     sub.ConversationID,
				accountID:  sub.AccountID,
				errCode:    status,
				errMsg:     msg,
			}
		}()
	}

	// 等待全部完成后关闭 channel
	go func() { wg.Wait(); close(ch) }()

	var (
		successCount int
		lastErrCode  string
		lastErrMsg   string
	)
	for sr := range ch {
		if sr.ok {
			successCount++
			result.FileIDs = append(result.FileIDs, sr.fileIDs...)
			result.SignedURLs = append(result.SignedURLs, sr.signedURLs...)
			if result.ConversationID == "" {
				result.ConversationID = sr.convID
			}
			if result.AccountID == 0 {
				result.AccountID = sr.accountID
			}
		} else {
			lastErrCode = sr.errCode
			lastErrMsg = sr.errMsg
		}
	}
	result.Attempts = n

	if successCount > 0 {
		result.Status = StatusSuccess
		result.ErrorCode = ""
		result.ErrorMessage = ""
		logger.L().Info("image runner parallel done",
			zap.String("task_id", opt.TaskID),
			zap.Int("requested", n),
			zap.Int("succeeded", successCount),
			zap.Int("got_images", len(result.FileIDs)),
		)
	} else {
		result.ErrorCode = lastErrCode
		result.ErrorMessage = lastErrMsg
		logger.L().Warn("image runner parallel all failed",
			zap.String("task_id", opt.TaskID),
			zap.Int("requested", n),
			zap.String("last_err", lastErrCode),
		)
	}
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
		AuthToken: lease.AuthToken,
		DeviceID:  lease.DeviceID,
		SessionID: lease.SessionID,
		ProxyURL:  lease.ProxyURL,
		Cookies:   "", // 目前不从 oai_account_cookies 加载,后续 M3+ 再做
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
	//
	// 超时预算:UploadFile 内部对网络层瞬时错误重试 4 次(0+0.5+1.5+3=5s 退避总和),
	// 三步串行各自最多耗时 30s 上下,叠加重试时单张图最坏 ≈ 3*(30s+5s) = 105s。
	// 给 180s 留一点余量;如果还是超时,说明根本不是瞬时问题,fail-fast 也合理。
	var refs []*chatgpt.UploadedFile
	if len(opt.References) > 0 {
		for idx, r0 := range opt.References {
			upCtx, cancel := context.WithTimeout(ctx, 180*time.Second)
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
	// 区分图像任务)。
	// 注意:免费账号(persona=chatgpt-freeaccount)也可以生成图片,只要 daily_image_quota > 0。
	// 不再按 persona 拒绝请求;persona 仅做日志记录。
	upstreamModel := "auto"
	if opt.UpstreamModel != "" && opt.UpstreamModel != "auto" {
		if cr.IsFreeAccount() {
			logger.L().Info("image: free account, force upstream model to auto",
				zap.Uint64("account_id", lease.Account.ID),
				zap.String("requested_model", opt.UpstreamModel))
		} else {
			upstreamModel = opt.UpstreamModel
		}
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

	// 图生图:ParseImageSSE 用正则扫整段 SSE,会把 user 消息里「参考图」的
	// file-service:// 指纹和 assistant 的出图一起扫进 FileIDs,且参考图往往先出现。
	// 必须在 len(fileRefs) >= opt.N 之前剔除,否则 n=1 时会误把上传图当结果图,
	// 进而跳过 Poll,最终 /p/img/.../0 下载到的是用户上传的参考图。
	if len(refs) > 0 {
		refSet := referenceUploadFileIDSet(refs)
		if len(refSet) > 0 {
			n0 := len(fileRefs)
			fileRefs = filterOutReferenceFileIDs(fileRefs, refSet)
			if n0 != len(fileRefs) {
				logger.L().Info("image runner stripped reference file_ids from SSE-captured refs",
					zap.String("task_id", opt.TaskID),
					zap.Int("before", n0), zap.Int("after", len(fileRefs)))
			}
		}
	}
	if len(fileRefs) == 0 && isImageRejectionMessage(sseResult.AssistantText) {
		return false, ErrUpstreamRejected, errors.New(sseResult.AssistantText)
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
		status, fids, sids, assistantText := cli.PollConversationForImages(ctx, convID, pollOpt)
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
			if msg := firstFilled(assistantText, sseResult.AssistantText); msg != "" {
				return false, assistantFailureCode(msg, ErrPollTimeout), errors.New(msg)
			}
			return false, ErrPollTimeout, errors.New("poll timeout without any image")
		default:
			if msg := firstFilled(assistantText, sseResult.AssistantText); msg != "" {
				return false, assistantFailureCode(msg, ErrUpstream), errors.New(msg)
			}
			return false, ErrUpstream, errors.New("poll error")
		}
	}

	if len(fileRefs) == 0 {
		if msg := strings.TrimSpace(sseResult.AssistantText); msg != "" {
			return false, assistantFailureCode(msg, ErrUpstream), errors.New(msg)
		}
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

// referenceUploadFileIDSet 收集图生图时 UploadFile 返回的 file_id(纯 file-service id,无 sed: 前缀)。
func referenceUploadFileIDSet(refs []*chatgpt.UploadedFile) map[string]struct{} {
	out := make(map[string]struct{})
	for _, u := range refs {
		if u == nil {
			continue
		}
		id := strings.TrimSpace(u.FileID)
		if id == "" {
			continue
		}
		out[strings.TrimPrefix(id, "sed:")] = struct{}{}
	}
	return out
}

// filterOutReferenceFileIDs 从合并后的 fileRefs 中移除与「用户参考上传」同 id 的 file-service 条目,
// 保留 sediment(sed:...) —— 参考图只走 file-service,不会以 sed: 形式出现在同一列表里和生成图冲突。
func filterOutReferenceFileIDs(fileRefs []string, refSet map[string]struct{}) []string {
	if len(refSet) == 0 {
		return fileRefs
	}
	out := make([]string, 0, len(fileRefs))
	for _, ref := range fileRefs {
		if strings.HasPrefix(ref, "sed:") {
			out = append(out, ref)
			continue
		}
		if _, skip := refSet[ref]; skip {
			continue
		}
		out = append(out, ref)
	}
	return out
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
	msg := err.Error()
	if strings.Contains(msg, "deadline exceeded") {
		return ErrPollTimeout
	}
	// uTLS 握手被对端强制关闭 (EOF / connection reset) 属于瞬态网络故障,允许重试。
	if strings.Contains(msg, "EOF") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "broken pipe") {
		return ErrNetworkTransient
	}
	return ErrUpstream
}

func runnerErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	var ue *chatgpt.UpstreamError
	if errors.As(err, &ue) {
		if msg := upstreamErrorBodyMessage(ue.Body); msg != "" {
			return msg
		}
		if body := strings.TrimSpace(ue.Body); body != "" {
			return truncate(body, 500)
		}
	}
	return err.Error()
}

func upstreamErrorBodyMessage(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(body), &obj); err != nil {
		return ""
	}
	if msg := stringField(obj, "message"); msg != "" {
		return msg
	}
	if detail := stringField(obj, "detail"); detail != "" {
		return detail
	}
	if e, ok := obj["error"].(string); ok {
		return strings.TrimSpace(e)
	}
	if e, ok := obj["error"].(map[string]interface{}); ok {
		if msg := stringField(e, "message"); msg != "" {
			return msg
		}
		if code := stringField(e, "code"); code != "" {
			return code
		}
	}
	return ""
}

func stringField(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	v, _ := m[key].(string)
	return strings.TrimSpace(v)
}

func isImageRejectionMessage(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return false
	}
	keywords := []string{
		"抱歉", "无法", "不能", "不可以", "违反", "防护", "限制", "政策",
		"sorry", "can't", "cannot", "unable", "not able", "policy", "safety",
	}
	for _, kw := range keywords {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}

func assistantFailureCode(message, fallback string) string {
	if isImageRejectionMessage(message) {
		return ErrUpstreamRejected
	}
	return fallback
}

func firstFilled(items ...string) string {
	for _, s := range items {
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
	}
	return ""
}

// GenerateTaskID 生成对外 task_id。
func GenerateTaskID() string {
	return "img_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:24]
}
