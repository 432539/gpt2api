package service

import (
	"context"
	crand "crypto/rand"
	"fmt"
	"net/url"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kleinai/backend/internal/dto"
	"github.com/kleinai/backend/internal/model"
	"github.com/kleinai/backend/internal/repo"
	"github.com/kleinai/backend/pkg/crypto"
	"github.com/kleinai/backend/pkg/errcode"
)

// ProxyService 代理 CRUD 与运行时 URL 拼装。
type ProxyService struct {
	repo *repo.ProxyRepo
	aes  *crypto.AESGCM

	// 简易缓存：避免热路径每次都查 DB。
	mu     sync.RWMutex
	cached map[uint64]*model.Proxy
	loaded time.Time
}

// NewProxyService 构造。
func NewProxyService(r *repo.ProxyRepo, aes *crypto.AESGCM) *ProxyService {
	return &ProxyService{repo: r, aes: aes, cached: map[uint64]*model.Proxy{}}
}

// Create 创建代理。
func (s *ProxyService) Create(ctx context.Context, adminID uint64, req *dto.ProxyCreateReq) (*model.Proxy, error) {
	if err := validateProtocol(req.Protocol); err != nil {
		return nil, err
	}
	p := &model.Proxy{
		Name:      strings.TrimSpace(req.Name),
		Protocol:  strings.ToLower(strings.TrimSpace(req.Protocol)),
		Host:      strings.TrimSpace(req.Host),
		Port:      req.Port,
		Status:    model.ProxyStatusEnabled,
		CreatedBy: &adminID,
	}
	if u := strings.TrimSpace(req.Username); u != "" {
		p.Username = &u
	}
	if pw := req.Password; pw != "" {
		enc, err := s.aes.Encrypt([]byte(pw))
		if err != nil {
			return nil, errcode.Internal.Wrap(err)
		}
		p.PasswordEnc = enc
	}
	if r := strings.TrimSpace(req.Remark); r != "" {
		p.Remark = &r
	}
	if err := s.repo.Create(ctx, p); err != nil {
		return nil, errcode.DBError.Wrap(err)
	}
	s.invalidate()
	return p, nil
}

// Update 更新代理（password 留空表示不变）。
func (s *ProxyService) Update(ctx context.Context, id uint64, req *dto.ProxyUpdateReq) error {
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return errcode.ResourceMissing
	}
	fields := map[string]any{}
	if req.Name != nil {
		fields["name"] = strings.TrimSpace(*req.Name)
	}
	if req.Protocol != nil {
		if err := validateProtocol(*req.Protocol); err != nil {
			return err
		}
		fields["protocol"] = strings.ToLower(*req.Protocol)
	}
	if req.Host != nil {
		fields["host"] = strings.TrimSpace(*req.Host)
	}
	if req.Port != nil {
		fields["port"] = *req.Port
	}
	if req.Username != nil {
		if u := strings.TrimSpace(*req.Username); u == "" {
			fields["username"] = nil
		} else {
			fields["username"] = u
		}
	}
	if req.Password != nil {
		if pw := *req.Password; pw == "" {
			fields["password_enc"] = nil
		} else {
			enc, err := s.aes.Encrypt([]byte(pw))
			if err != nil {
				return errcode.Internal.Wrap(err)
			}
			fields["password_enc"] = enc
		}
	}
	if req.Status != nil {
		fields["status"] = *req.Status
	}
	if req.Remark != nil {
		fields["remark"] = strings.TrimSpace(*req.Remark)
	}
	if err := s.repo.Update(ctx, id, fields); err != nil {
		return errcode.DBError.Wrap(err)
	}
	s.invalidate()
	return nil
}

// Delete 软删除。
func (s *ProxyService) Delete(ctx context.Context, id uint64) error {
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return errcode.ResourceMissing
	}
	if err := s.repo.SoftDelete(ctx, id); err != nil {
		return errcode.DBError.Wrap(err)
	}
	s.invalidate()
	return nil
}

// List 列表（出参脱敏）。
func (s *ProxyService) List(ctx context.Context, req *dto.ProxyListReq) ([]*dto.ProxyResp, int64, error) {
	items, total, err := s.repo.List(ctx, repo.ProxyListFilter{
		Status:   req.Status,
		Keyword:  req.Keyword,
		Page:     req.Page,
		PageSize: req.PageSize,
	})
	if err != nil {
		return nil, 0, errcode.DBError.Wrap(err)
	}
	resp := make([]*dto.ProxyResp, 0, len(items))
	for _, it := range items {
		resp = append(resp, proxyToResp(it))
	}
	return resp, total, nil
}

// ListEnabled 获取全部启用代理。
func (s *ProxyService) ListEnabled(ctx context.Context) ([]*model.Proxy, error) {
	items, err := s.repo.ListEnabled(ctx)
	if err != nil {
		return nil, errcode.DBError.Wrap(err)
	}
	return items, nil
}

// GetByID 获取（用于其它服务）。
func (s *ProxyService) GetByID(ctx context.Context, id uint64) (*model.Proxy, error) {
	if id == 0 {
		return nil, nil
	}
	s.mu.RLock()
	if p, ok := s.cached[id]; ok && time.Since(s.loaded) < 30*time.Second {
		s.mu.RUnlock()
		return p, nil
	}
	s.mu.RUnlock()
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.cached[id] = p
	s.loaded = time.Now()
	s.mu.Unlock()
	return p, nil
}

// ResolvePassword 把 PasswordEnc 解密成明文（仅在内部需要时用）。
func (s *ProxyService) ResolvePassword(p *model.Proxy) (string, error) {
	if len(p.PasswordEnc) == 0 {
		return "", nil
	}
	plain, err := s.aes.Decrypt(p.PasswordEnc)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// BuildURL 把代理拼成 url.URL（含密码）。
func (s *ProxyService) BuildURL(p *model.Proxy) (*url.URL, error) {
	if p == nil {
		return nil, nil
	}
	u := &url.URL{
		Scheme: p.Protocol,
		Host:   p.Host + ":" + strconv.Itoa(int(p.Port)),
	}
	if p.Username != nil && *p.Username != "" {
		pw, err := s.ResolvePassword(p)
		if err != nil {
			return nil, err
		}
		u.User = url.UserPassword(*p.Username, pw)
	}
	return u, nil
}

// MarkCheck 记录代理探测结果。
func (s *ProxyService) MarkCheck(ctx context.Context, id uint64, ok bool, latencyMs int, errMsg string) error {
	if errMsg != "" && len(errMsg) > 250 {
		errMsg = errMsg[:250]
	}
	if err := s.repo.MarkCheck(ctx, id, ok, latencyMs, errMsg); err != nil {
		return errcode.DBError.Wrap(err)
	}
	s.invalidate()
	return nil
}

// BatchDelete 批量删除代理。
func (s *ProxyService) BatchDelete(ctx context.Context, ids []uint64) (int64, error) {
	n, err := s.repo.SoftDeleteMany(ctx, ids)
	if err != nil {
		return 0, errcode.DBError.Wrap(err)
	}
	if n > 0 {
		s.invalidate()
	}
	return n, nil
}

// ImportText 批量导入代理，每行一个 URI，可追加 #名称。
func (s *ProxyService) ImportText(ctx context.Context, adminID uint64, text string) (*dto.ProxyBatchImportResult, error) {
	lines := strings.Split(text, "\n")
	items := make([]*model.Proxy, 0, len(lines))
	errs := make([]string, 0)
	skipped := 0
	created := 0
	for idx, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			skipped++
			continue
		}
		item, err := s.parseProxyLine(adminID, line)
		if err != nil {
			errs = append(errs, fmt.Sprintf("line %d: %v", idx+1, err))
			continue
		}
		items = append(items, item)
	}
	for _, item := range items {
		if err := s.repo.Create(ctx, item); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", item.Name, err))
			continue
		}
		created++
	}
	s.invalidate()
	failed := len(errs)
	if len(errs) > 20 {
		errs = errs[:20]
	}
	return &dto.ProxyBatchImportResult{
		Created: created,
		Skipped: skipped,
		Failed:  failed,
		Errors:  errs,
	}, nil
}

// PickEnabledRandom 随机选择一个启用代理。
func (s *ProxyService) PickEnabledRandom(ctx context.Context) (*model.Proxy, error) {
	items, err := s.repo.ListEnabled(ctx)
	if err != nil {
		return nil, errcode.DBError.Wrap(err)
	}
	if len(items) == 0 {
		return nil, nil
	}
	n, err := crand.Int(crand.Reader, big.NewInt(int64(len(items))))
	if err != nil {
		return nil, errcode.Internal.Wrap(err)
	}
	return items[int(n.Int64())], nil
}

// invalidate 简易缓存失效。
func (s *ProxyService) invalidate() {
	s.mu.Lock()
	s.cached = map[uint64]*model.Proxy{}
	s.mu.Unlock()
}

// === helpers ===

func validateProtocol(proto string) error {
	switch strings.ToLower(strings.TrimSpace(proto)) {
	case model.ProxyProtoHTTP,
		model.ProxyProtoHTTPS,
		model.ProxyProtoSOCKS5,
		model.ProxyProtoSOCKS5H:
		return nil
	default:
		return errcode.InvalidParam.WithMsg(fmt.Sprintf("不支持的协议: %s", proto))
	}
}

func (s *ProxyService) parseProxyLine(adminID uint64, line string) (*model.Proxy, error) {
	name := ""
	if hash := strings.LastIndex(line, "#"); hash >= 0 {
		name = strings.TrimSpace(line[hash+1:])
		line = strings.TrimSpace(line[:hash])
	}
	u, err := url.Parse(strings.TrimSpace(line))
	if err != nil {
		return nil, err
	}
	if err := validateProtocol(u.Scheme); err != nil {
		return nil, err
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return nil, fmt.Errorf("missing host")
	}
	portStr := u.Port()
	if portStr == "" {
		return nil, fmt.Errorf("missing port")
	}
	portN, err := strconv.Atoi(portStr)
	if err != nil || portN <= 0 || portN > 65535 {
		return nil, fmt.Errorf("invalid port")
	}
	if name == "" {
		name = host + ":" + portStr
	}
	p := &model.Proxy{
		Name:      name,
		Protocol:  strings.ToLower(strings.TrimSpace(u.Scheme)),
		Host:      host,
		Port:      uint16(portN),
		Status:    model.ProxyStatusEnabled,
		CreatedBy: &adminID,
	}
	if user := u.User.Username(); user != "" {
		p.Username = &user
	}
	if pass, ok := u.User.Password(); ok && pass != "" {
		enc, err := s.aes.Encrypt([]byte(pass))
		if err != nil {
			return nil, errcode.Internal.Wrap(err)
		}
		p.PasswordEnc = enc
	}
	return p, nil
}

func proxyToResp(p *model.Proxy) *dto.ProxyResp {
	r := &dto.ProxyResp{
		ID:           p.ID,
		Name:         p.Name,
		Protocol:     p.Protocol,
		Host:         p.Host,
		Port:         p.Port,
		Status:       p.Status,
		HasPassword:  len(p.PasswordEnc) > 0,
		LastCheckOK:  p.LastCheckOK,
		LastCheckMs:  p.LastCheckMs,
		CreatedAt:    p.CreatedAt.Unix(),
		UpdatedAt:    p.UpdatedAt.Unix(),
	}
	if p.Username != nil {
		r.Username = *p.Username
	}
	if p.LastCheckAt != nil {
		r.LastCheckAt = p.LastCheckAt.Unix()
	}
	if p.LastError != nil {
		r.LastError = *p.LastError
	}
	if p.Remark != nil {
		r.Remark = *p.Remark
	}
	return r
}
