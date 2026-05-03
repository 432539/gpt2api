package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kleinai/backend/internal/dto"
	"github.com/kleinai/backend/internal/model"
	"github.com/kleinai/backend/internal/repo"
	"github.com/kleinai/backend/pkg/crypto"
	"github.com/kleinai/backend/pkg/errcode"
	"github.com/kleinai/backend/pkg/jwtpayload"
	"github.com/kleinai/backend/pkg/outbound"
)

type flexStringList []string

func (l *flexStringList) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		*l = nil
		return nil
	}

	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		*l = []string{strings.TrimSpace(trimmed)}
		return nil
	}

	out := make([]string, 0, 4)
	collectFlexStrings(&out, raw)
	*l = out
	return nil
}

func (l flexStringList) Slice() []string {
	if len(l) == 0 {
		return nil
	}
	return append([]string(nil), l...)
}

func collectFlexStrings(dst *[]string, v any) {
	switch t := v.(type) {
	case string:
		s := strings.TrimSpace(t)
		if s != "" {
			*dst = append(*dst, s)
		}
	case []any:
		for _, it := range t {
			collectFlexStrings(dst, it)
		}
	case map[string]any:
		for _, it := range t {
			collectFlexStrings(dst, it)
		}
	case json.Number:
		s := strings.TrimSpace(t.String())
		if s != "" {
			*dst = append(*dst, s)
		}
	case bool:
		*dst = append(*dst, fmt.Sprint(t))
	case nil:
		return
	default:
		s := strings.TrimSpace(fmt.Sprint(t))
		if s != "" && s != "<nil>" {
			*dst = append(*dst, s)
		}
	}
}

type AccountTestService struct {
	accountRepo *repo.AccountRepo
	proxySvc    *ProxyService
	cfgSvc      *SystemConfigService
	openaiOAuth *OpenAIOAuthService
	aes         *crypto.AESGCM
}

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

func (s *AccountTestService) decryptCredential(account *model.Account) (string, error) {
	if len(account.CredentialEnc) == 0 {
		return "", errors.New("账号未配置凭证")
	}
	plain, err := s.aes.Decrypt(account.CredentialEnc)
	if err != nil {
		return "", fmt.Errorf("瑙ｅ瘑鍑瘉澶辫触: %w", err)
	}
	return strings.TrimSpace(string(plain)), nil
}

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

func (s *AccountTestService) decryptSessionToken(account *model.Account) string {
	if len(account.SessionTokenEnc) == 0 {
		return ""
	}
	plain, err := s.aes.Decrypt(account.SessionTokenEnc)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(plain))
}

func (s *AccountTestService) Test(ctx context.Context, account *model.Account) (*dto.AccountTestResp, error) {
	proxyURL, err := s.resolveProxyURL(ctx, account)
	if err != nil {
		errMsg := "代理配置不可用: " + err.Error()
		if len(errMsg) > 250 {
			errMsg = errMsg[:250]
		}
		now := time.Now().UTC()
		_ = s.accountRepo.Update(ctx, account.ID, map[string]any{
			"last_test_at":         now,
			"last_test_status":     model.AccountTestFail,
			"last_test_latency_ms": 0,
			"last_test_error":      errMsg,
		})
		return &dto.AccountTestResp{OK: false, Error: errMsg}, nil
	}

	if account.IsOAuth() {
		if err := s.maybeRefresh(ctx, account, proxyURL); err != nil {
			fmt.Printf("[account-test] refresh failed: %v\n", err)
		}
	}

	start := time.Now()
	var (
		ok        bool
		errMsg    string
		info      *accountTestInfo
		latencyMs int
	)
	switch account.Provider {
	case model.ProviderGPT:
		ok, errMsg, info = s.testGPT(ctx, account, proxyURL)
	case model.ProviderGROK:
		ok, errMsg, info = s.testGROK(ctx, account, proxyURL)
	default:
		return nil, errcode.InvalidParam.WithMsg("涓嶆敮鎸佺殑 provider: " + account.Provider)
	}
	latencyMs = int(time.Since(start) / time.Millisecond)

	st := model.AccountTestFail
	if ok {
		st = model.AccountTestOK
	}
	if len(errMsg) > 250 {
		errMsg = errMsg[:250]
	}
	now := time.Now().UTC()
	updates := map[string]any{
		"last_test_at":         now,
		"last_test_status":     st,
		"last_test_latency_ms": latencyMs,
		"last_test_error":      errMsg,
	}
	if account.Provider == model.ProviderGROK && account.AuthType == model.AuthTypeCookie && ok {
		updates["access_token_expires_at"] = now.Add(grokTokenTTL)
	}
	_ = s.accountRepo.Update(ctx, account.ID, updates)

	return &dto.AccountTestResp{
		OK:                  ok,
		LatencyMs:           latencyMs,
		Error:               errMsg,
		PlanType:            accountTestPlanType(info),
		DefaultModel:        accountTestDefaultModel(info),
		ImageQuotaRemaining: accountTestImageRemaining(info),
		ImageQuotaTotal:     accountTestImageTotal(info),
		ImageQuotaResetAt:   accountTestImageResetAt(info),
	}, nil
}

func (s *AccountTestService) testGPT(ctx context.Context, account *model.Account, proxyURL string) (bool, string, *accountTestInfo) {
	if account.AuthType == model.AuthTypeOAuth {
		return s.testOpenAIOAuth(ctx, account, proxyURL)
	}

	base := "https://api.openai.com"
	if account.BaseURL != nil && *account.BaseURL != "" {
		base = strings.TrimRight(*account.BaseURL, "/")
	}
	endpoint := base + "/v1/models"

	authHeader, err := s.buildAuthHeader(account)
	if err != nil {
		return false, err.Error(), nil
	}
	client, err := outbound.NewClient(outbound.Options{
		ProxyURL: proxyURL,
		Timeout:  20 * time.Second,
		Mode:     outbound.ModeUTLS,
		Profile:  outbound.ProfileChrome,
	})
	if err != nil {
		return false, err.Error(), nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, err.Error(), nil
	}
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Sprintf("璇锋眰澶辫触: %v", err), nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode/100 != 2 {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return false, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, msg), nil
	}
	return true, "", nil
}

type accountTestInfo struct {
	PlanType            string
	DefaultModel        string
	ImageQuotaRemaining int
	ImageQuotaTotal     int
	ImageQuotaResetAt   int64
	BlockedFeatures     []string
}

func (s *AccountTestService) testOpenAIOAuth(ctx context.Context, account *model.Account, proxyURL string) (bool, string, *accountTestInfo) {
	at, err := s.decryptAccessToken(account)
	if err != nil {
		return false, fmt.Sprintf("瑙ｅ瘑 access_token 澶辫触: %v", err), nil
	}
	if at == "" {
		return false, "OAuth 璐﹀彿鏈彇寰?access_token锛岃鍏堝埛鏂?RT", nil
	}
	claims, ok := jwtpayload.ClaimsFromJWT(at)
	if !ok {
		return false, "access_token 涓嶆槸鍙В鏋愮殑 JWT", nil
	}
	exp, ok := jwtpayload.ExpUnixFromJWT(at)
	if !ok {
		return false, "access_token 缂哄皯 exp", nil
	}
	if time.Now().Unix() >= exp {
		return false, "access_token 宸茶繃鏈燂紝璇峰埛鏂?RT", nil
	}
	cid := accountOAuthClientID(account)
	if cid == "" {
		if cidFromToken, ok := claims["client_id"].(string); ok {
			cid = strings.TrimSpace(cidFromToken)
		}
	}
	if cid == "" {
		return false, "OAuth 元数据缺少 client_id，请重新导入或刷新账号", nil
	}
	if _, ok := claims["https://api.openai.com/auth"]; !ok {
		if _, ok := claims["https://api.openai.com/profile"]; !ok {
			return false, "access_token 缂哄皯 OpenAI OAuth 鏉冮檺淇℃伅", nil
		}
	}

	info, err := s.probeChatGPTAccount(ctx, account, at, proxyURL)
	if err != nil {
		return false, err.Error(), nil
	}
	if info.PlanType == "" {
		info.PlanType = planTypeFromClaims(claims)
	}
	s.persistOAuthProbe(ctx, account, cid, info)
	return true, "", info
}

func (s *AccountTestService) probeChatGPTAccount(ctx context.Context, account *model.Account, accessToken, proxyURL string) (*accountTestInfo, error) {
	client, err := outbound.NewClient(outbound.Options{
		ProxyURL: proxyURL,
		Timeout:  30 * time.Second,
		Mode:     outbound.ModeUTLS,
		Profile:  outbound.ProfileChrome,
	})
	if err != nil {
		return nil, err
	}
	sessionToken := s.decryptSessionToken(account)
	cookieHeader := bootstrapChatGPTCookies(ctx, client, sessionToken)
	body := []byte(`{"gizmo_id":null,"requested_default_model":null,"conversation_id":null,"timezone_offset_min":-480,"system_hints":["picture_v2"]}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://chatgpt.com/backend-api/conversation/init", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	setChatGPTProbeHeaders(req, account, accessToken, cookieHeader)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("conversation/init 璇锋眰澶辫触: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(data))
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return nil, fmt.Errorf("conversation/init HTTP %d: %s", resp.StatusCode, msg)
	}

	var payload struct {
		BlockedFeatures  flexStringList `json:"blocked_features"`
		DefaultModelSlug string         `json:"default_model_slug"`
		LimitsProgress   []struct {
			FeatureName string `json:"feature_name"`
			Remaining   *int   `json:"remaining"`
			ResetAfter  string `json:"reset_after"`
			MaxValue    *int   `json:"max_value"`
			Cap         *int   `json:"cap"`
			Total       *int   `json:"total"`
			Limit       *int   `json:"limit"`
			Used        *int   `json:"used"`
			UsedValue   *int   `json:"used_value"`
			Consumed    *int   `json:"consumed"`
		} `json:"limits_progress"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}

	out := &accountTestInfo{
		DefaultModel:        payload.DefaultModelSlug,
		ImageQuotaRemaining: -1,
		ImageQuotaTotal:     -1,
		BlockedFeatures:     payload.BlockedFeatures.Slice(),
	}
	for _, item := range payload.LimitsProgress {
		if !isImageQuotaFeature(item.FeatureName) {
			continue
		}
		if item.Remaining != nil && (out.ImageQuotaRemaining < 0 || *item.Remaining < out.ImageQuotaRemaining) {
			out.ImageQuotaRemaining = *item.Remaining
		}
		if maxV := firstInt(item.MaxValue, item.Cap, item.Total, item.Limit); maxV != nil && *maxV > out.ImageQuotaTotal {
			out.ImageQuotaTotal = *maxV
		}
		if out.ImageQuotaTotal < 0 && item.Remaining != nil {
			if usedV := firstInt(item.Used, item.UsedValue, item.Consumed); usedV != nil {
				out.ImageQuotaTotal = *item.Remaining + *usedV
			}
		}
		if item.ResetAfter != "" {
			if t, e := time.Parse(time.RFC3339, item.ResetAfter); e == nil {
				ts := t.Unix()
				if out.ImageQuotaResetAt == 0 || ts < out.ImageQuotaResetAt {
					out.ImageQuotaResetAt = ts
				}
			}
		}
	}
	return out, nil
}

func bootstrapChatGPTCookies(ctx context.Context, client *http.Client, sessionToken string) string {
	cookies := make([]string, 0, 4)
	if strings.TrimSpace(sessionToken) != "" {
		cookies = append(cookies, "__Secure-next-auth.session-token="+strings.TrimSpace(sessionToken))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://chatgpt.com/", nil)
	if err != nil {
		return strings.Join(cookies, "; ")
	}
	setChatGPTProbeHeaders(req, &model.Account{}, "", strings.Join(cookies, "; "))
	resp, err := client.Do(req)
	if err != nil {
		return strings.Join(cookies, "; ")
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	for _, c := range resp.Cookies() {
		if c.Name == "" || c.Value == "" {
			continue
		}
		cookies = append(cookies, c.Name+"="+c.Value)
	}
	return strings.Join(cookies, "; ")
}

func setChatGPTProbeHeaders(req *http.Request, account *model.Account, accessToken, cookieHeader string) {
	const (
		baseURL          = "https://chatgpt.com"
		userAgent        = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36 Edg/143.0.0.0"
		clientVersion    = "prod-81e0c5cdf6140e8c5db714d613337f4aeab94029"
		clientBuildNum   = "6128297"
		defaultLanguage  = "zh-CN"
		defaultSecCHUA   = `"Microsoft Edge";v="143", "Chromium";v="143", "Not A(Brand";v="24"`
		defaultPlatform  = `"Windows"`
		defaultPriority  = "u=1, i"
		defaultAcceptLng = "zh-CN,zh;q=0.9,en;q=0.8,en-GB;q=0.7,en-US;q=0.6"
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Origin", baseURL)
	req.Header.Set("Referer", baseURL+"/")
	req.Header.Set("Accept-Language", defaultAcceptLng)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Sec-Ch-Ua", defaultSecCHUA)
	req.Header.Set("Sec-Ch-Ua-Arch", `"x86"`)
	req.Header.Set("Sec-Ch-Ua-Bitness", `"64"`)
	req.Header.Set("Sec-Ch-Ua-Full-Version", `"143.0.3650.96"`)
	req.Header.Set("Sec-Ch-Ua-Full-Version-List", `"Microsoft Edge";v="143.0.3650.96", "Chromium";v="143.0.7499.147", "Not A(Brand";v="24.0.0.0"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Model", `""`)
	req.Header.Set("Sec-Ch-Ua-Platform", defaultPlatform)
	req.Header.Set("Sec-Ch-Ua-Platform-Version", `"19.0.0"`)
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Priority", defaultPriority)
	req.Header.Set("Oai-Language", defaultLanguage)
	req.Header.Set("Oai-Client-Version", clientVersion)
	req.Header.Set("Oai-Client-Build-Number", clientBuildNum)
	req.Header.Set("Oai-Device-Id", fallbackChatGPTDeviceID(account.ID))
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	} else {
		req.Header.Del("Authorization")
	}
	if cookieHeader != "" {
		req.Header.Set("Cookie", cookieHeader)
	}
	req.Header.Set("X-Openai-Target-Path", req.URL.Path)
	req.Header.Set("X-Openai-Target-Route", req.URL.Path)
}

func (s *AccountTestService) persistOAuthProbe(ctx context.Context, account *model.Account, clientID string, info *accountTestInfo) {
	if info == nil {
		return
	}
	meta := accountOAuthMeta(account)
	meta["client_id"] = clientID
	if info.PlanType != "" {
		meta["plan_type"] = info.PlanType
	}
	if info.DefaultModel != "" {
		meta["default_model_slug"] = info.DefaultModel
	}
	if info.ImageQuotaRemaining >= 0 {
		meta["image_quota_remaining"] = info.ImageQuotaRemaining
	}
	if info.ImageQuotaTotal > 0 {
		meta["image_quota_total"] = info.ImageQuotaTotal
	} else if info.ImageQuotaRemaining > 0 && intFromMeta(meta, "image_quota_total") <= 0 {
		meta["image_quota_total"] = info.ImageQuotaRemaining
	}
	if info.ImageQuotaResetAt > 0 {
		meta["image_quota_reset_at"] = info.ImageQuotaResetAt
	}
	if len(info.BlockedFeatures) > 0 {
		meta["blocked_features"] = info.BlockedFeatures
	} else {
		delete(meta, "blocked_features")
	}
	meta["probed_at"] = time.Now().UTC().Unix()
	raw, _ := json.Marshal(meta)
	_ = s.accountRepo.Update(ctx, account.ID, map[string]any{"oauth_meta": string(raw)})
}

func (s *AccountTestService) testGROK(ctx context.Context, account *model.Account, proxyURL string) (bool, string, *accountTestInfo) {
	if account.AuthType == model.AuthTypeCookie {
		info, err := s.testGrokSSO(ctx, account, proxyURL)
		if err != nil {
			return false, err.Error(), nil
		}
		return true, "", info
	}

	base := "https://api.x.ai"
	if account.BaseURL != nil && *account.BaseURL != "" {
		base = strings.TrimRight(*account.BaseURL, "/")
	}
	endpoint := base + "/v1/models"

	cred, err := s.decryptCredential(account)
	if err != nil {
		return false, err.Error(), nil
	}
	client, err := outbound.NewClient(outbound.Options{
		ProxyURL: proxyURL,
		Timeout:  20 * time.Second,
		Mode:     outbound.ModeUTLS,
		Profile:  outbound.ProfileChrome,
	})
	if err != nil {
		return false, err.Error(), nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, err.Error(), nil
	}
	req.Header.Set("Authorization", "Bearer "+cred)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Sprintf("请求失败: %v", err), nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode/100 != 2 {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return false, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, msg), nil
	}
	return true, "", nil
}

type grokRateLimitResp struct {
	RemainingQueries  int `json:"remainingQueries"`
	TotalQueries      int `json:"totalQueries"`
	WindowSizeSeconds int `json:"windowSizeSeconds"`
}

type grokQuotaInfo struct {
	Remaining     int   `json:"remaining"`
	Total         int   `json:"total"`
	WindowSeconds int   `json:"window_seconds"`
	ResetAt       int64 `json:"reset_at"`
	SyncedAt      int64 `json:"synced_at"`
}

var grokRateLimitModes = []string{
	"auto",
	"fast",
	"expert",
	"heavy",
	"grok-420-computer-use-sa",
}

func (s *AccountTestService) testGrokSSO(ctx context.Context, account *model.Account, proxyURL string) (*accountTestInfo, error) {
	cred, err := s.decryptCredential(account)
	if err != nil {
		return nil, err
	}
	token := normalizeGrokSSOToken(cred)
	if token == "" {
		return nil, errors.New("Grok SSO token 涓虹┖")
	}
	client, err := outbound.NewClient(outbound.Options{
		ProxyURL: proxyURL,
		Timeout:  20 * time.Second,
		Mode:     outbound.ModeUTLS,
		Profile:  outbound.ProfileChrome,
	})
	if err != nil {
		return nil, err
	}

	quotas := map[string]grokQuotaInfo{}
	now := time.Now().UTC()
	for _, modeName := range grokRateLimitModes {
		q, err := s.fetchGrokRateLimit(ctx, client, token, modeName)
		if err != nil {
			if len(quotas) == 0 {
				return nil, err
			}
			continue
		}
		quotas[modeName] = grokQuotaInfo{
			Remaining:     q.RemainingQueries,
			Total:         q.TotalQueries,
			WindowSeconds: q.WindowSizeSeconds,
			ResetAt:       now.Add(time.Duration(q.WindowSizeSeconds) * time.Second).Unix(),
			SyncedAt:      now.Unix(),
		}
	}
	if len(quotas) == 0 {
		return nil, errors.New("Grok rate-limits 未返回可用额度")
	}

	plan := inferGrokPlanType(quotas)
	meta := accountOAuthMeta(account)
	meta["source"] = "grok2api-token"
	meta["plan_type"] = plan
	meta["default_model_slug"] = "fast"
	meta["grok_quota"] = quotas
	meta["probed_at"] = now.Unix()

	totalRemaining, totalQuota := 0, 0
	resetAt := int64(0)
	for _, q := range quotas {
		totalRemaining += q.Remaining
		totalQuota += q.Total
		if q.ResetAt > 0 && (resetAt == 0 || q.ResetAt < resetAt) {
			resetAt = q.ResetAt
		}
	}
	meta["image_quota_remaining"] = totalRemaining
	meta["image_quota_total"] = totalQuota
	if resetAt > 0 {
		meta["image_quota_reset_at"] = resetAt
	}
	raw, _ := json.Marshal(meta)
	_ = s.accountRepo.Update(ctx, account.ID, map[string]any{"oauth_meta": string(raw)})

	return &accountTestInfo{
		PlanType:            plan,
		DefaultModel:        "fast",
		ImageQuotaRemaining: totalRemaining,
		ImageQuotaTotal:     totalQuota,
		ImageQuotaResetAt:   resetAt,
	}, nil
}

func (s *AccountTestService) fetchGrokRateLimit(ctx context.Context, client *http.Client, token, modeName string) (*grokRateLimitResp, error) {
	body, _ := json.Marshal(map[string]string{"modelName": modeName})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://grok.com/rest/rate-limits", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", "sso="+token+"; sso-rw="+token)
	req.Header.Set("Origin", "https://grok.com")
	req.Header.Set("Referer", "https://grok.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("X-Statsig-ID", "YXV0aGVudGljYXRlZA==")
	req.Header.Set("X-XAI-Request-ID", uuid.NewString())
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rate-limits 璇锋眰澶辫触: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode/100 != 2 {
		msg := strings.TrimSpace(string(raw))
		if isGrokInvalidCredential(msg) || resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, fmt.Errorf("Grok token 鏃犳晥鎴栧凡杩囨湡: HTTP %d: %s", resp.StatusCode, trimMsg(msg, 200))
		}
		return nil, fmt.Errorf("rate-limits HTTP %d: %s", resp.StatusCode, trimMsg(msg, 200))
	}
	var out grokRateLimitResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("rate-limits 鍝嶅簲瑙ｆ瀽澶辫触: %w", err)
	}
	return &out, nil
}

func inferGrokPlanType(quotas map[string]grokQuotaInfo) string {
	maxTotal := 0
	for _, q := range quotas {
		if q.Total > maxTotal {
			maxTotal = q.Total
		}
	}
	switch {
	case maxTotal >= 150:
		return "heavy"
	case maxTotal >= 50:
		return "super"
	default:
		return "basic"
	}
}

func isGrokInvalidCredential(msg string) bool {
	msg = strings.ToLower(msg)
	markers := []string{
		"invalid-credentials",
		"bad-credentials",
		"failed to look up session id",
		"blocked-user",
		"email-domain-rejected",
		"session not found",
		"account suspended",
		"token revoked",
		"token expired",
	}
	for _, marker := range markers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

func trimMsg(msg string, n int) string {
	msg = strings.TrimSpace(msg)
	if len(msg) > n {
		return msg[:n]
	}
	return msg
}

func (s *AccountTestService) buildAuthHeader(account *model.Account) (string, error) {
	switch account.AuthType {
	case model.AuthTypeAPIKey:
		cred, err := s.decryptCredential(account)
		if err != nil {
			return "", err
		}
		return "Bearer " + cred, nil
	case model.AuthTypeOAuth:
		at, err := s.decryptAccessToken(account)
		if err != nil {
			return "", fmt.Errorf("瑙ｅ瘑 access_token 澶辫触: %w", err)
		}
		if at == "" {
			return "", errors.New("OAuth 璐﹀彿鏈彇寰?access_token锛岃鍏堝埛鏂?RT")
		}
		return "Bearer " + at, nil
	case model.AuthTypeCookie:
		cred, err := s.decryptCredential(account)
		if err != nil {
			return "", err
		}
		return cred, nil
	default:
		return "", fmt.Errorf("鏈煡 auth_type: %s", account.AuthType)
	}
}

func accountOAuthMeta(account *model.Account) map[string]any {
	meta := map[string]any{}
	if account == nil || account.OAuthMeta == nil || strings.TrimSpace(*account.OAuthMeta) == "" {
		return meta
	}
	_ = json.Unmarshal([]byte(*account.OAuthMeta), &meta)
	if meta == nil {
		meta = map[string]any{}
	}
	return meta
}

func accountOAuthClientID(account *model.Account) string {
	meta := accountOAuthMeta(account)
	if v, ok := meta["client_id"].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func (s *AccountTestService) RefreshOAuth(ctx context.Context, account *model.Account) (*dto.AccountRefreshResp, error) {
	if !account.IsOAuth() {
		return nil, errcode.InvalidParam.WithMsg("浠?OAuth 璐﹀彿鏀寔鍒锋柊 RT")
	}
	if account.Provider != model.ProviderGPT {
		return nil, errcode.InvalidParam.WithMsg("浠呮敮鎸?OpenAI / GPT 璐﹀彿鍒锋柊 RT")
	}

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
		return nil, errcode.InvalidParam.WithMsg("璐﹀彿鏈厤缃?refresh_token")
	}

	proxyURL, err := s.resolveProxyURL(ctx, account)
	if err != nil {
		return nil, errcode.Internal.Wrap(err)
	}

	clientID, err := oauthRefreshClientID(account)
	if err != nil {
		return nil, errcode.InvalidParam.WithMsg(err.Error())
	}
	tr, err := s.openaiOAuth.RefreshToken(ctx, rt, clientID, proxyURL)
	if err != nil {
		errMsg := err.Error()
		if len(errMsg) > 250 {
			errMsg = errMsg[:250]
		}
		_ = s.accountRepo.Update(ctx, account.ID, map[string]any{
			"last_error":  errMsg,
			"error_count": gorm.Expr("error_count + 1"),
		})
		return nil, errcode.GPTUnavailable.Wrap(err).WithMsg("鍒锋柊澶辫触: " + err.Error())
	}

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
	if strings.TrimSpace(tr.RefreshToken) != "" {
		rtEnc, err := s.aes.Encrypt([]byte(tr.RefreshToken))
		if err != nil {
			return nil, errcode.Internal.Wrap(err)
		}
		updates["refresh_token_enc"] = rtEnc
	}
	meta := accountOAuthMeta(account)
	if clientID != "" {
		meta["client_id"] = clientID
	}
	meta["scope"] = tr.Scope
	meta["updated"] = now.Unix()
	if tr.IDToken != "" {
		meta["id_token_present"] = true
	}
	rawMeta, _ := json.Marshal(meta)
	updates["oauth_meta"] = string(rawMeta)

	if err := s.accountRepo.Update(ctx, account.ID, updates); err != nil {
		return nil, errcode.DBError.Wrap(err)
	}

	return &dto.AccountRefreshResp{
		OK:           true,
		ExpiresIn:    tr.ExpiresIn,
		RefreshedAt:  now.Unix(),
		HasRefreshTK: tr.RefreshToken != "",
	}, nil
}

func (s *AccountTestService) maybeRefresh(ctx context.Context, account *model.Account, _ string) error {
	if !account.IsOAuth() || account.Provider != model.ProviderGPT {
		return nil
	}
	at, _ := s.decryptAccessToken(account)
	if at == "" {
		_, err := s.RefreshOAuth(ctx, account)
		if err != nil {
			return err
		}
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

func (s *AccountTestService) TestProxy(ctx context.Context, p *model.Proxy) (*dto.ProxyTestResp, error) {
	u, err := s.proxySvc.BuildURL(p)
	if err != nil {
		return nil, errcode.Internal.Wrap(err)
	}
	client, err := outbound.NewClient(outbound.Options{
		ProxyURL: u.String(),
		Timeout:  15 * time.Second,
		Mode:     outbound.ModeUTLS,
		Profile:  outbound.ProfileChrome,
	})
	if err != nil {
		return nil, errcode.Internal.Wrap(err)
	}
	target := "https://httpbin.org/ip"
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
	ok := resp.StatusCode/100 == 2
	errMsg := ""
	if !ok {
		errMsg = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	_ = s.proxySvc.MarkCheck(ctx, p.ID, ok, latency, errMsg)
	return &dto.ProxyTestResp{OK: ok, LatencyMs: latency, Error: errMsg}, nil
}

func planTypeFromClaims(claims map[string]any) string {
	auth, _ := claims["https://api.openai.com/auth"].(map[string]any)
	if auth == nil {
		return ""
	}
	for _, key := range []string{"chatgpt_plan_type", "plan_type", "account_plan_type"} {
		if v, ok := auth[key].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func fallbackChatGPTDeviceID(accountID uint64) string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", accountID%1_000_000_000_000)
}

func isImageQuotaFeature(name string) bool {
	n := strings.ToLower(name)
	switch n {
	case "image_gen", "image_generation", "image_edit", "img_gen":
		return true
	}
	return strings.Contains(n, "image_gen") || strings.Contains(n, "img_gen")
}

func firstInt(ps ...*int) *int {
	for _, p := range ps {
		if p != nil {
			return p
		}
	}
	return nil
}

func accountTestPlanType(info *accountTestInfo) string {
	if info == nil {
		return ""
	}
	return info.PlanType
}

func accountTestDefaultModel(info *accountTestInfo) string {
	if info == nil {
		return ""
	}
	return info.DefaultModel
}

func accountTestImageRemaining(info *accountTestInfo) int {
	if info == nil || info.ImageQuotaRemaining < 0 {
		return 0
	}
	return info.ImageQuotaRemaining
}

func accountTestImageTotal(info *accountTestInfo) int {
	if info == nil || info.ImageQuotaTotal < 0 {
		return 0
	}
	return info.ImageQuotaTotal
}

func accountTestImageResetAt(info *accountTestInfo) int64 {
	if info == nil {
		return 0
	}
	return info.ImageQuotaResetAt
}
