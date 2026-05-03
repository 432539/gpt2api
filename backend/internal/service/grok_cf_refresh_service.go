package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kleinai/backend/internal/model"
	"github.com/kleinai/backend/pkg/logger"
)

const defaultGrokCFStatePath = "/app/storage/grok_cf.json"

type GrokCFRefreshService struct {
	cfg      *SystemConfigService
	proxySvc *ProxyService
	client   *http.Client
}

type grokCFState struct {
	Cookies     string `json:"cookies"`
	CFClearance string `json:"cf_clearance"`
	UserAgent   string `json:"user_agent"`
	Browser     string `json:"browser"`
	ProxyURL    string `json:"proxy_url,omitempty"`
	UpdatedAt   int64  `json:"updated_at"`
}

func NewGrokCFRefreshService(cfg *SystemConfigService, proxySvc *ProxyService) *GrokCFRefreshService {
	return &GrokCFRefreshService{
		cfg:      cfg,
		proxySvc: proxySvc,
		client:   &http.Client{Timeout: 5 * time.Minute},
	}
}

func (s *GrokCFRefreshService) Start(ctx context.Context) {
	if s == nil || s.cfg == nil {
		return
	}
	go s.loop(ctx)
}

func (s *GrokCFRefreshService) loop(ctx context.Context) {
	s.refreshOnce(ctx)
	ticker := time.NewTicker(s.cfg.GrokCFRefreshInterval(ctx))
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.refreshOnce(ctx)
			ticker.Reset(s.cfg.GrokCFRefreshInterval(ctx))
		}
	}
}

func (s *GrokCFRefreshService) refreshOnce(parent context.Context) {
	if !s.cfg.GrokCFEnabled(parent) {
		return
	}
	solverURL := s.cfg.GrokCFSolverURL(parent)
	if solverURL == "" {
		return
	}
	timeout := s.cfg.GrokCFTimeout(parent)
	ctx, cancel := context.WithTimeout(parent, timeout+15*time.Second)
	defer cancel()

	proxyURL, err := s.globalProxyURL(ctx)
	if err != nil {
		s.recordError(ctx, fmt.Sprintf("resolve proxy: %v", err))
		return
	}
	state, err := s.solve(ctx, solverURL, proxyURL, timeout)
	if err != nil {
		s.recordError(ctx, err.Error())
		return
	}
	if state.Cookies == "" && state.CFClearance == "" {
		s.recordError(ctx, "flaresolverr returned no cf cookies")
		return
	}
	if err := writeGrokCFState(state); err != nil {
		s.recordError(ctx, fmt.Sprintf("write state: %v", err))
		return
	}
	_ = s.cfg.UpsertMany(ctx, map[string]any{
		SettingGrokCFCookies:       state.Cookies,
		SettingGrokCFClearance:     state.CFClearance,
		SettingGrokCFUserAgent:     state.UserAgent,
		SettingGrokCFBrowser:       state.Browser,
		SettingGrokCFLastRefreshAt: state.UpdatedAt,
		SettingGrokCFLastError:     "",
	}, 0)
	logger.L().Info("grok cf refreshed",
		zap.Bool("has_clearance", state.CFClearance != ""),
		zap.Bool("has_proxy", state.ProxyURL != ""),
		zap.String("browser", state.Browser),
	)
}

func (s *GrokCFRefreshService) globalProxyURL(ctx context.Context) (string, error) {
	if s.proxySvc == nil || !s.cfg.GlobalProxyEnabled(ctx) {
		return "", nil
	}
	pid := s.cfg.GlobalProxyID(ctx)
	if pid == 0 {
		return "", nil
	}
	p, err := s.proxySvc.GetByID(ctx, pid)
	if err != nil || p == nil || p.Status != model.ProxyStatusEnabled {
		return "", err
	}
	u, err := s.proxySvc.BuildURL(p)
	if err != nil || u == nil {
		return "", err
	}
	return u.String(), nil
}

func (s *GrokCFRefreshService) solve(ctx context.Context, solverURL, proxyURL string, timeout time.Duration) (*grokCFState, error) {
	reqBody := map[string]any{
		"cmd":        "request.get",
		"url":        "https://grok.com",
		"maxTimeout": int(timeout / time.Millisecond),
	}
	if proxyURL != "" {
		reqBody["proxy"] = map[string]any{"url": proxyURL}
	}
	rawReq, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(solverURL, "/")+"/v1", bytes.NewReader(rawReq))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("flaresolverr request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("flaresolverr HTTP %d: %s", resp.StatusCode, snippetString(raw, 300))
	}
	var obj struct {
		Status   string `json:"status"`
		Message  string `json:"message"`
		Solution struct {
			UserAgent string `json:"userAgent"`
			Cookies   []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"cookies"`
		} `json:"solution"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("decode flaresolverr: %w", err)
	}
	if !strings.EqualFold(obj.Status, "ok") {
		return nil, fmt.Errorf("flaresolverr status %q: %s", obj.Status, obj.Message)
	}
	parts := make([]string, 0, len(obj.Solution.Cookies))
	cf := ""
	for _, c := range obj.Solution.Cookies {
		if strings.TrimSpace(c.Name) == "" {
			continue
		}
		parts = append(parts, strings.TrimSpace(c.Name)+"="+strings.TrimSpace(c.Value))
		if c.Name == "cf_clearance" {
			cf = strings.TrimSpace(c.Value)
		}
	}
	return &grokCFState{
		Cookies:     strings.Join(parts, "; "),
		CFClearance: cf,
		UserAgent:   strings.TrimSpace(obj.Solution.UserAgent),
		Browser:     browserFromUA(obj.Solution.UserAgent),
		ProxyURL:    proxyURL,
		UpdatedAt:   time.Now().Unix(),
	}, nil
}

func (s *GrokCFRefreshService) recordError(ctx context.Context, msg string) {
	logger.L().Warn("grok cf refresh failed", zap.String("error", msg))
	_ = s.cfg.UpsertMany(ctx, map[string]any{
		SettingGrokCFLastError: msg,
	}, 0)
}

func grokCFStatePath() string {
	if v := strings.TrimSpace(os.Getenv("KLEIN_GROK_CF_STATE_PATH")); v != "" {
		return v
	}
	return defaultGrokCFStatePath
}

func writeGrokCFState(state *grokCFState) error {
	path := grokCFStatePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func browserFromUA(ua string) string {
	ua = strings.ToLower(ua)
	if strings.Contains(ua, "chrome/") || strings.Contains(ua, "chromium/") {
		return "chrome"
	}
	if strings.Contains(ua, "firefox/") {
		return "firefox"
	}
	return ""
}

func snippetString(raw []byte, limit int) string {
	s := strings.TrimSpace(string(raw))
	if limit > 0 && len(s) > limit {
		return s[:limit] + "...(truncated)"
	}
	return s
}
