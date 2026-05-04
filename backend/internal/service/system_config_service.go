package service

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kleinai/backend/internal/model"
	"github.com/kleinai/backend/internal/repo"
	"github.com/kleinai/backend/pkg/errcode"
)

// 系统配置 key 常量。
const (
	SettingProxyGlobalEnabled  = "proxy.global_enabled"
	SettingProxyGlobalID       = "proxy.global_id"
	SettingProxySelectionMode  = "proxy.selection_mode"
	SettingOAuthRefreshHours   = "oauth.refresh_before_hours"
	SettingOAuthOpenAIClientID = "oauth.openai_client_id"
	SettingOAuthOpenAITokenURL = "oauth.openai_token_url"
	SettingRetryMaxAttempts    = "retry.max_attempts"
	SettingRetryBaseDelayMs    = "retry.base_delay_ms"
	SettingRetryTimeoutSeconds = "retry.timeout_seconds"
	SettingCircuitFailures     = "tolerance.circuit_failures"
	SettingCircuitCooldown     = "tolerance.circuit_cooldown_seconds"
	SettingGrokCFEnabled       = "grok.cf.enabled"
	SettingGrokCFSolverURL     = "grok.cf.flaresolverr_url"
	SettingGrokCFRefreshSec    = "grok.cf.refresh_interval_seconds"
	SettingGrokCFTimeoutSec    = "grok.cf.timeout_seconds"
	SettingGrokCFCookies       = "grok.cf.cookies"
	SettingGrokCFClearance     = "grok.cf.clearance"
	SettingGrokCFUserAgent     = "grok.cf.user_agent"
	SettingGrokCFBrowser       = "grok.cf.browser"
	SettingGrokCFLastError     = "grok.cf.last_error"
	SettingGrokCFLastRefreshAt = "grok.cf.last_refresh_at"
)

// SystemConfigService 通用系统配置 KV 服务，带 30s 内存缓存。
type SystemConfigService struct {
	repo *repo.SystemConfigRepo

	mu     sync.RWMutex
	cache  map[string]string
	loaded time.Time
	ttl    time.Duration
}

// NewSystemConfigService 构造。ttl<=0 时默认 30s。
func NewSystemConfigService(r *repo.SystemConfigRepo) *SystemConfigService {
	return &SystemConfigService{repo: r, cache: map[string]string{}, ttl: 30 * time.Second}
}

// GetAll 全部 KV（已 JSON 解码）。
func (s *SystemConfigService) GetAll(ctx context.Context) (map[string]any, error) {
	rows, err := s.repo.GetAll(ctx)
	if err != nil {
		return nil, errcode.DBError.Wrap(err)
	}
	out := make(map[string]any, len(rows))
	for _, r := range rows {
		var v any
		_ = json.Unmarshal([]byte(r.Value), &v)
		out[r.Key] = v
	}
	return out, nil
}

// UpsertMany 批量更新。
// values 中每个值会先 JSON 序列化再写入。
func (s *SystemConfigService) UpsertMany(ctx context.Context, values map[string]any, updatedBy uint64) error {
	if len(values) == 0 {
		return nil
	}
	kvs := make(map[string]string, len(values))
	for k, v := range values {
		raw, err := json.Marshal(v)
		if err != nil {
			return errcode.InvalidParam.Wrap(err)
		}
		kvs[k] = string(raw)
	}
	uid := updatedBy
	if err := s.repo.UpsertMany(ctx, kvs, &uid); err != nil {
		return errcode.DBError.Wrap(err)
	}
	s.invalidate()
	return nil
}

// GetString 读字符串配置。fallback 为默认。
func (s *SystemConfigService) GetString(ctx context.Context, key, fallback string) string {
	raw, ok := s.getRaw(ctx, key)
	if !ok {
		return fallback
	}
	var v string
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		// 兼容字符串本身不带引号的旧值
		return strings.Trim(raw, "\"")
	}
	if v == "" {
		return fallback
	}
	return v
}

// GetInt 读 int64 配置。
func (s *SystemConfigService) GetInt(ctx context.Context, key string, fallback int64) int64 {
	raw, ok := s.getRaw(ctx, key)
	if !ok {
		return fallback
	}
	var v int64
	if err := json.Unmarshal([]byte(raw), &v); err == nil {
		return v
	}
	if n, err := strconv.ParseInt(strings.Trim(raw, "\""), 10, 64); err == nil {
		return n
	}
	return fallback
}

// GetBool 读 bool 配置。
func (s *SystemConfigService) GetBool(ctx context.Context, key string, fallback bool) bool {
	raw, ok := s.getRaw(ctx, key)
	if !ok {
		return fallback
	}
	var v bool
	if err := json.Unmarshal([]byte(raw), &v); err == nil {
		return v
	}
	switch strings.ToLower(strings.Trim(raw, "\"")) {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	}
	return fallback
}

// GetUint64 读 uint64 配置。
func (s *SystemConfigService) GetUint64(ctx context.Context, key string, fallback uint64) uint64 {
	raw, ok := s.getRaw(ctx, key)
	if !ok {
		return fallback
	}
	var v uint64
	if err := json.Unmarshal([]byte(raw), &v); err == nil {
		return v
	}
	if n, err := strconv.ParseUint(strings.Trim(raw, "\""), 10, 64); err == nil {
		return n
	}
	return fallback
}

// === 类型化便捷方法 ===

// GlobalProxyEnabled 是否启用全局代理。
func (s *SystemConfigService) GlobalProxyEnabled(ctx context.Context) bool {
	return s.GetBool(ctx, SettingProxyGlobalEnabled, false)
}

// GlobalProxyID 全局默认代理 ID（0 = 无）。
func (s *SystemConfigService) GlobalProxyID(ctx context.Context) uint64 {
	return s.GetUint64(ctx, SettingProxyGlobalID, 0)
}

// GlobalProxySelectionMode 全局代理选择模式：fixed / random。
func (s *SystemConfigService) GlobalProxySelectionMode(ctx context.Context) string {
	mode := strings.ToLower(strings.TrimSpace(s.GetString(ctx, SettingProxySelectionMode, "fixed")))
	switch mode {
	case "random":
		return "random"
	default:
		return "fixed"
	}
}

// RefreshBeforeHours OAuth 提前刷新窗口（小时）。
func (s *SystemConfigService) RefreshBeforeHours(ctx context.Context) int64 {
	v := s.GetInt(ctx, SettingOAuthRefreshHours, 24)
	if v <= 0 {
		v = 24
	}
	if v > 168 {
		v = 168
	}
	return v
}

// OpenAIClientID Codex CLI 公开 client_id。
func (s *SystemConfigService) OpenAIClientID(ctx context.Context) string {
	return s.GetString(ctx, SettingOAuthOpenAIClientID, "app_EMoamEEZ73f0CkXaXp7hrann")
}

// OpenAITokenURL OAuth Token Endpoint。
func (s *SystemConfigService) OpenAITokenURL(ctx context.Context) string {
	return s.GetString(ctx, SettingOAuthOpenAITokenURL, "https://auth.openai.com/oauth/token")
}

func (s *SystemConfigService) RetryMaxAttempts(ctx context.Context) int {
	v := s.GetInt(ctx, SettingRetryMaxAttempts, 2)
	if v < 0 {
		v = 0
	}
	if v > 20 {
		v = 20
	}
	return int(v) + 1
}

func (s *SystemConfigService) RetryBaseDelay(ctx context.Context) time.Duration {
	v := s.GetInt(ctx, SettingRetryBaseDelayMs, 800)
	if v < 0 {
		v = 0
	}
	if v > 60000 {
		v = 60000
	}
	return time.Duration(v) * time.Millisecond
}

func (s *SystemConfigService) RetryTimeout(ctx context.Context, fallback time.Duration) time.Duration {
	if fallback <= 0 {
		fallback = 5 * time.Minute
	}
	v := s.GetInt(ctx, SettingRetryTimeoutSeconds, int64(fallback/time.Second))
	if v <= 0 {
		return fallback
	}
	if v > 3600 {
		v = 3600
	}
	return time.Duration(v) * time.Second
}

// CircuitFailureThreshold 连续失败达到该次数后才把账号置为熔断。
func (s *SystemConfigService) CircuitFailureThreshold(ctx context.Context) int64 {
	v := s.GetInt(ctx, SettingCircuitFailures, 3)
	if v <= 0 {
		return 1
	}
	return v
}

// CircuitCooldownSeconds 账号熔断后的冷却秒数。
func (s *SystemConfigService) CircuitCooldownSeconds(ctx context.Context) int64 {
	v := s.GetInt(ctx, SettingCircuitCooldown, 300)
	if v < 0 {
		return 0
	}
	return v
}

func (s *SystemConfigService) GrokCFEnabled(ctx context.Context) bool {
	return s.GetBool(ctx, SettingGrokCFEnabled, true)
}

func (s *SystemConfigService) GrokCFSolverURL(ctx context.Context) string {
	return strings.TrimRight(s.GetString(ctx, SettingGrokCFSolverURL, "http://flaresolverr:8191"), "/")
}

func (s *SystemConfigService) GrokCFRefreshInterval(ctx context.Context) time.Duration {
	v := s.GetInt(ctx, SettingGrokCFRefreshSec, 600)
	if v < 60 {
		v = 60
	}
	if v > 86400 {
		v = 86400
	}
	return time.Duration(v) * time.Second
}

func (s *SystemConfigService) GrokCFTimeout(ctx context.Context) time.Duration {
	v := s.GetInt(ctx, SettingGrokCFTimeoutSec, 90)
	if v < 30 {
		v = 30
	}
	if v > 300 {
		v = 300
	}
	return time.Duration(v) * time.Second
}

// === internal ===

// getRaw 拿原始 JSON 字符串（命中缓存）。
func (s *SystemConfigService) getRaw(ctx context.Context, key string) (string, bool) {
	s.mu.RLock()
	if time.Since(s.loaded) < s.ttl {
		v, ok := s.cache[key]
		s.mu.RUnlock()
		return v, ok
	}
	s.mu.RUnlock()
	if err := s.reload(ctx); err != nil {
		return "", false
	}
	s.mu.RLock()
	v, ok := s.cache[key]
	s.mu.RUnlock()
	return v, ok
}

func (s *SystemConfigService) reload(ctx context.Context) error {
	rows, err := s.repo.GetAll(ctx)
	if err != nil {
		return err
	}
	m := make(map[string]string, len(rows))
	for _, r := range rows {
		m[r.Key] = r.Value
	}
	s.mu.Lock()
	s.cache = m
	s.loaded = time.Now()
	s.mu.Unlock()
	return nil
}

func (s *SystemConfigService) invalidate() {
	s.mu.Lock()
	s.loaded = time.Time{}
	s.mu.Unlock()
}

var _ = model.SystemConfig{} // 防止 import 被裁剪
