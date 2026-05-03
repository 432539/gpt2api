// Package service 账号池管理后台业务。
package service

import (
	"context"
	"strings"

	"github.com/kleinai/backend/internal/dto"
	"github.com/kleinai/backend/internal/model"
	"github.com/kleinai/backend/internal/repo"
	"github.com/kleinai/backend/pkg/crypto"
	"github.com/kleinai/backend/pkg/errcode"
)

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
func (s *AccountAdminService) Create(ctx context.Context, adminID uint64, req *dto.AccountCreateReq) (*model.Account, error) {
	enc, err := s.aes.Encrypt([]byte(strings.TrimSpace(req.Credential)))
	if err != nil {
		return nil, errcode.Internal.Wrap(err)
	}
	weight := req.Weight
	if weight <= 0 {
		weight = 10
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
	// OAuth 类型：把 credential（即 refresh_token）也写进 refresh_token_enc，方便后续读取
	if req.AuthType == model.AuthTypeOAuth {
		rtEnc, err := s.aes.Encrypt([]byte(strings.TrimSpace(req.Credential)))
		if err != nil {
			return nil, errcode.Internal.Wrap(err)
		}
		a.RefreshTokenEnc = rtEnc
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
		enc, err := s.aes.Encrypt([]byte(strings.TrimSpace(*req.Credential)))
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

// List 列表分页。
func (s *AccountAdminService) List(ctx context.Context, req *dto.AccountListReq) ([]*dto.AccountResp, int64, error) {
	items, total, err := s.repo.List(ctx, repo.AccountListFilter{
		Provider: req.Provider,
		Status:   req.Status,
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

// BatchImport 文本批量导入；每行一条。返回成功导入数量。
//
// 行格式（按优先级匹配）：
//   <name>@@<credential>
//   <credential>@<base_url>
//   <credential>
func (s *AccountAdminService) BatchImport(ctx context.Context, adminID uint64, req *dto.AccountBatchImportReq) (int, error) {
	weight := req.Weight
	if weight <= 0 {
		weight = 10
	}
	lines := strings.Split(req.Text, "\n")
	items := make([]*model.Account, 0, len(lines))
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, cred, base := parseImportLine(line, req.BaseURL)
		if cred == "" {
			continue
		}
		enc, err := s.aes.Encrypt([]byte(cred))
		if err != nil {
			return 0, errcode.Internal.Wrap(err)
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
		items = append(items, a)
	}
	if err := s.repo.BatchCreate(ctx, items); err != nil {
		return 0, errcode.DBError.Wrap(err)
	}
	s.pool.Reload(req.Provider)
	return len(items), nil
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
func (s *AccountAdminService) BatchRefreshOAuth(ctx context.Context, provider string) (int, []uint64, error) {
	if s.testSvc == nil {
		return 0, nil, errcode.Internal.WithMsg("刷新服务未启用")
	}
	items, _, err := s.repo.List(ctx, repo.AccountListFilter{
		Provider: provider, Page: 1, PageSize: 200,
	})
	if err != nil {
		return 0, nil, errcode.DBError.Wrap(err)
	}
	ok := 0
	failed := []uint64{}
	for _, a := range items {
		if !a.IsOAuth() {
			continue
		}
		if _, err := s.testSvc.RefreshOAuth(ctx, a); err != nil {
			failed = append(failed, a.ID)
			continue
		}
		ok++
	}
	if provider != "" {
		s.pool.Reload(provider)
	}
	return ok, failed, nil
}

// === helpers ===

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
	return r
}

// maskCredential 凭证密文不可解密返回前端，仅给一个掩码占位。
func maskCredential(enc []byte) string {
	if len(enc) < 4 {
		return "******"
	}
	return "******" // 只暴露存在性
}

func strPtr(s string) *string { return &s }
