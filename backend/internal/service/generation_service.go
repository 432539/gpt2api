// Package service 生成任务编排：创建 → 预扣 → 调度账号 → 调用 provider → 结算 / 退款。
//
// 当前实现为同步 inline 执行（开发期）。生产建议替换为 asynq 投递到 worker。
package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/kleinai/backend/internal/model"
	"github.com/kleinai/backend/internal/provider"
	"github.com/kleinai/backend/internal/repo"
	"github.com/kleinai/backend/pkg/crypto"
	"github.com/kleinai/backend/pkg/errcode"
	"github.com/kleinai/backend/pkg/jwtpayload"
	"github.com/kleinai/backend/pkg/logger"
)

const codexOAuthClientID = "app_EMoamEEZ73f0CkXaXp7hrann"

// GenerationService 生成调度服务。
type GenerationService struct {
	db        *gorm.DB
	repo      *repo.GenerationRepo
	pool      *AccountPool
	billing   *BillingService
	providers map[string]provider.Provider // key: "gpt" / "grok"
	priceFn   PriceFunc
	aes       *crypto.AESGCM // 用于解密 account.credential_enc
	proxySvc  *ProxyService
	cfg       *SystemConfigService
}

// PriceFunc 模型计费：返回单次成本（点 *100）。
type PriceFunc func(modelCode string, kind provider.Kind, params map[string]any) int64

// NewGenerationService 构造。aes 必须非空（账号凭证加密强制）。
func NewGenerationService(db *gorm.DB, r *repo.GenerationRepo, pool *AccountPool, billing *BillingService, providers map[string]provider.Provider, priceFn PriceFunc, aes *crypto.AESGCM, proxySvc *ProxyService, cfg *SystemConfigService) *GenerationService {
	return &GenerationService{
		db:        db,
		repo:      r,
		pool:      pool,
		billing:   billing,
		providers: providers,
		priceFn:   priceFn,
		aes:       aes,
		proxySvc:  proxySvc,
		cfg:       cfg,
	}
}

// CreateRequest 创建生成请求 DTO（被 handler 填充）。
type CreateRequest struct {
	UserID    uint64
	APIKeyID  *uint64
	Kind      provider.Kind
	Mode      provider.Mode
	ModelCode string
	Provider  string
	Prompt    string
	NegPrompt string
	Params    map[string]any
	RefAssets []string
	Count     int
	IdemKey   string
	ClientIP  string
}

// Create 同步创建 + 触发任务。返回最终 task。
func (s *GenerationService) Create(ctx context.Context, req CreateRequest) (*model.GenerationTask, error) {
	if req.Count <= 0 {
		req.Count = 1
	}
	if req.IdemKey == "" {
		req.IdemKey = uuid.NewString()
	}

	if existing, err := s.repo.GetByIdem(ctx, req.UserID, req.IdemKey); err == nil && existing != nil {
		return existing, nil
	}

	cost := int64(0)
	if s.priceFn != nil {
		cost = s.priceFn(req.ModelCode, req.Kind, req.Params) * int64(req.Count)
	}
	if cost < 0 {
		return nil, errcode.InvalidParam.WithMsg("model price not configured")
	}

	taskID := newULID()
	req.RefAssets = s.normalizeInputRefs(ctx, &model.GenerationTask{TaskID: taskID}, req.RefAssets)
	req.Params = compactLargeInlineParams(req.Params)
	paramsJSON, _ := json.Marshal(req.Params)
	var refJSON *string
	if len(req.RefAssets) > 0 {
		b, _ := json.Marshal(req.RefAssets)
		s := string(b)
		refJSON = &s
	}
	t := &model.GenerationTask{
		TaskID:       taskID,
		UserID:       req.UserID,
		Kind:         string(req.Kind),
		Mode:         string(req.Mode),
		ModelCode:    req.ModelCode,
		Prompt:       req.Prompt,
		Params:       string(paramsJSON),
		RefAssets:    refJSON,
		Count:        req.Count,
		CostPoints:   cost,
		IdemKey:      req.IdemKey,
		Provider:     req.Provider,
		Status:       model.GenStatusPending,
		FromAPIKeyID: req.APIKeyID,
	}
	if req.NegPrompt != "" {
		ng := req.NegPrompt
		t.NegPrompt = &ng
	}
	if req.ClientIP != "" {
		ip := req.ClientIP
		t.ClientIP = &ip
	}

	if err := s.repo.Create(ctx, t); err != nil {
		return nil, errcode.DBError.Wrap(err)
	}

	if cost > 0 {
		if err := s.billing.PreDeduct(ctx, PreDeductReq{
			UserID:     req.UserID,
			TaskID:     taskID,
			Kind:       string(req.Kind),
			ModelCode:  req.ModelCode,
			Count:      req.Count,
			UnitPoints: cost / int64(req.Count),
		}); err != nil {
			_ = s.repo.SetFailed(ctx, taskID, err.Error())
			return nil, err
		}
	}

	go s.runTask(context.Background(), t)
	return t, nil
}

// runTask 后台执行：取池中账号 → 调 provider → 结算 / 退款。
func (s *GenerationService) runTask(ctx context.Context, t *model.GenerationTask) {
	log := logger.L().With(zap.String("task", t.TaskID))

	prov, ok := s.providers[t.Provider]
	if !ok {
		s.failTask(ctx, t, "provider not registered: "+t.Provider)
		return
	}

	var params map[string]any
	_ = json.Unmarshal([]byte(t.Params), &params)
	var refs []string
	if t.RefAssets != nil {
		_ = json.Unmarshal([]byte(*t.RefAssets), &refs)
	}
	refs = s.normalizeInputRefs(ctx, t, refs)

	timeout := 5 * time.Minute
	if t.Kind == "video" {
		timeout = 15 * time.Minute
	}
	if t.Provider == model.ProviderGPT && t.Kind == string(provider.KindImage) && strings.EqualFold(t.ModelCode, "gpt-image-2") && shouldUseGPTWebRoute(params) {
		timeout = 10 * time.Minute
	}
	maxAttempts := 3
	retryDelay := 800 * time.Millisecond
	if s.cfg != nil {
		timeout = s.cfg.RetryTimeout(ctx, timeout)
		maxAttempts = s.cfg.RetryMaxAttempts(ctx)
		retryDelay = s.cfg.RetryBaseDelay(ctx)
	}
	var acc *model.Account
	var res *provider.Result
	var lastErr error
	releaseAcc := func(a *model.Account) {
		if a != nil {
			s.pool.Release(a.ID)
		}
	}
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		picked, err := s.pickAccountForTask(ctx, t, params)
		if err != nil {
			if lastErr != nil {
				s.failTask(ctx, t, fmt.Sprintf("provider call: %v", lastErr))
			} else {
				s.failTask(ctx, t, fmt.Sprintf("pick account: %v", err))
			}
			return
		}
		acc = picked
		if err := s.repo.SetRunning(ctx, t.TaskID, acc.ID); err != nil {
			log.Warn("set running failed", zap.Error(err))
		}

		provReq := &provider.Request{
			TaskID:    t.TaskID,
			Kind:      provider.Kind(t.Kind),
			Mode:      provider.Mode(t.Mode),
			ModelCode: t.ModelCode,
			Prompt:    t.Prompt,
			Params:    params,
			RefAssets: refs,
			Count:     t.Count,
			Account:   acc,
		}
		provReq.UpstreamLog = s.makeUpstreamLogger(t, acc)
		if t.NegPrompt != nil {
			provReq.NegPrompt = *t.NegPrompt
		}
		if acc.BaseURL != nil {
			provReq.BaseURL = *acc.BaseURL
		} else if accountRequiresCodexRoute(t, params) && isCodexOAuthAccount(acc) {
			provReq.BaseURL = "https://chatgpt.com/backend-api/codex"
		}
		if proxyURL, perr := s.resolveProxyURL(ctx, acc); perr == nil {
			provReq.ProxyURL = proxyURL
		} else {
			log.Warn("resolve proxy failed", zap.Error(perr))
		}
		if s.aes != nil {
			cred, derr := s.providerCredential(ctx, acc, provReq.ProxyURL)
			if derr != nil {
				lastErr = derr
				if isFatalOAuthRefreshError(derr) {
					s.disableProviderAccount(ctx, acc, derr.Error())
				} else {
					s.markProviderFailed(ctx, acc, derr.Error(), 30*time.Minute)
				}
				releaseAcc(acc)
				acc = nil
				if attempt == maxAttempts || !retryableProviderError(derr) {
					s.failTask(ctx, t, fmt.Sprintf("provider call: %v", derr))
					return
				}
				sleepBeforeRetry(ctx, retryDelay, attempt)
				continue
			}
			provReq.Credential = cred
		}

		rctx, cancel := context.WithTimeout(ctx, timeout)
		out, err := prov.Generate(rctx, provReq)
		cancel()
		if err == nil {
			res = out
			break
		}
		lastErr = err
		if isUsageLimitReachedError(err) {
			s.markProviderQuotaLimited(ctx, acc, err.Error(), usageLimitResetAt(err))
		} else if isTransientProviderPathError(t.Provider, err) {
			s.pool.MarkTransientFailed(ctx, acc.ID, err.Error())
		} else {
			cooldown := providerCooldown(err)
			s.markProviderFailed(ctx, acc, err.Error(), cooldown)
		}
		releaseAcc(acc)
		acc = nil
		if attempt == maxAttempts || !retryableProviderError(err) {
			s.failTask(ctx, t, fmt.Sprintf("provider call: %v", err))
			return
		}
		log.Warn("provider retrying with next account", zap.Int("attempt", attempt), zap.Uint64("account_id", picked.ID), zap.Error(err))
		sleepBeforeRetry(ctx, retryDelay, attempt)
	}
	if res == nil {
		releaseAcc(acc)
		if lastErr != nil {
			s.failTask(ctx, t, fmt.Sprintf("provider call: %v", lastErr))
		} else {
			s.failTask(ctx, t, "provider call failed")
		}
		return
	}
	releaseAcc(acc)
	s.pool.MarkUsed(ctx, acc.ID)

	results := make([]*model.GenerationResult, 0, len(res.Assets))
	for i, a := range res.Assets {
		gr := &model.GenerationResult{
			TaskID: t.TaskID,
			UserID: t.UserID,
			Kind:   t.Kind,
			Seq:    int8(i),
			URL:    a.URL,
			Width:  intPtr(a.Width),
			Height: intPtr(a.Height),
		}
		if a.ThumbURL != "" {
			s := a.ThumbURL
			gr.ThumbURL = &s
		}
		if a.DurationMs > 0 {
			d := a.DurationMs
			gr.DurationMs = &d
		}
		if a.SizeBytes > 0 {
			b := a.SizeBytes
			gr.SizeBytes = &b
		}
		if len(a.Meta) > 0 {
			b, _ := json.Marshal(a.Meta)
			s := string(b)
			gr.Meta = &s
		}
		results = append(results, gr)
	}
	s.cacheResultAssets(ctx, t, acc, results)

	if err := s.repo.SetSucceeded(ctx, t.TaskID, results); err != nil {
		log.Error("set succeeded failed", zap.Error(err))
	}
	s.updateAccountUsageMeta(ctx, acc, t, len(results))
	if t.CostPoints > 0 {
		if err := s.billing.Settle(ctx, t.TaskID, &acc.ID); err != nil {
			log.Error("settle failed", zap.Error(err))
		}
	}
}

func (s *GenerationService) makeUpstreamLogger(t *model.GenerationTask, acc *model.Account) provider.UpstreamLogger {
	return func(ctx context.Context, e provider.UpstreamLogEntry) {
		if t == nil {
			return
		}
		meta := ""
		if len(e.Meta) > 0 {
			if b, err := json.Marshal(e.Meta); err == nil {
				meta = string(b)
			}
		}
		row := &model.GenerationUpstreamLog{
			TaskID:     t.TaskID,
			Provider:   e.Provider,
			Stage:      e.Stage,
			Method:     e.Method,
			URL:        truncate(e.URL, 512),
			StatusCode: e.StatusCode,
			DurationMs: e.DurationMs,
		}
		if row.Provider == "" {
			row.Provider = t.Provider
		}
		if acc != nil {
			row.AccountID = &acc.ID
		}
		if e.RequestExcerpt != "" {
			v := truncate(e.RequestExcerpt, 12000)
			row.RequestExcerpt = &v
		}
		if e.ResponseExcerpt != "" {
			v := truncate(e.ResponseExcerpt, 12000)
			row.ResponseExcerpt = &v
		}
		if e.Error != "" {
			v := truncate(e.Error, 4000)
			row.Error = &v
		}
		if meta != "" {
			row.Meta = &meta
		}
		if err := s.repo.CreateUpstreamLog(ctx, row); err != nil {
			logger.FromCtx(ctx).Warn("generation.upstream_log_failed", zap.String("task_id", t.TaskID), zap.String("stage", e.Stage), zap.Error(err))
		}
	}
}

func (s *GenerationService) providerCredential(ctx context.Context, acc *model.Account, proxyURL string) (string, error) {
	if acc == nil {
		return "", fmt.Errorf("missing account")
	}
	if acc.AuthType == model.AuthTypeOAuth && acc.Provider == model.ProviderGPT {
		return s.gptOAuthAccessToken(ctx, acc, proxyURL)
	}
	if len(acc.CredentialEnc) == 0 {
		return "", fmt.Errorf("account credential is empty")
	}
	plain, err := s.aes.Decrypt(acc.CredentialEnc)
	if err != nil {
		return "", fmt.Errorf("decrypt credential failed: %w", err)
	}
	cred := strings.TrimSpace(string(plain))
	if cred == "" {
		return "", fmt.Errorf("account credential is empty")
	}
	return cred, nil
}

func (s *GenerationService) pickAccountForTask(ctx context.Context, t *model.GenerationTask, params map[string]any) (*model.Account, error) {
	if t == nil {
		return nil, errcode.NoAvailableAcc
	}
	if t.Provider != model.ProviderGPT || t.Kind != string(provider.KindImage) || !strings.EqualFold(t.ModelCode, "gpt-image-2") {
		return s.pool.ReserveWhere(ctx, t.Provider, "round_robin", nil)
	}
	if accountRequiresCodexRoute(t, params) {
		return s.pool.ReserveWhere(ctx, t.Provider, "round_robin", isCodexOAuthAccount)
	}
	return s.pool.ReserveWhere(ctx, t.Provider, "round_robin", func(acc *model.Account) bool {
		if acc == nil {
			return false
		}
		return acc.AuthType == model.AuthTypeOAuth
	})
}

func accountRequiresCodexRoute(t *model.GenerationTask, params map[string]any) bool {
	if t == nil || t.Provider != model.ProviderGPT || t.Kind != string(provider.KindImage) || !strings.EqualFold(t.ModelCode, "gpt-image-2") {
		return false
	}
	return !shouldUseGPTWebRoute(params)
}

func shouldUseGPTWebRoute(params map[string]any) bool {
	tier := strings.ToUpper(strings.TrimSpace(strParamAny(params, "resolution", strParamAny(params, "size_tier", ""))))
	if tier == "" {
		size := strParamAny(params, "size", "")
		w, h := parseWH(size)
		if size == "" || w*h <= 1500000 {
			return true
		}
		return false
	}
	return tier == "1K" || tier == "1"
}

func isCodexOAuthAccount(acc *model.Account) bool {
	return acc != nil && acc.Provider == model.ProviderGPT && acc.AuthType == model.AuthTypeOAuth && strings.EqualFold(accountOAuthClientID(acc), codexOAuthClientID)
}

func strParamAny(p map[string]any, key, def string) string {
	if p == nil {
		return def
	}
	if v, ok := p[key]; ok {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return def
}

func parseWH(size string) (int, int) {
	parts := strings.SplitN(strings.TrimSpace(size), "x", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	var w, h int
	fmt.Sscanf(parts[0], "%d", &w)
	fmt.Sscanf(parts[1], "%d", &h)
	return w, h
}

func (s *GenerationService) markProviderFailed(ctx context.Context, acc *model.Account, reason string, desiredCooldown time.Duration) {
	if acc == nil {
		return
	}
	threshold := int64(3)
	cooldown := desiredCooldown
	if s.cfg != nil {
		threshold = s.cfg.CircuitFailureThreshold(ctx)
		if desiredCooldown > 0 {
			if sec := s.cfg.CircuitCooldownSeconds(ctx); sec > 0 {
				cooldown = time.Duration(sec) * time.Second
			}
		}
	}
	acc.ErrorCount++
	if threshold > 1 && int64(acc.ErrorCount) < threshold {
		cooldown = 0
	}
	s.pool.MarkFailed(ctx, acc.ID, reason, cooldown)
}

func (s *GenerationService) disableProviderAccount(ctx context.Context, acc *model.Account, reason string) {
	if acc == nil || s.pool == nil || s.pool.repo == nil {
		return
	}
	now := time.Now().UTC()
	fields := map[string]any{
		"status":           model.AccountStatusDisabled,
		"last_error":       truncate(reason, 240),
		"last_test_status": model.AccountTestFail,
		"last_test_error":  truncate(reason, 240),
		"last_test_at":     now,
		"cooldown_until":   nil,
		"error_count":      gorm.Expr("error_count + 1"),
	}
	if err := s.pool.repo.Update(ctx, acc.ID, fields); err != nil {
		logger.FromCtx(ctx).Warn("account.disable_failed", zap.Uint64("account_id", acc.ID), zap.Error(err))
		return
	}
	acc.Status = model.AccountStatusDisabled
	s.pool.Reload(acc.Provider)
	logger.FromCtx(ctx).Warn("account.disabled_after_oauth_refresh_401", zap.Uint64("account_id", acc.ID), zap.String("provider", acc.Provider), zap.String("reason", truncate(reason, 240)))
}

func (s *GenerationService) markProviderQuotaLimited(ctx context.Context, acc *model.Account, reason string, until time.Time) {
	if acc == nil || s.pool == nil || s.pool.repo == nil {
		return
	}
	fields := map[string]any{
		"status":      model.AccountStatusBroken,
		"last_error":  truncate(reason, 240),
		"error_count": gorm.Expr("error_count + 1"),
	}
	if until.IsZero() {
		until = time.Now().UTC().Add(24 * time.Hour)
	}
	fields["cooldown_until"] = until.UTC()
	if err := s.pool.repo.Update(ctx, acc.ID, fields); err != nil {
		logger.FromCtx(ctx).Warn("account.quota_limit_failed", zap.Uint64("account_id", acc.ID), zap.Error(err))
		return
	}
	acc.Status = model.AccountStatusBroken
	s.pool.Reload(acc.Provider)
	logger.FromCtx(ctx).Warn("account.quota_limited", zap.Uint64("account_id", acc.ID), zap.String("provider", acc.Provider), zap.Time("cooldown_until", until), zap.String("reason", truncate(reason, 240)))
}

func sleepBeforeRetry(ctx context.Context, base time.Duration, attempt int) {
	if base <= 0 || attempt <= 0 {
		return
	}
	delay := base * time.Duration(attempt)
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func (s *GenerationService) updateAccountUsageMeta(ctx context.Context, acc *model.Account, t *model.GenerationTask, units int) {
	if acc == nil || units <= 0 || t == nil || acc.OAuthMeta == nil || strings.TrimSpace(*acc.OAuthMeta) == "" {
		return
	}
	if t.Kind != string(provider.KindImage) && t.Kind != string(provider.KindVideo) {
		return
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(*acc.OAuthMeta), &meta); err != nil || meta == nil {
		return
	}
	remaining, ok := metaInt(meta, "image_quota_remaining")
	if !ok {
		return
	}
	remaining -= units
	if remaining < 0 {
		remaining = 0
	}
	meta["image_quota_remaining"] = remaining
	if total, ok := metaInt(meta, "image_quota_total"); ok && total >= remaining {
		meta["image_quota_used"] = total - remaining
	}
	meta["usage_updated_at"] = time.Now().UTC().Unix()
	raw, err := json.Marshal(meta)
	if err != nil {
		return
	}
	sv := string(raw)
	if err := s.db.WithContext(ctx).Model(&model.Account{}).Where("id = ?", acc.ID).Update("oauth_meta", sv).Error; err != nil {
		logger.FromCtx(ctx).Warn("account.usage_meta_update", zap.Uint64("id", acc.ID), zap.Error(err))
		return
	}
	acc.OAuthMeta = &sv
}

func metaInt(meta map[string]any, key string) (int, bool) {
	switch v := meta[key].(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case json.Number:
		n, err := v.Int64()
		return int(n), err == nil
	default:
		return 0, false
	}
}

func (s *GenerationService) gptOAuthAccessToken(ctx context.Context, acc *model.Account, proxyURL string) (string, error) {
	at, err := s.decryptOptional(acc.AccessTokenEnc)
	if err != nil {
		return "", fmt.Errorf("decrypt access_token failed: %w", err)
	}
	rt, err := s.decryptOptional(acc.RefreshTokenEnc)
	if err != nil {
		return "", fmt.Errorf("decrypt refresh_token failed: %w", err)
	}
	if rt == "" {
		rt, err = s.decryptOptional(acc.CredentialEnc)
		if err != nil {
			return "", fmt.Errorf("decrypt refresh credential failed: %w", err)
		}
	}
	if at != "" && rt == "" && !s.accessTokenNeedsRefresh(ctx, acc, at) {
		return at, nil
	}
	if at != "" && rt != "" && !s.accessTokenNeedsRefresh(ctx, acc, at) && !s.accessTokenShouldRefreshForCodex(acc) {
		return at, nil
	}
	if rt == "" {
		return "", fmt.Errorf("OAuth account missing refresh_token")
	}
	clientID, err := oauthRefreshClientID(acc)
	if err != nil {
		return "", err
	}
	oauth := NewOpenAIOAuthService(s.cfg)
	tr, err := oauth.RefreshToken(ctx, rt, clientID, proxyURL)
	if err != nil {
		return "", fmt.Errorf("refresh OAuth access_token failed: %w", err)
	}
	now := time.Now().UTC()
	updates := map[string]any{"last_refresh_at": now}
	atEnc, err := s.aes.Encrypt([]byte(strings.TrimSpace(tr.AccessToken)))
	if err != nil {
		return "", fmt.Errorf("encrypt access_token failed: %w", err)
	}
	updates["access_token_enc"] = atEnc
	if exp, ok := jwtpayload.ExpUnixFromJWT(tr.AccessToken); ok {
		t := time.Unix(exp, 0).UTC()
		updates["access_token_expires_at"] = t
	} else if tr.ExpiresIn > 0 {
		t := now.Add(time.Duration(tr.ExpiresIn) * time.Second)
		updates["access_token_expires_at"] = t
	}
	if strings.TrimSpace(tr.RefreshToken) != "" {
		rtEnc, err := s.aes.Encrypt([]byte(strings.TrimSpace(tr.RefreshToken)))
		if err != nil {
			return "", fmt.Errorf("encrypt refresh_token failed: %w", err)
		}
		updates["refresh_token_enc"] = rtEnc
		updates["credential_enc"] = rtEnc
	}
	meta := accountOAuthMeta(acc)
	meta["scope"] = tr.Scope
	meta["updated"] = now.Unix()
	if tr.IDToken != "" {
		meta["id_token_present"] = true
	}
	if raw, err := json.Marshal(meta); err == nil {
		updates["oauth_meta"] = string(raw)
	}
	if s.pool != nil && s.pool.repo != nil {
		if err := s.pool.repo.Update(ctx, acc.ID, updates); err != nil {
			return "", errcode.DBError.Wrap(err)
		}
	}
	acc.AccessTokenEnc = atEnc
	if v, ok := updates["access_token_expires_at"].(time.Time); ok {
		acc.AccessTokenExpiresAt = &v
	}
	if raw, ok := updates["oauth_meta"].(string); ok {
		acc.OAuthMeta = &raw
	}
	return strings.TrimSpace(tr.AccessToken), nil
}

func (s *GenerationService) accessTokenShouldRefreshForCodex(acc *model.Account) bool {
	if !isCodexOAuthAccount(acc) {
		return false
	}
	if acc.BaseURL != nil && strings.TrimSpace(*acc.BaseURL) != "" && !strings.Contains(strings.ToLower(*acc.BaseURL), "/codex") {
		return false
	}
	if acc.LastRefreshAt == nil {
		return true
	}
	return acc.LastRefreshAt.Before(time.Now().UTC().Add(-30 * time.Minute))
}

func oauthRefreshClientID(acc *model.Account) (string, error) {
	cid := strings.TrimSpace(accountOAuthClientID(acc))
	if isCodexOAuthAccount(acc) {
		return codexOAuthClientID, nil
	}
	if cid == "" {
		return "", fmt.Errorf("OAuth account missing client_id; ordinary ChatGPT accounts cannot fall back to Codex client_id")
	}
	return cid, nil
}

func (s *GenerationService) decryptOptional(cipher []byte) (string, error) {
	if len(cipher) == 0 {
		return "", nil
	}
	plain, err := s.aes.Decrypt(cipher)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(plain)), nil
}

func (s *GenerationService) accessTokenNeedsRefresh(ctx context.Context, acc *model.Account, at string) bool {
	if strings.TrimSpace(at) == "" {
		return true
	}
	expAt := acc.AccessTokenExpiresAt
	if expAt == nil {
		if exp, ok := jwtpayload.ExpUnixFromJWT(at); ok {
			t := time.Unix(exp, 0).UTC()
			expAt = &t
		}
	}
	if expAt == nil {
		return false
	}
	hours := int64(24)
	if s.cfg != nil {
		hours = s.cfg.RefreshBeforeHours(ctx)
	}
	return expAt.Before(time.Now().UTC().Add(time.Duration(hours) * time.Hour))
}

func (s *GenerationService) resolveProxyURL(ctx context.Context, acc *model.Account) (string, error) {
	if s.proxySvc == nil || s.cfg == nil {
		return "", nil
	}
	var (
		p   *model.Proxy
		err error
	)
	if acc != nil && acc.ProxyID != nil {
		p, err = s.proxySvc.GetByID(ctx, *acc.ProxyID)
	} else if s.cfg.GlobalProxyEnabled(ctx) {
		if s.cfg.GlobalProxySelectionMode(ctx) == "random" {
			p, err = s.proxySvc.PickEnabledRandom(ctx)
		} else {
			p, err = s.proxySvc.GetByID(ctx, s.cfg.GlobalProxyID(ctx))
		}
	}
	if err != nil || p == nil || p.Status != model.ProxyStatusEnabled {
		return "", err
	}
	u, err := s.proxySvc.BuildURL(p)
	if err != nil || u == nil {
		return "", err
	}
	return u.String(), nil
}

func (s *GenerationService) cacheResultAssets(ctx context.Context, t *model.GenerationTask, acc *model.Account, results []*model.GenerationResult) {
	if len(results) == 0 || s.cfg == nil || s.aes == nil || acc == nil {
		return
	}
	driver := strings.ToLower(strings.TrimSpace(s.cfg.GetString(ctx, "storage.result_cache_driver", "local")))
	if driver == "off" || driver == "none" {
		return
	}
	if driver == "oss" && !s.cfg.GetBool(ctx, "oss.enabled", false) {
		driver = "local"
	}
	if driver != "local" && driver != "oss" {
		driver = "local"
	}
	plain, err := s.aes.Decrypt(acc.CredentialEnc)
	if err != nil {
		logger.FromCtx(ctx).Warn("asset.cache.decrypt_failed", zap.Error(err))
		return
	}
	cookie := buildCookieForAssetDownload(string(plain))
	for i, gr := range results {
		if u, ok := s.cacheOneAsset(ctx, driver, cookie, gr.URL, t.TaskID, i, false); ok {
			gr.URL = u
		}
		if gr.ThumbURL != nil && *gr.ThumbURL != "" {
			if u, ok := s.cacheOneAsset(ctx, driver, cookie, *gr.ThumbURL, t.TaskID, i, true); ok {
				gr.ThumbURL = &u
			}
		}
	}
}

func (s *GenerationService) cacheOneAsset(ctx context.Context, driver, cookie, rawURL, taskID string, seq int, thumb bool) (string, bool) {
	if strings.HasPrefix(strings.TrimSpace(rawURL), "data:") {
		return s.cacheDataURLAsset(ctx, driver, rawURL, taskID, seq, thumb)
	}
	source := normalizeAssetSourceURL(rawURL)
	if source == "" || strings.HasPrefix(source, "/api/v1/gen/cached/") {
		return rawURL, false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return rawURL, false
	}
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Referer", "https://grok.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
	if err != nil {
		logger.FromCtx(ctx).Warn("asset.cache.download_failed", zap.String("url", source), zap.Error(err))
		return rawURL, false
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		logger.FromCtx(ctx).Warn("asset.cache.bad_status", zap.String("url", source), zap.Int("status", resp.StatusCode))
		return rawURL, false
	}
	ext := assetExt(source, resp.Header.Get("Content-Type"), thumb)
	now := time.Now()
	rel := path.Join("generated", now.Format("2006"), now.Format("01"), now.Format("02"), fmt.Sprintf("%s_%d%s%s", taskID, seq, map[bool]string{true: "_thumb", false: ""}[thumb], ext))
	root := strings.TrimSpace(os.Getenv("KLEIN_STORAGE_ROOT"))
	if root == "" {
		root = "/app/storage/public"
	}
	dst := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		logger.FromCtx(ctx).Warn("asset.cache.mkdir_failed", zap.Error(err))
		return rawURL, false
	}
	f, err := os.Create(dst)
	if err != nil {
		logger.FromCtx(ctx).Warn("asset.cache.create_failed", zap.Error(err))
		return rawURL, false
	}
	defer f.Close()
	written, err := io.Copy(f, resp.Body)
	if err != nil {
		logger.FromCtx(ctx).Warn("asset.cache.write_failed", zap.Error(err))
		return rawURL, false
	}
	if written <= 0 {
		_ = f.Close()
		_ = os.Remove(dst)
		logger.FromCtx(ctx).Warn("asset.cache.empty_file", zap.String("url", source), zap.String("file", dst))
		return rawURL, false
	}
	localURL := "/api/v1/gen/cached/" + rel
	if driver == "oss" {
		if ossURL, err := s.uploadCachedAssetToOSS(ctx, dst, rel, resp.Header.Get("Content-Type")); err == nil && ossURL != "" {
			return ossURL, true
		} else if err != nil {
			logger.FromCtx(ctx).Warn("asset.cache.oss_upload_failed", zap.String("file", dst), zap.Error(err))
		}
	}
	return localURL, true
}

func (s *GenerationService) cacheDataURLAsset(ctx context.Context, driver, rawURL, taskID string, seq int, thumb bool) (string, bool) {
	contentType, payload, ok := strings.Cut(strings.TrimSpace(rawURL), ",")
	if !ok || !strings.Contains(contentType, ";base64") {
		return rawURL, false
	}
	contentType = strings.TrimPrefix(contentType, "data:")
	if idx := strings.Index(contentType, ";"); idx >= 0 {
		contentType = contentType[:idx]
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		logger.FromCtx(ctx).Warn("asset.cache.data_url_decode_failed", zap.Error(err))
		return rawURL, false
	}
	if len(data) == 0 {
		logger.FromCtx(ctx).Warn("asset.cache.data_url_empty")
		return rawURL, false
	}
	ext := assetExt("", contentType, thumb)
	now := time.Now()
	rel := path.Join("generated", now.Format("2006"), now.Format("01"), now.Format("02"), fmt.Sprintf("%s_%d%s%s", taskID, seq, map[bool]string{true: "_thumb", false: ""}[thumb], ext))
	root := strings.TrimSpace(os.Getenv("KLEIN_STORAGE_ROOT"))
	if root == "" {
		root = "/app/storage/public"
	}
	dst := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		logger.FromCtx(ctx).Warn("asset.cache.mkdir_failed", zap.Error(err))
		return rawURL, false
	}
	if err := os.WriteFile(dst, data, 0644); err != nil {
		logger.FromCtx(ctx).Warn("asset.cache.write_failed", zap.Error(err))
		return rawURL, false
	}
	localURL := "/api/v1/gen/cached/" + rel
	if driver == "oss" {
		if ossURL, err := s.uploadCachedAssetToOSS(ctx, dst, rel, contentType); err == nil && ossURL != "" {
			return ossURL, true
		} else if err != nil {
			logger.FromCtx(ctx).Warn("asset.cache.oss_upload_failed", zap.String("file", dst), zap.Error(err))
		}
	}
	return localURL, true
}

func (s *GenerationService) normalizeInputRefs(ctx context.Context, t *model.GenerationTask, refs []string) []string {
	if len(refs) == 0 || s == nil || s.cfg == nil {
		return refs
	}
	driver := strings.ToLower(strings.TrimSpace(s.cfg.GetString(ctx, "storage.result_cache_driver", "local")))
	if driver == "off" || driver == "none" {
		driver = "local"
	}
	if driver != "local" && driver != "oss" {
		driver = "local"
	}
	out := make([]string, 0, len(refs))
	for i, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		if strings.HasPrefix(ref, "data:") {
			if cached, ok := s.cacheDataURLAsset(ctx, driver, ref, t.TaskID, i, false); ok && cached != "" {
				out = append(out, cached)
				continue
			}
		}
		out = append(out, ref)
	}
	return out
}

func compactLargeInlineParams(params map[string]any) map[string]any {
	if len(params) == 0 {
		return params
	}
	out := make(map[string]any, len(params))
	for k, v := range params {
		out[k] = compactLargeInlineValue(v)
	}
	return out
}

func compactLargeInlineValue(v any) any {
	switch x := v.(type) {
	case string:
		if len(x) > 2048 && strings.HasPrefix(strings.TrimSpace(x), "data:image/") {
			return "[inline image cached in ref_assets]"
		}
		return x
	case []any:
		out := make([]any, len(x))
		for i := range x {
			out[i] = compactLargeInlineValue(x[i])
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, vv := range x {
			out[k] = compactLargeInlineValue(vv)
		}
		return out
	default:
		return v
	}
}

func (s *GenerationService) uploadCachedAssetToOSS(ctx context.Context, filePath, rel, contentType string) (string, error) {
	if s.cfg == nil {
		return "", fmt.Errorf("missing system config")
	}
	provider := strings.ToLower(strings.TrimSpace(s.cfg.GetString(ctx, "oss.provider", "aliyun")))
	if provider != "" && provider != "aliyun" && provider != "oss" {
		return "", fmt.Errorf("unsupported oss provider %s", provider)
	}
	endpoint := strings.TrimSpace(s.cfg.GetString(ctx, "oss.endpoint", ""))
	bucket := strings.TrimSpace(s.cfg.GetString(ctx, "oss.bucket", ""))
	accessKeyID := strings.TrimSpace(s.cfg.GetString(ctx, "oss.access_key_id", ""))
	accessKeySecret := strings.TrimSpace(s.cfg.GetString(ctx, "oss.access_key_secret", ""))
	if endpoint == "" || bucket == "" || accessKeyID == "" || accessKeySecret == "" {
		return "", fmt.Errorf("oss config incomplete")
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	key := s.ossObjectKey(ctx, rel)
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", err
	}
	date := time.Now().UTC().Format(http.TimeFormat)
	resource := "/" + bucket + "/" + key
	signing := "PUT\n\n" + contentType + "\n" + date + "\n" + resource
	mac := hmac.New(sha1.New, []byte(accessKeySecret))
	_, _ = mac.Write([]byte(signing))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	putURL := ossObjectURL(endpoint, bucket, key)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, putURL, f)
	if err != nil {
		return "", err
	}
	req.ContentLength = st.Size()
	req.Header.Set("Date", date)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "OSS "+accessKeyID+":"+signature)
	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("oss upload HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	publicBase := strings.TrimRight(strings.TrimSpace(s.cfg.GetString(ctx, "oss.public_base_url", "")), "/")
	if publicBase != "" {
		return publicBase + "/" + key, nil
	}
	return ossObjectURL(endpoint, bucket, key), nil
}

func (s *GenerationService) ossObjectKey(ctx context.Context, rel string) string {
	prefix := "generated/{yyyy}/{mm}/{dd}"
	if s.cfg != nil {
		prefix = strings.TrimSpace(s.cfg.GetString(ctx, "oss.path_prefix", prefix))
	}
	now := time.Now()
	prefix = strings.Trim(prefix, "/")
	prefix = strings.ReplaceAll(prefix, "{yyyy}", now.Format("2006"))
	prefix = strings.ReplaceAll(prefix, "{mm}", now.Format("01"))
	prefix = strings.ReplaceAll(prefix, "{dd}", now.Format("02"))
	if prefix == "" {
		return path.Base(rel)
	}
	return prefix + "/" + path.Base(rel)
}

func ossObjectURL(endpoint, bucket, key string) string {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "https://" + endpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return endpoint + "/" + escapePathSegments(key)
	}
	if !strings.HasPrefix(u.Host, bucket+".") {
		u.Host = bucket + "." + u.Host
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/" + escapePathSegments(key)
	u.RawQuery = ""
	return u.String()
}

func escapePathSegments(v string) string {
	parts := strings.Split(v, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

func normalizeAssetSourceURL(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || strings.HasPrefix(v, "data:") {
		return ""
	}
	if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
		return v
	}
	return "https://assets.grok.com/" + strings.TrimLeft(v, "/")
}

func assetExt(source, contentType string, thumb bool) string {
	lower := strings.ToLower(source)
	for _, ext := range []string{".mp4", ".webm", ".png", ".jpg", ".jpeg", ".webp"} {
		if strings.Contains(lower, ext) {
			if ext == ".jpeg" {
				return ".jpg"
			}
			return ext
		}
	}
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "video/webm"):
		return ".webm"
	case strings.Contains(ct, "video/"):
		return ".mp4"
	case strings.Contains(ct, "png"):
		return ".png"
	case strings.Contains(ct, "webp"):
		return ".webp"
	case thumb:
		return ".jpg"
	default:
		return ".bin"
	}
}

func buildCookieForAssetDownload(cred string) string {
	cred = strings.TrimSpace(cred)
	if strings.Contains(cred, "=") {
		if !strings.Contains(cred, "sso-rw=") {
			if token := extractCookieValue(cred, "sso"); token != "" {
				cred = strings.TrimRight(cred, "; ") + "; sso-rw=" + token
			}
		}
		return cred
	}
	return "sso=" + cred + "; sso-rw=" + cred
}

func extractCookieValue(cookie, name string) string {
	prefix := name + "="
	for _, part := range strings.Split(cookie, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, prefix) {
			return strings.TrimPrefix(part, prefix)
		}
	}
	return ""
}

func (s *GenerationService) failTask(ctx context.Context, t *model.GenerationTask, reason string) {
	displayReason := userFacingGenerationError(reason)
	if err := s.repo.SetFailed(ctx, t.TaskID, displayReason); err != nil {
		logger.FromCtx(ctx).Warn("gen.fail.update_status", zap.Error(err))
	}
	if t.CostPoints > 0 {
		if err := s.billing.FailRefund(ctx, t.TaskID, displayReason); err != nil {
			logger.FromCtx(ctx).Warn("gen.fail.refund", zap.Error(err))
		}
	}
}

// ReapStaleTasks closes tasks that were left pending/running after a restart or
// a killed provider request. Normal in-flight jobs have much shorter context
// deadlines than these cutoffs, so this only catches genuinely abandoned rows.
func (s *GenerationService) ReapStaleTasks(ctx context.Context, userID uint64) {
	if s == nil || s.db == nil {
		return
	}
	now := time.Now().UTC()
	cutoff := now.Add(-1 * time.Hour)
	var tasks []*model.GenerationTask
	q := s.db.WithContext(ctx).
		Where("deleted_at IS NULL AND status IN ?", []int8{model.GenStatusPending, model.GenStatusRunning}).
		Where("(started_at IS NOT NULL AND started_at < ?) OR (started_at IS NULL AND created_at < ?)", cutoff, cutoff).
		Order("id ASC").
		Limit(200)
	if userID > 0 {
		q = q.Where("user_id = ?", userID)
	}
	if err := q.Find(&tasks).Error; err != nil {
		logger.FromCtx(ctx).Warn("gen.stale.query_failed", zap.Error(err))
		return
	}
	for _, t := range tasks {
		s.failTask(ctx, t, "任务执行超时，已自动结束")
	}
}

// === helpers ===

func intPtr(v int) *int {
	if v == 0 {
		return nil
	}
	return &v
}

// newULID 生成一个 26 字符 ULID（Crockford base32 简化版）。
//
// 用 UUID 转 hex 后截 26 位（在严格 ULID 库引入前的过渡方案）。
func newULID() string {
	id := uuid.NewString()
	clean := ""
	for i := 0; i < len(id); i++ {
		ch := id[i]
		if ch == '-' {
			continue
		}
		clean += string(ch)
		if len(clean) == 26 {
			break
		}
	}
	return clean
}

var _ = errors.New

var usageLimitResetAtRe = regexp.MustCompile(`"resets_at"\s*:\s*([0-9]+)`)

func isFatalOAuthRefreshError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "refresh oauth access_token failed") {
		return false
	}
	return strings.Contains(msg, " 401") ||
		strings.Contains(msg, "返回 401") ||
		strings.Contains(msg, "already been used") ||
		strings.Contains(msg, "please try signing in again") ||
		strings.Contains(msg, "invalid_request_error")
}

func retryableProviderError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return isFatalOAuthRefreshError(err) ||
		isUsageLimitReachedError(err) ||
		strings.Contains(msg, "http 429") ||
		strings.Contains(msg, "too many requests") ||
		isGrokRetryableForbiddenError(msg)
}

func isTransientProviderPathError(provider string, err error) bool {
	if err == nil || provider != model.ProviderGROK {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "http 403") && isGrokRetryableForbiddenError(msg)
}

func isGrokRetryableForbiddenError(msg string) bool {
	if msg == "" {
		return false
	}
	return strings.Contains(msg, "grok upload http 403") ||
		strings.Contains(msg, "grok video http 403") ||
		strings.Contains(msg, "grok media post http 403") ||
		strings.Contains(msg, "grok http 403") ||
		strings.Contains(msg, "forbidden") ||
		strings.Contains(msg, "cloudflare") ||
		strings.Contains(msg, "just a moment") ||
		strings.Contains(msg, "request rejected by anti-bot rules")
}

func isUsageLimitReachedError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "usage_limit_reached") ||
		strings.Contains(msg, "the usage limit has been reached") ||
		strings.Contains(msg, "\"plan_type\":\"free\"") ||
		strings.Contains(msg, "\"plan_type\": \"free\"")
}

func usageLimitResetAt(err error) time.Time {
	if err == nil {
		return time.Time{}
	}
	m := usageLimitResetAtRe.FindStringSubmatch(err.Error())
	if len(m) != 2 {
		return time.Time{}
	}
	sec, e := strconv.ParseInt(m[1], 10, 64)
	if e != nil || sec <= 0 {
		return time.Time{}
	}
	return time.Unix(sec, 0).UTC()
}

func providerCooldown(err error) time.Duration {
	if err == nil {
		return 5 * time.Minute
	}
	msg := strings.ToLower(err.Error())
	if isGrokRetryableForbiddenError(msg) {
		return 0
	}
	switch {
	case strings.Contains(msg, "http 429"), strings.Contains(msg, "too many requests"):
		return 30 * time.Minute
	case strings.Contains(msg, "http 403"), strings.Contains(msg, "forbidden"),
		strings.Contains(msg, "cloudflare"), strings.Contains(msg, "just a moment"),
		strings.Contains(msg, "anti-bot"), strings.Contains(msg, "request rejected"):
		return 2 * time.Hour
	case strings.Contains(msg, "anti-bot"), strings.Contains(msg, "request rejected"):
		return 2 * time.Hour
	default:
		return 10 * time.Minute
	}
}

func userFacingGenerationError(reason string) string {
	msg := strings.ToLower(reason)
	switch {
	case strings.Contains(msg, "just a moment"), strings.Contains(msg, "cloudflare"):
		return "GROK 触发了 Cloudflare 验证，请配置可用的 CF Cookie/代理后再试"
	case strings.Contains(msg, "grok video http 429"), strings.Contains(msg, "too many requests"):
		return "GROK 视频生成频率受限，请稍后重试，或更换可用账号/代理后再试"
	case strings.Contains(msg, "anti-bot"), strings.Contains(msg, "request rejected"):
		return "GROK 风控拦截了本次请求，请更换代理或稍后重试"
	default:
		return reason
	}
}
