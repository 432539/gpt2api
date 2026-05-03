package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/kleinai/backend/internal/dto"
	"github.com/kleinai/backend/internal/model"
	"github.com/kleinai/backend/internal/repo"
	"github.com/kleinai/backend/pkg/crypto"
	"github.com/kleinai/backend/pkg/errcode"
	"github.com/kleinai/backend/pkg/proxyx"
)

// AccountTestService 提供账号健康检查（测试连通性）与 OAuth 刷新。
type AccountTestService struct {
	accountRepo *repo.AccountRepo
	proxySvc    *ProxyService
	cfgSvc      *SystemConfigService
	openaiOAuth *OpenAIOAuthService
	aes         *crypto.AESGCM
}

// NewAccountTestService 构造。
func NewAccountTestService(
	r *repo.AccountRepo,
	proxySvc *ProxyService,
	cfgSvc *SystemConfigService,
	openaiOAuth *OpenAIOAuthService,
	aes *crypto.AESGCM,
) *AccountTestService {
	return &AccountTestService{
		accountRepo: r,
		proxySvc:    proxySvc,
		cfgSvc:      cfgSvc,
		openaiOAuth: openaiOAuth,
		aes:         aes,
	}
}

// resolveProxyURL 选择代理：账号 ProxyID 优先；否则若启用全局代理则用全局；否则空。
func (s *AccountTestService) resolveProxyURL(ctx context.Context, account *model.Account) (string, error) {
	pid := uint64(0)
	if account.ProxyID != nil {
		pid = *account.ProxyID
	} else if s.cfgSvc.GlobalProxyEnabled(ctx) {
		pid = s.cfgSvc.GlobalProxyID(ctx)
	}
	if pid == 0 {
		return "", nil
	}
	p, err := s.proxySvc.GetByID(ctx, pid)
	if err != nil {
		return "", err
	}
	if p == nil || p.Status != model.ProxyStatusEnabled {
		return "", nil
	}
	u, err := s.proxySvc.BuildURL(p)
	if err != nil {
		return "", err
	}
	if u == nil {
		return "", nil
	}
	return u.String(), nil
}

// decryptCredential 解密 credential 字段（明文 API Key / cookie / refresh_token）。
func (s *AccountTestService) decryptCredential(account *model.Account) (string, error) {
	if len(account.CredentialEnc) == 0 {
		return "", errors.New("账号未配置凭证")
	}
	plain, err := s.aes.Decrypt(account.CredentialEnc)
	if err != nil {
		return "", fmt.Errorf("解密凭证失败: %w", err)
	}
	return strings.TrimSpace(string(plain)), nil
}

// decryptAccessToken 解密 OAuth access_token；空返回空。
func (s *AccountTestService) decryptAccessToken(account *model.Account) (string, error) {
	if len(account.AccessTokenEnc) == 0 {
		return "", nil
	}
	plain, err := s.aes.Decrypt(account.AccessTokenEnc)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(plain)), nil
}

// Test 通用账号测试入口（自动按 provider + auth_type 派发）。
func (s *AccountTestService) Test(ctx context.Context, account *model.Account) (*dto.AccountTestResp, error) {
	proxyURL, err := s.resolveProxyURL(ctx, account)
	if err != nil {
		return nil, errcode.Internal.Wrap(err)
	}

	// OAuth 账号：用过期前阈值判断是否需要顺手刷新一次
	if account.IsOAuth() {
		if err := s.maybeRefresh(ctx, account, proxyURL); err != nil {
			// 刷新失败不直接终止：仍尝试用旧 access_token 探测
			fmt.Printf("[account-test] refresh failed: %v\n", err)
		}
	}

	start := time.Now()
	var (
		ok        bool
		latencyMs int
		errMsg    string
	)
	switch account.Provider {
	case model.ProviderGPT:
		ok, errMsg = s.testGPT(ctx, account, proxyURL)
	case model.ProviderGROK:
		ok, errMsg = s.testGROK(ctx, account, proxyURL)
	default:
		return nil, errcode.InvalidParam.WithMsg("不支持的 provider: " + account.Provider)
	}
	latencyMs = int(time.Since(start) / time.Millisecond)

	// 落库测试结果
	st := model.AccountTestFail
	if ok {
		st = model.AccountTestOK
	}
	if len(errMsg) > 250 {
		errMsg = errMsg[:250]
	}
	now := time.Now().UTC()
	_ = s.accountRepo.Update(ctx, account.ID, map[string]any{
		"last_test_at":         now,
		"last_test_status":     st,
		"last_test_latency_ms": latencyMs,
		"last_test_error":      errMsg,
	})

	return &dto.AccountTestResp{
		OK:        ok,
		LatencyMs: latencyMs,
		Error:     errMsg,
	}, nil
}

// === GPT (OpenAI) ===

// testGPT GET /v1/models（对 api_key / oauth 通用）。
func (s *AccountTestService) testGPT(ctx context.Context, account *model.Account, proxyURL string) (bool, string) {
	base := "https://api.openai.com"
	if account.BaseURL != nil && *account.BaseURL != "" {
		base = strings.TrimRight(*account.BaseURL, "/")
	}
	endpoint := base + "/v1/models"

	authHeader, err := s.buildAuthHeader(account)
	if err != nil {
		return false, err.Error()
	}

	pu, err := proxyx.Parse(proxyURL)
	if err != nil {
		return false, err.Error()
	}
	client, err := proxyx.BuildClient(pu, 20*time.Second)
	if err != nil {
		return false, err.Error()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, err.Error()
	}
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Sprintf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode/100 != 2 {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return false, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, msg)
	}
	return true, ""
}

// === GROK (xAI) ===

// testGROK 走 xAI api.x.ai 或自定义 base_url 的 /v1/models。
func (s *AccountTestService) testGROK(ctx context.Context, account *model.Account, proxyURL string) (bool, string) {
	base := "https://api.x.ai"
	if account.BaseURL != nil && *account.BaseURL != "" {
		base = strings.TrimRight(*account.BaseURL, "/")
	}
	endpoint := base + "/v1/models"

	cred, err := s.decryptCredential(account)
	if err != nil {
		return false, err.Error()
	}

	pu, err := proxyx.Parse(proxyURL)
	if err != nil {
		return false, err.Error()
	}
	client, err := proxyx.BuildClient(pu, 20*time.Second)
	if err != nil {
		return false, err.Error()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, err.Error()
	}
	req.Header.Set("Authorization", "Bearer "+cred)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Sprintf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode/100 != 2 {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return false, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, msg)
	}
	return true, ""
}

// buildAuthHeader 根据 auth_type 拼装 Authorization。
func (s *AccountTestService) buildAuthHeader(account *model.Account) (string, error) {
	switch account.AuthType {
	case model.AuthTypeAPIKey:
		cred, err := s.decryptCredential(account)
		if err != nil {
			return "", err
		}
		return "Bearer " + cred, nil
	case model.AuthTypeOAuth:
		// OAuth 优先用 access_token，没有则报错
		at, err := s.decryptAccessToken(account)
		if err != nil {
			return "", fmt.Errorf("解密 access_token 失败: %w", err)
		}
		if at == "" {
			return "", errors.New("OAuth 账号未取得 access_token，请先刷新 RT")
		}
		return "Bearer " + at, nil
	case model.AuthTypeCookie:
		// Cookie 类型这里直接当作 raw header，前端调用 chatgpt.com 后端时会传给 Cookie 头
		// 此处仍尝试以 Bearer 形式探测（不一定通过）
		cred, err := s.decryptCredential(account)
		if err != nil {
			return "", err
		}
		return cred, nil
	default:
		return "", fmt.Errorf("未知 auth_type: %s", account.AuthType)
	}
}

// === OAuth Refresh ===

// RefreshOAuth 刷新指定 OAuth 账号的 access_token，并写回 DB。
//
// 仅支持 provider=gpt + auth_type=oauth；refresh_token 存放在 credential_enc 中。
func (s *AccountTestService) RefreshOAuth(ctx context.Context, account *model.Account) (*dto.AccountRefreshResp, error) {
	if !account.IsOAuth() {
		return nil, errcode.InvalidParam.WithMsg("仅 OAuth 账号支持刷新 RT")
	}
	if account.Provider != model.ProviderGPT {
		return nil, errcode.InvalidParam.WithMsg("仅支持 OpenAI / GPT 账号刷新 RT")
	}

	// 取 RT：优先 refresh_token_enc，否则 credential_enc（首次导入时存的）
	rt := ""
	if len(account.RefreshTokenEnc) > 0 {
		plain, err := s.aes.Decrypt(account.RefreshTokenEnc)
		if err != nil {
			return nil, errcode.Internal.Wrap(err)
		}
		rt = strings.TrimSpace(string(plain))
	}
	if rt == "" {
		cred, err := s.decryptCredential(account)
		if err != nil {
			return nil, errcode.InvalidParam.Wrap(err)
		}
		rt = cred
	}
	if rt == "" {
		return nil, errcode.InvalidParam.WithMsg("账号未配置 refresh_token")
	}

	proxyURL, err := s.resolveProxyURL(ctx, account)
	if err != nil {
		return nil, errcode.Internal.Wrap(err)
	}

	tr, err := s.openaiOAuth.RefreshToken(ctx, rt, proxyURL)
	if err != nil {
		errMsg := err.Error()
		if len(errMsg) > 250 {
			errMsg = errMsg[:250]
		}
		_ = s.accountRepo.Update(ctx, account.ID, map[string]any{
			"last_error":  errMsg,
			"error_count": gorm.Expr("error_count + 1"),
		})
		return nil, errcode.GPTUnavailable.Wrap(err).WithMsg("刷新失败: " + err.Error())
	}

	// 加密 access_token
	atEnc, err := s.aes.Encrypt([]byte(tr.AccessToken))
	if err != nil {
		return nil, errcode.Internal.Wrap(err)
	}

	now := time.Now().UTC()
	updates := map[string]any{
		"access_token_enc": atEnc,
		"last_refresh_at":  now,
		"last_error":       "",
	}
	if tr.ExpiresIn > 0 {
		exp := now.Add(time.Duration(tr.ExpiresIn) * time.Second)
		updates["access_token_expires_at"] = exp
	}
	// 仅当 OpenAI 返回新 RT 时才覆盖
	if strings.TrimSpace(tr.RefreshToken) != "" {
		rtEnc, err := s.aes.Encrypt([]byte(tr.RefreshToken))
		if err != nil {
			return nil, errcode.Internal.Wrap(err)
		}
		updates["refresh_token_enc"] = rtEnc
	}
	// 把 id_token / scope 之类元信息合并到 oauth_meta
	meta := map[string]any{
		"scope":    tr.Scope,
		"updated":  now.Unix(),
	}
	if tr.IDToken != "" {
		meta["id_token_present"] = true
	}
	rawMeta, _ := json.Marshal(meta)
	updates["oauth_meta"] = string(rawMeta)

	if err := s.accountRepo.Update(ctx, account.ID, updates); err != nil {
		return nil, errcode.DBError.Wrap(err)
	}

	resp := &dto.AccountRefreshResp{
		OK:           true,
		ExpiresIn:    tr.ExpiresIn,
		RefreshedAt:  now.Unix(),
		HasRefreshTK: tr.RefreshToken != "",
	}
	return resp, nil
}

// maybeRefresh 当 OAuth access_token 距过期 N 小时内时自动刷新一次。
func (s *AccountTestService) maybeRefresh(ctx context.Context, account *model.Account, _ string) error {
	if !account.IsOAuth() || account.Provider != model.ProviderGPT {
		return nil
	}
	at, _ := s.decryptAccessToken(account)
	if at == "" {
		// 没有 access_token —— 强制刷新
		_, err := s.RefreshOAuth(ctx, account)
		if err != nil {
			return err
		}
		// 重新加载 account 字段（调用方下一行会用最新值）
		fresh, err := s.accountRepo.GetByID(ctx, account.ID)
		if err == nil {
			*account = *fresh
		}
		return nil
	}
	if account.AccessTokenExpiresAt == nil {
		return nil
	}
	hours := s.cfgSvc.RefreshBeforeHours(ctx)
	threshold := time.Now().UTC().Add(time.Duration(hours) * time.Hour)
	if account.AccessTokenExpiresAt.Before(threshold) {
		_, err := s.RefreshOAuth(ctx, account)
		if err != nil {
			return err
		}
		fresh, err := s.accountRepo.GetByID(ctx, account.ID)
		if err == nil {
			*account = *fresh
		}
	}
	return nil
}

// TestProxy 探测代理是否可用。
//
// 通过代理访问一个稳定的探测 URL 并测延迟。
func (s *AccountTestService) TestProxy(ctx context.Context, p *model.Proxy) (*dto.ProxyTestResp, error) {
	u, err := s.proxySvc.BuildURL(p)
	if err != nil {
		return nil, errcode.Internal.Wrap(err)
	}
	pu, err := proxyx.Parse(u.String())
	if err != nil {
		return nil, errcode.Internal.Wrap(err)
	}
	client, err := proxyx.BuildClient(pu, 15*time.Second)
	if err != nil {
		return nil, errcode.Internal.Wrap(err)
	}
	target := "https://www.google.com/generate_204"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, errcode.Internal.Wrap(err)
	}
	start := time.Now()
	resp, err := client.Do(req)
	latency := int(time.Since(start) / time.Millisecond)
	if err != nil {
		_ = s.proxySvc.MarkCheck(ctx, p.ID, false, latency, err.Error())
		return &dto.ProxyTestResp{OK: false, LatencyMs: latency, Error: err.Error()}, nil
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	ok := resp.StatusCode == http.StatusNoContent || resp.StatusCode/100 == 2
	errMsg := ""
	if !ok {
		errMsg = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	_ = s.proxySvc.MarkCheck(ctx, p.ID, ok, latency, errMsg)
	return &dto.ProxyTestResp{OK: ok, LatencyMs: latency, Error: errMsg}, nil
}

