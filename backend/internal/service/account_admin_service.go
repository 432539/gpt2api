// Package service 账号池管理后台业务。
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kleinai/backend/internal/dto"
	"github.com/kleinai/backend/internal/model"
	"github.com/kleinai/backend/internal/repo"
	"github.com/kleinai/backend/pkg/crypto"
	"github.com/kleinai/backend/pkg/errcode"
	"github.com/kleinai/backend/pkg/jwtpayload"
)

const grokTokenTTL = 72 * time.Hour

// AccountAdminService 管理后台对账号池的增删改查 + 批量导入。
type AccountAdminService struct {
	repo    *repo.AccountRepo
	pool    *AccountPool
	aes     *crypto.AESGCM
	testSvc *AccountTestService // 可空：未注入则 Test/Refresh 返回不可用
}

// NewAccountAdminService 构造。aes 必须非空。
func NewAccountAdminService(r *repo.AccountRepo, pool *AccountPool, aes *crypto.AESGCM) *AccountAdminService {
	return &AccountAdminService{repo: r, pool: pool, aes: aes}
}

// SetTestService 注入测试服务（路由层装配后回填，避免循环依赖）。
func (s *AccountAdminService) SetTestService(t *AccountTestService) { s.testSvc = t }

// Create 创建单个账号。
// OAuth（GPT）对齐常见工具：access_token + refresh_token + session_token（可选）+ client_id（可选）；
// credential 仍可单独填 refresh_token（旧版单框）。
func (s *AccountAdminService) Create(ctx context.Context, adminID uint64, req *dto.AccountCreateReq) (*model.Account, error) {
	weight := req.Weight
	if weight <= 0 {
		weight = 10
	}

	var credPlain string
	switch req.AuthType {
	case model.AuthTypeOAuth:
		at := strings.TrimSpace(req.AccessToken)
		rt := strings.TrimSpace(req.RefreshToken)
		legacy := strings.TrimSpace(req.Credential)
		if rt == "" {
			rt = legacy
		}
		if at == "" && rt == "" {
			return nil, errcode.InvalidParam.WithMsg("OAuth 请至少填写 access_token、refresh_token 或凭证其一")
		}
		if rt != "" {
			credPlain = rt
		} else {
			credPlain = " "
		}
	default:
		credPlain = strings.TrimSpace(req.Credential)
		if req.Provider == model.ProviderGROK && req.AuthType == model.AuthTypeCookie {
			credPlain = normalizeGrokSSOToken(credPlain)
		}
		if credPlain == "" {
			return nil, errcode.InvalidParam.WithMsg("请填写凭证")
		}
	}

	enc, err := s.aes.Encrypt([]byte(credPlain))
	if err != nil {
		return nil, errcode.Internal.Wrap(err)
	}
	a := &model.Account{
		Provider:      req.Provider,
		Name:          req.Name,
		AuthType:      req.AuthType,
		CredentialEnc: enc,
		Weight:        weight,
		RPMLimit:      req.RPMLimit,
		TPMLimit:      req.TPMLimit,
		DailyQuota:    req.DailyQuota,
		MonthlyQuota:  req.MonthlyQuota,
		Status:        model.AccountStatusEnabled,
		CreatedBy:     &adminID,
	}
	if req.BaseURL != "" {
		a.BaseURL = strPtr(req.BaseURL)
	}
	if req.ProxyID != nil && *req.ProxyID > 0 {
		a.ProxyID = req.ProxyID
	}
	if req.Remark != "" {
		a.Remark = strPtr(req.Remark)
	}
	if req.AuthType == model.AuthTypeOAuth {
		rt := strings.TrimSpace(req.RefreshToken)
		if rt == "" {
			rt = strings.TrimSpace(req.Credential)
		}
		if rt != "" {
			rtEnc, err := s.aes.Encrypt([]byte(rt))
			if err != nil {
				return nil, errcode.Internal.Wrap(err)
			}
			a.RefreshTokenEnc = rtEnc
		}
		if at := strings.TrimSpace(req.AccessToken); at != "" {
			atEnc, err := s.aes.Encrypt([]byte(at))
			if err != nil {
				return nil, errcode.Internal.Wrap(err)
			}
			a.AccessTokenEnc = atEnc
			if exp, ok := jwtpayload.ExpUnixFromJWT(at); ok {
				t := time.Unix(exp, 0).UTC()
				a.AccessTokenExpiresAt = &t
			}
		}
		if st := strings.TrimSpace(req.SessionToken); st != "" {
			stEnc, err := s.aes.Encrypt([]byte(st))
			if err != nil {
				return nil, errcode.Internal.Wrap(err)
			}
			a.SessionTokenEnc = stEnc
		}
		if req.ClientID != "" {
			b, _ := json.Marshal(map[string]any{"client_id": req.ClientID})
			ms := string(b)
			a.OAuthMeta = &ms
		}
	}
	if req.Provider == model.ProviderGROK && req.AuthType == model.AuthTypeCookie {
		exp := time.Now().UTC().Add(grokTokenTTL)
		a.AccessTokenExpiresAt = &exp
	}
	if err := s.repo.Create(ctx, a); err != nil {
		return nil, errcode.DBError.Wrap(err)
	}
	s.pool.Reload(req.Provider)
	return a, nil
}

// Update 部分更新。
func (s *AccountAdminService) Update(ctx context.Context, id uint64, req *dto.AccountUpdateReq) error {
	cur, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return errcode.ResourceMissing
	}

	fields := map[string]any{}
	if req.Name != nil {
		fields["name"] = *req.Name
	}
	if req.Credential != nil && *req.Credential != "" {
		credPlain := strings.TrimSpace(*req.Credential)
		if cur.Provider == model.ProviderGROK && cur.AuthType == model.AuthTypeCookie {
			credPlain = normalizeGrokSSOToken(credPlain)
		}
		enc, err := s.aes.Encrypt([]byte(credPlain))
		if err != nil {
			return errcode.Internal.Wrap(err)
		}
		fields["credential_enc"] = enc
		// OAuth 凭证更新时同步刷新 refresh_token_enc，并清掉旧 access_token
		if cur.AuthType == model.AuthTypeOAuth {
			fields["refresh_token_enc"] = enc
			fields["access_token_enc"] = nil
			fields["access_token_expires_at"] = nil
		}
	}
	// OAuth 三件套：单独更新（仅 OAuth 账号生效；非 OAuth 静默忽略）
	if cur.AuthType == model.AuthTypeOAuth {
		if req.AccessToken != nil {
			at := strings.TrimSpace(*req.AccessToken)
			if at == "" {
				fields["access_token_enc"] = nil
				fields["access_token_expires_at"] = nil
			} else {
				enc, err := s.aes.Encrypt([]byte(at))
				if err != nil {
					return errcode.Internal.Wrap(err)
				}
				fields["access_token_enc"] = enc
				if exp, ok := jwtpayload.ExpUnixFromJWT(at); ok {
					t := time.Unix(exp, 0).UTC()
					fields["access_token_expires_at"] = t
				} else {
					fields["access_token_expires_at"] = nil
				}
			}
		}
		if req.RefreshToken != nil {
			rt := strings.TrimSpace(*req.RefreshToken)
			if rt == "" {
				fields["refresh_token_enc"] = nil
			} else {
				enc, err := s.aes.Encrypt([]byte(rt))
				if err != nil {
					return errcode.Internal.Wrap(err)
				}
				fields["refresh_token_enc"] = enc
				// credential_enc 同步用 RT，确保旧逻辑可用
				fields["credential_enc"] = enc
			}
		}
		if req.SessionToken != nil {
			st := strings.TrimSpace(*req.SessionToken)
			if st == "" {
				fields["session_token_enc"] = nil
			} else {
				enc, err := s.aes.Encrypt([]byte(st))
				if err != nil {
					return errcode.Internal.Wrap(err)
				}
				fields["session_token_enc"] = enc
			}
		}
		if req.ClientID != nil {
			cid := strings.TrimSpace(*req.ClientID)
			if cid == "" {
				fields["oauth_meta"] = nil
			} else {
				b, _ := json.Marshal(map[string]any{"client_id": cid})
				ms := string(b)
				fields["oauth_meta"] = ms
			}
		}
	}
	if req.BaseURL != nil {
		fields["base_url"] = *req.BaseURL
	}
	if req.ProxyID != nil {
		if *req.ProxyID == 0 {
			fields["proxy_id"] = nil
		} else {
			fields["proxy_id"] = *req.ProxyID
		}
	}
	if req.Weight != nil {
		fields["weight"] = *req.Weight
	}
	if req.RPMLimit != nil {
		fields["rpm_limit"] = *req.RPMLimit
	}
	if req.TPMLimit != nil {
		fields["tpm_limit"] = *req.TPMLimit
	}
	if req.DailyQuota != nil {
		fields["daily_quota"] = *req.DailyQuota
	}
	if req.MonthlyQuota != nil {
		fields["monthly_quota"] = *req.MonthlyQuota
	}
	if req.Status != nil {
		fields["status"] = *req.Status
	}
	if req.Remark != nil {
		fields["remark"] = *req.Remark
	}
	if err := s.repo.Update(ctx, id, fields); err != nil {
		return errcode.DBError.Wrap(err)
	}
	s.pool.Reload(cur.Provider)
	return nil
}

// Delete 软删除并刷新池。
func (s *AccountAdminService) Delete(ctx context.Context, id uint64) error {
	cur, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return errcode.ResourceMissing
	}
	if err := s.repo.SoftDelete(ctx, id); err != nil {
		return errcode.DBError.Wrap(err)
	}
	s.pool.Reload(cur.Provider)
	return nil
}

func (s *AccountAdminService) reloadPoolGPTAndGrok() {
	s.pool.Reload(model.ProviderGPT)
	s.pool.Reload(model.ProviderGROK)
}

// BatchDeleteByIDs 批量软删账号。
func (s *AccountAdminService) BatchDeleteByIDs(ctx context.Context, ids []uint64) (int64, error) {
	n, err := s.repo.SoftDeleteMany(ctx, ids)
	if err != nil {
		return 0, errcode.DBError.Wrap(err)
	}
	if n > 0 {
		s.reloadPoolGPTAndGrok()
	}
	return n, nil
}

// GetSecrets 解密返回单个账号的明文凭证（管理员专用，用于编辑面板回显）。
// 解密失败的字段返回空串，不阻断响应。
func (s *AccountAdminService) GetSecrets(ctx context.Context, id uint64) (*dto.AccountSecretsResp, error) {
	a, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, errcode.ResourceMissing
	}
	out := &dto.AccountSecretsResp{}
	if len(a.CredentialEnc) > 0 {
		if b, err := s.aes.Decrypt(a.CredentialEnc); err == nil {
			out.Credential = string(b)
		}
	}
	if len(a.AccessTokenEnc) > 0 {
		if b, err := s.aes.Decrypt(a.AccessTokenEnc); err == nil {
			out.AccessToken = string(b)
		}
	}
	if len(a.RefreshTokenEnc) > 0 {
		if b, err := s.aes.Decrypt(a.RefreshTokenEnc); err == nil {
			out.RefreshToken = string(b)
		}
	}
	if len(a.SessionTokenEnc) > 0 {
		if b, err := s.aes.Decrypt(a.SessionTokenEnc); err == nil {
			out.SessionToken = string(b)
		}
	}
	if a.OAuthMeta != nil && *a.OAuthMeta != "" {
		var m map[string]any
		if err := json.Unmarshal([]byte(*a.OAuthMeta), &m); err == nil {
			if v, ok := m["client_id"].(string); ok {
				out.ClientID = v
			}
		}
	}
	return out, nil
}

// PurgeAccounts 按条件批量软删：invalid 或 all（须 confirm）。
func (s *AccountAdminService) PurgeAccounts(ctx context.Context, req *dto.AccountPurgeReq) (int64, error) {
	var (
		n   int64
		err error
	)
	switch req.Scope {
	case "invalid":
		n, err = s.repo.SoftDeleteInvalid(ctx, req.Provider)
	case "zero_quota":
		n, err = s.repo.SoftDeleteZeroQuota(ctx, req.Provider)
	case "all":
		if req.Confirm != "DELETE_ALL_ACCOUNTS" {
			return 0, errcode.InvalidParam.WithMsg("清空全部账号须在 confirm 填入 DELETE_ALL_ACCOUNTS")
		}
		n, err = s.repo.SoftDeleteAll(ctx, req.Provider)
	default:
		return 0, errcode.InvalidParam.WithMsg("scope 仅支持 all / invalid")
	}
	if err != nil {
		return 0, errcode.DBError.Wrap(err)
	}
	if n > 0 {
		s.reloadPoolGPTAndGrok()
	}
	return n, nil
}

// List 列表分页。
func (s *AccountAdminService) List(ctx context.Context, req *dto.AccountListReq) ([]*dto.AccountResp, int64, error) {
	items, total, err := s.repo.List(ctx, repo.AccountListFilter{
		Provider: req.Provider,
		Status:   req.Status,
		PlanType: strings.ToLower(strings.TrimSpace(req.PlanType)),
		Keyword:  req.Keyword,
		Page:     req.Page,
		PageSize: req.PageSize,
	})
	if err != nil {
		return nil, 0, errcode.DBError.Wrap(err)
	}
	resp := make([]*dto.AccountResp, 0, len(items))
	for _, it := range items {
		resp = append(resp, accountToResp(it, s.aes))
	}
	return resp, total, nil
}

// BatchImport 文本行导入（format=lines）或 sub2api JSON 分片导入（format=sub2api）。
func (s *AccountAdminService) BatchImport(ctx context.Context, adminID uint64, req *dto.AccountBatchImportReq) (*dto.BatchImportResult, error) {
	format := strings.ToLower(strings.TrimSpace(req.Format))
	if format == "" {
		format = "lines"
	}
	if format == "sub2api" {
		return s.batchImportSub2API(ctx, adminID, req)
	}
	if strings.TrimSpace(req.AuthType) == "" {
		return nil, errcode.InvalidParam.WithMsg("auth_type 不能为空")
	}
	if strings.TrimSpace(req.Text) == "" {
		return nil, errcode.InvalidParam.WithMsg("text 不能为空")
	}

	weight := req.Weight
	if weight <= 0 {
		weight = 10
	}
	isOAuth := req.AuthType == model.AuthTypeOAuth
	lines := strings.Split(req.Text, "\n")
	items := make([]*model.Account, 0, len(lines))
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, cred, base := parseImportLine(line, req.BaseURL)
		if req.Provider == model.ProviderGROK && req.AuthType == model.AuthTypeCookie {
			name, cred, base = parseGrokCookieImportLine(line, req.BaseURL)
		}
		name = clampImportAccountName(name)
		if cred == "" {
			continue
		}
		if req.Provider == model.ProviderGROK && req.AuthType == model.AuthTypeCookie {
			cred = normalizeGrokSSOToken(cred)
			if name == "" {
				name = "grok-" + shortTokenName(cred)
			}
		}
		enc, err := s.aes.Encrypt([]byte(cred))
		if err != nil {
			return nil, errcode.Internal.Wrap(err)
		}
		a := &model.Account{
			Provider:      req.Provider,
			Name:          name,
			AuthType:      req.AuthType,
			CredentialEnc: enc,
			Weight:        weight,
			Status:        model.AccountStatusEnabled,
			CreatedBy:     &adminID,
		}
		if base != "" {
			b := base
			a.BaseURL = &b
		}
		if req.ProxyID != nil && *req.ProxyID > 0 {
			pid := *req.ProxyID
			a.ProxyID = &pid
		}
		if req.Provider == model.ProviderGROK && req.AuthType == model.AuthTypeCookie {
			exp := time.Now().UTC().Add(grokTokenTTL)
			a.AccessTokenExpiresAt = &exp
		}
		if isOAuth {
			rtEnc, err := s.aes.Encrypt([]byte(cred))
			if err != nil {
				return nil, errcode.Internal.Wrap(err)
			}
			a.RefreshTokenEnc = rtEnc
		}
		items = append(items, a)
	}
	if err := s.repo.BatchCreate(ctx, items); err != nil {
		return nil, errcode.DBError.Wrap(err)
	}
	s.pool.Reload(req.Provider)
	detected, failed := s.probeImportedGrokAccounts(ctx, items)
	return &dto.BatchImportResult{
		Imported: len(items),
		Skipped:  0,
		Detected: detected,
		Pending:  maxInt(0, len(items)-detected-failed),
		Failed:   failed,
	}, nil
}

func (s *AccountAdminService) batchImportSub2API(ctx context.Context, adminID uint64, req *dto.AccountBatchImportReq) (*dto.BatchImportResult, error) {
	defWeight := req.Weight
	if defWeight <= 0 {
		defWeight = 10
	}
	var items []*model.Account
	skipped := 0
	for _, item := range req.Accounts {
		t := strings.ToLower(strings.TrimSpace(item.Type))
		if t == "api_key" || t == "cookie" {
			skipped++
			continue
		}
		if item.Credentials == nil {
			skipped++
			continue
		}
		rt := strings.TrimSpace(item.Credentials.RefreshToken)
		at := strings.TrimSpace(item.Credentials.AccessToken)
		if rt == "" && at == "" {
			skipped++
			continue
		}
		credPlain := rt
		if credPlain == "" {
			credPlain = " "
		}
		prov := mapSub2APIPlatform(item.Platform)
		if prov == "" {
			prov = req.Provider
		}
		name := clampImportAccountName(item.Name)
		if name == "" {
			name = "sub2api-import"
		}
		w := defWeight
		if item.Priority > 0 {
			w = item.Priority
			if w > 1000 {
				w = 1000
			}
		}

		credEnc, err := s.aes.Encrypt([]byte(credPlain))
		if err != nil {
			return nil, errcode.Internal.Wrap(err)
		}
		a := &model.Account{
			Provider:      prov,
			Name:          name,
			AuthType:      model.AuthTypeOAuth,
			CredentialEnc: credEnc,
			Weight:        w,
			Status:        model.AccountStatusEnabled,
			CreatedBy:     &adminID,
		}
		if rt != "" {
			rtEnc, err := s.aes.Encrypt([]byte(rt))
			if err != nil {
				return nil, errcode.Internal.Wrap(err)
			}
			a.RefreshTokenEnc = rtEnc
		}
		if norm := strings.TrimSpace(req.BaseURL); norm != "" {
			b := norm
			low := strings.ToLower(b)
			if !strings.HasPrefix(low, "http://") && !strings.HasPrefix(low, "https://") {
				b = "https://" + b
			}
			a.BaseURL = &b
		}
		if req.ProxyID != nil && *req.ProxyID > 0 {
			pid := *req.ProxyID
			a.ProxyID = &pid
		}
		meta := map[string]any{
			"source":             "sub2api",
			"email":              item.Credentials.Email,
			"chatgpt_account_id": item.Credentials.ChatgptAccountID,
			"chatgpt_user_id":    item.Credentials.ChatgptUserID,
			"organization_id":    item.Credentials.OrganizationID,
			"plan_type":          item.Credentials.PlanType,
		}
		if cid := accountClientIDFromImport(item.Credentials); cid != "" {
			meta["client_id"] = cid
		}
		if mb, err := json.Marshal(meta); err == nil {
			ms := string(mb)
			a.OAuthMeta = &ms
		}

		if at != "" {
			atEnc, err := s.aes.Encrypt([]byte(at))
			if err != nil {
				return nil, errcode.Internal.Wrap(err)
			}
			a.AccessTokenEnc = atEnc
			if exp, ok := jwtpayload.ExpUnixFromJWT(at); ok {
				t := time.Unix(exp, 0).UTC()
				a.AccessTokenExpiresAt = &t
			}
		}
		if idt := strings.TrimSpace(item.Credentials.IDToken); idt != "" {
			idEnc, err := s.aes.Encrypt([]byte(idt))
			if err != nil {
				return nil, errcode.Internal.Wrap(err)
			}
			a.SessionTokenEnc = idEnc
		}

		items = append(items, a)
	}
	if len(items) == 0 {
		return &dto.BatchImportResult{Imported: 0, Skipped: skipped}, nil
	}
	if err := s.repo.BatchCreate(ctx, items); err != nil {
		return nil, errcode.DBError.Wrap(err)
	}
	seen := map[string]struct{}{}
	for _, it := range items {
		seen[it.Provider] = struct{}{}
	}
	for p := range seen {
		s.pool.Reload(p)
	}
	detected, failed := s.probeImportedGrokAccounts(ctx, items)
	return &dto.BatchImportResult{
		Imported: len(items),
		Skipped:  skipped,
		Detected: detected,
		Pending:  maxInt(0, len(items)-detected-failed),
		Failed:   failed,
	}, nil
}

func mapSub2APIPlatform(p string) string {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "openai":
		return model.ProviderGPT
	case "grok", "x-ai", "xai":
		return model.ProviderGROK
	default:
		return ""
	}
}

func accountClientIDFromImport(c *dto.Sub2APICreds) string {
	if c == nil {
		return ""
	}
	if cid := strings.TrimSpace(c.ClientID); cid != "" {
		return cid
	}
	if cid, ok := jwtpayload.StringClaimFromJWT(c.AccessToken, "client_id"); ok {
		return cid
	}
	if cid, ok := jwtpayload.StringClaimFromJWT(c.IDToken, "azp"); ok {
		return cid
	}
	return ""
}

// Test 触发账号连通性测试。
func (s *AccountAdminService) Test(ctx context.Context, id uint64) (*dto.AccountTestResp, error) {
	if s.testSvc == nil {
		return nil, errcode.Internal.WithMsg("测试服务未启用")
	}
	a, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, errcode.ResourceMissing
	}
	return s.testSvc.Test(ctx, a)
}

// RefreshOAuth 刷新 OAuth 账号 RT。
func (s *AccountAdminService) RefreshOAuth(ctx context.Context, id uint64) (*dto.AccountRefreshResp, error) {
	if s.testSvc == nil {
		return nil, errcode.Internal.WithMsg("刷新服务未启用")
	}
	a, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, errcode.ResourceMissing
	}
	resp, err := s.testSvc.RefreshOAuth(ctx, a)
	if err != nil {
		return nil, err
	}
	s.pool.Reload(a.Provider)
	return resp, nil
}

// BatchRefreshOAuth 批量刷新（按 provider）。返回成功数和失败 ID 列表。
func (s *AccountAdminService) BatchRefreshOAuth(ctx context.Context, provider string, page, pageSize int) (*dto.AccountBatchRefreshResp, error) {
	if s.testSvc == nil {
		return nil, errcode.Internal.WithMsg("刷新服务未启用")
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 1000 {
		pageSize = 1000
	}
	items, total, err := s.repo.List(ctx, repo.AccountListFilter{
		Provider: provider,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		return nil, errcode.DBError.Wrap(err)
	}
	ok := 0
	failed := []uint64{}
	var mu sync.Mutex
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for _, acc := range items {
		a := acc
		if !a.IsOAuth() {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				mu.Lock()
				failed = append(failed, a.ID)
				mu.Unlock()
				return
			}
			defer func() { <-sem }()
			if _, err := s.testSvc.RefreshOAuth(ctx, a); err != nil {
				mu.Lock()
				failed = append(failed, a.ID)
				mu.Unlock()
				return
			}
			mu.Lock()
			ok++
			mu.Unlock()
		}()
	}
	wg.Wait()
	if provider != "" {
		s.pool.Reload(provider)
	}
	hasMore := page*pageSize < int(total)
	nextPage := 0
	if hasMore {
		nextPage = page + 1
	}
	return &dto.AccountBatchRefreshResp{
		Refreshed: ok,
		FailedIDs: failed,
		Page:      page,
		PageSize:  pageSize,
		Total:     total,
		HasMore:   hasMore,
		NextPage:  nextPage,
	}, nil
}

func (s *AccountAdminService) BatchProbeQuota(ctx context.Context, provider string, page, pageSize int) (*dto.AccountBatchProbeResp, error) {
	if s.testSvc == nil {
		return nil, errcode.Internal.WithMsg("quota probe service disabled")
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 1000 {
		pageSize = 1000
	}
	probed := 0
	failed := []uint64{}

	items, total, err := s.repo.List(ctx, repo.AccountListFilter{
		Provider: provider,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		return nil, errcode.DBError.Wrap(err)
	}
	for _, a := range items {
		if !accountSupportsQuotaProbe(a) {
			continue
		}
		res, err := s.testSvc.Test(ctx, a)
		if err != nil || res == nil || !res.OK {
			failed = append(failed, a.ID)
			continue
		}
		probed++
	}
	if provider != "" {
		s.pool.Reload(provider)
	} else {
		s.reloadPoolGPTAndGrok()
	}
	hasMore := page*pageSize < int(total)
	nextPage := 0
	if hasMore {
		nextPage = page + 1
	}
	return &dto.AccountBatchProbeResp{
		Probed:    probed,
		FailedIDs: failed,
		Page:      page,
		PageSize:  pageSize,
		Total:     total,
		HasMore:   hasMore,
		NextPage:  nextPage,
	}, nil
}

// BatchAssignProxy 批量设置账号代理。
func (s *AccountAdminService) BatchAssignProxy(ctx context.Context, req *dto.AccountBatchAssignProxyReq) (*dto.AccountBatchAssignProxyResp, error) {
	if len(req.AccountIDs) == 0 {
		return nil, errcode.InvalidParam.WithMsg("account_ids 不能为空")
	}
	switch req.Mode {
	case "single":
		if req.ProxyID == nil {
			return nil, errcode.InvalidParam.WithMsg("single 模式需要 proxy_id")
		}
	case "cycle":
		if len(req.ProxyIDs) == 0 {
			return nil, errcode.InvalidParam.WithMsg("cycle 模式需要 proxy_ids")
		}
	default:
		return nil, errcode.InvalidParam.WithMsg("mode 仅支持 single / cycle")
	}

	updated := 0
	seenProvider := map[string]struct{}{}
	for idx, accountID := range req.AccountIDs {
		acc, err := s.repo.GetByID(ctx, accountID)
		if err != nil {
			return nil, errcode.ResourceMissing.WithMsg(fmt.Sprintf("账号 %d 不存在", accountID))
		}
		var proxyValue any
		if req.Mode == "single" {
			if req.ProxyID != nil && *req.ProxyID > 0 {
				proxyValue = *req.ProxyID
			} else {
				proxyValue = nil
			}
		} else {
			pid := req.ProxyIDs[idx%len(req.ProxyIDs)]
			if pid > 0 {
				proxyValue = pid
			} else {
				proxyValue = nil
			}
		}
		if err := s.repo.Update(ctx, acc.ID, map[string]any{"proxy_id": proxyValue}); err != nil {
			return nil, errcode.DBError.Wrap(err)
		}
		seenProvider[acc.Provider] = struct{}{}
		updated++
	}
	for provider := range seenProvider {
		s.pool.Reload(provider)
	}
	return &dto.AccountBatchAssignProxyResp{Updated: updated}, nil
}

func accountSupportsQuotaProbe(a *model.Account) bool {
	if a == nil {
		return false
	}
	switch a.Provider {
	case model.ProviderGPT:
		return a.AuthType == model.AuthTypeOAuth
	case model.ProviderGROK:
		return a.AuthType == model.AuthTypeCookie
	default:
		return false
	}
}

func (s *AccountAdminService) probeImportedGrokAccounts(ctx context.Context, items []*model.Account) (int, int) {
	if s.testSvc == nil || len(items) == 0 {
		return 0, 0
	}
	detected := 0
	failed := 0
	var mu sync.Mutex
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for _, item := range items {
		if item == nil || item.Provider != model.ProviderGROK || item.AuthType != model.AuthTypeCookie {
			continue
		}
		acc := item
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				mu.Lock()
				failed++
				mu.Unlock()
				return
			}
			defer func() { <-sem }()
			res, err := s.testSvc.Test(ctx, acc)
			mu.Lock()
			defer mu.Unlock()
			if err != nil || res == nil || !res.OK {
				failed++
				return
			}
			if strings.TrimSpace(res.PlanType) != "" {
				detected++
			}
		}()
	}
	wg.Wait()
	if detected > 0 || failed > 0 {
		s.pool.Reload(model.ProviderGROK)
	}
	return detected, failed
}

// === helpers ===

// clampImportAccountName 截断到库表 name 上限（utf8mb4 VARCHAR(128)）。
func clampImportAccountName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) <= 128 {
		return s
	}
	return string(r[:128])
}

func parseImportLine(line, defaultBase string) (name, cred, base string) {
	base = defaultBase
	if i := strings.Index(line, "@@"); i > 0 {
		name = strings.TrimSpace(line[:i])
		cred = strings.TrimSpace(line[i+2:])
		return
	}
	if i := strings.Index(line, "@http"); i > 0 {
		cred = strings.TrimSpace(line[:i])
		base = strings.TrimSpace(line[i+1:])
		return
	}
	cred = line
	if cred != "" {
		// 用 credential 末 6 位做默认 name
		if l := len(cred); l > 6 {
			name = "auto-" + cred[l-6:]
		} else {
			name = "auto-" + cred
		}
	}
	return
}

func parseGrokCookieImportLine(line, defaultBase string) (name, cred, base string) {
	base = defaultBase
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	if parts := strings.Split(line, "----"); len(parts) >= 3 {
		name = strings.TrimSpace(parts[0])
		cred = strings.TrimSpace(parts[2])
		return
	}
	if i := strings.Index(line, "@@"); i > 0 {
		name = strings.TrimSpace(line[:i])
		cred = strings.TrimSpace(line[i+2:])
		return
	}
	cred = line
	if cred != "" {
		name = "grok-" + shortTokenName(cred)
	}
	return
}

func accountToResp(a *model.Account, _ *crypto.AESGCM) *dto.AccountResp {
	r := &dto.AccountResp{
		ID:                a.ID,
		Provider:          a.Provider,
		Name:              a.Name,
		AuthType:          a.AuthType,
		CredentialMask:    maskCredential(a.CredentialEnc),
		Weight:            a.Weight,
		RPMLimit:          a.RPMLimit,
		TPMLimit:          a.TPMLimit,
		DailyQuota:        a.DailyQuota,
		MonthlyQuota:      a.MonthlyQuota,
		Status:            a.Status,
		ErrorCount:        a.ErrorCount,
		SuccessCount:      a.SuccessCount,
		HasRefreshToken:   len(a.RefreshTokenEnc) > 0,
		HasAccessToken:    len(a.AccessTokenEnc) > 0,
		LastTestStatus:    a.LastTestStatus,
		LastTestLatencyMs: a.LastTestLatencyMs,
		CreatedAt:         a.CreatedAt.Unix(),
		UpdatedAt:         a.UpdatedAt.Unix(),
	}
	if a.BaseURL != nil {
		r.BaseURL = *a.BaseURL
	}
	if a.ProxyID != nil {
		r.ProxyID = *a.ProxyID
	}
	if a.LastUsedAt != nil {
		r.LastUsedAt = a.LastUsedAt.Unix()
	}
	if a.CooldownUntil != nil {
		r.CooldownUntil = a.CooldownUntil.Unix()
	}
	if a.LastError != nil {
		r.LastError = *a.LastError
	}
	if a.Remark != nil {
		r.Remark = *a.Remark
	}
	if a.AccessTokenExpiresAt != nil {
		r.AccessTokenExpireAt = a.AccessTokenExpiresAt.Unix()
	}
	if a.LastRefreshAt != nil {
		r.LastRefreshAt = a.LastRefreshAt.Unix()
	}
	if a.LastTestAt != nil {
		r.LastTestAt = a.LastTestAt.Unix()
	}
	if a.LastTestError != nil {
		r.LastTestError = *a.LastTestError
	}
	fillAccountProbeFields(r, a)
	return r
}

// maskCredential 凭证密文不可解密返回前端，仅给一个掩码占位。
func maskCredential(enc []byte) string {
	if len(enc) < 4 {
		return "******"
	}
	return "******" // 只暴露存在性
}

func fillAccountProbeFields(r *dto.AccountResp, a *model.Account) {
	meta := accountOAuthMeta(a)
	if v, ok := meta["plan_type"].(string); ok {
		r.PlanType = strings.TrimSpace(v)
	}
	if v, ok := meta["default_model_slug"].(string); ok {
		r.DefaultModel = strings.TrimSpace(v)
	}
	r.ImageQuotaRemaining = intFromMeta(meta, "image_quota_remaining")
	r.ImageQuotaTotal = intFromMeta(meta, "image_quota_total")
	r.ImageQuotaResetAt = int64FromMeta(meta, "image_quota_reset_at")
}

func intFromMeta(meta map[string]any, key string) int {
	switch v := meta[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	default:
		return 0
	}
}

func int64FromMeta(meta map[string]any, key string) int64 {
	switch v := meta[key].(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	default:
		return 0
	}
}

func strPtr(s string) *string { return &s }

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
