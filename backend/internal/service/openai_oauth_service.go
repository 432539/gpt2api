package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kleinai/backend/pkg/errcode"
	"github.com/kleinai/backend/pkg/proxyx"
)

// OpenAITokenResponse OAuth Token 响应（Codex CLI 流）。
type OpenAITokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	ExpiresIn    int64  `json:"expires_in,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// OpenAIOAuthService 调用 auth.openai.com/oauth/token 刷新 access_token。
type OpenAIOAuthService struct {
	cfg *SystemConfigService
}

// NewOpenAIOAuthService 构造。
func NewOpenAIOAuthService(cfg *SystemConfigService) *OpenAIOAuthService {
	return &OpenAIOAuthService{cfg: cfg}
}

// RefreshToken 用 refresh_token 兑换新 access_token。
//
// proxyURL 可空字符串（直连）；返回的 RefreshToken 可能为空，调用方需自行保留旧值。
func (s *OpenAIOAuthService) RefreshToken(ctx context.Context, refreshToken, proxyURL string) (*OpenAITokenResponse, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil, errcode.InvalidParam.WithMsg("缺少 refresh_token")
	}
	clientID := s.cfg.OpenAIClientID(ctx)
	tokenURL := s.cfg.OpenAITokenURL(ctx)

	pu, err := proxyx.Parse(proxyURL)
	if err != nil {
		return nil, errcode.Internal.Wrap(err)
	}
	client, err := proxyx.BuildClient(pu, 30*time.Second)
	if err != nil {
		return nil, errcode.Internal.Wrap(err)
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", clientID)
	form.Set("scope", "openid profile email")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, errcode.Internal.Wrap(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "codex-cli/0.91.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OpenAI 刷新请求失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		// 提取 error 描述
		msg := strings.TrimSpace(string(body))
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return nil, fmt.Errorf("OpenAI 返回 %d: %s", resp.StatusCode, msg)
	}
	var tr OpenAITokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("解析 OpenAI Token 响应失败: %w", err)
	}
	if tr.AccessToken == "" {
		return nil, errors.New("OpenAI 响应缺少 access_token")
	}
	return &tr, nil
}
