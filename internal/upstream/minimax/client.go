// Package minimax 封装对 MiniMax 官方 OpenAI 兼容 API 的调用。
//
// MiniMax 提供与 OpenAI 完全兼容的 REST API,包括:
//   - POST /v1/chat/completions   文字对话(流式 & 非流式)
//
// 认证:Bearer API Key(从配置文件 minimax.api_key 读取)。
// 文档:https://platform.minimaxi.com/document/ChatCompletion
package minimax

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/432539/gpt2api/internal/upstream/chatgpt"
)

const defaultBaseURL = "https://api.minimax.chat/v1"

// Client 是 MiniMax API 的 HTTP 客户端。
type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

// Options 构造选项。
type Options struct {
	// APIKey 是 MiniMax 平台颁发的 Bearer Key(必填)。
	APIKey string
	// BaseURL 覆盖默认 API 地址,供测试或代理使用;空字符串使用默认值。
	BaseURL string
	// Timeout HTTP 超时,0 表示不设超时。
	Timeout time.Duration
}

// New 创建 MiniMax 客户端。
func New(opt Options) (*Client, error) {
	if opt.APIKey == "" {
		return nil, fmt.Errorf("minimax: api_key is required")
	}
	base := opt.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	base = strings.TrimRight(base, "/")
	hc := &http.Client{}
	if opt.Timeout > 0 {
		hc.Timeout = opt.Timeout
	}
	return &Client{apiKey: opt.APIKey, baseURL: base, http: hc}, nil
}

// chatRequest 是发往 MiniMax /v1/chat/completions 的请求体(OpenAI 兼容)。
type chatRequest struct {
	Model    string               `json:"model"`
	Messages []chatgpt.ChatMessage `json:"messages"`
	Stream   bool                 `json:"stream"`
	// MaxTokens 传 0 时省略,由上游使用默认值。
	MaxTokens int `json:"max_tokens,omitempty"`
}

// StreamChat 发起流式对话,返回一个 SSEEvent 管道(与 chatgpt 包保持一致的类型)。
// 调用方应在 ctx 取消后耗尽管道。
func (c *Client) StreamChat(ctx context.Context, model string, messages []chatgpt.ChatMessage, maxTokens int) (<-chan chatgpt.SSEEvent, error) {
	body, err := json.Marshal(chatRequest{
		Model:     model,
		Messages:  messages,
		Stream:    true,
		MaxTokens: maxTokens,
	})
	if err != nil {
		return nil, fmt.Errorf("minimax: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("minimax: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("minimax: http do: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		return nil, fmt.Errorf("minimax: upstream HTTP %d: %s", resp.StatusCode, string(body))
	}

	ch := make(chan chatgpt.SSEEvent, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(line[5:])
			if payload == "[DONE]" {
				break
			}
			ch <- chatgpt.SSEEvent{Data: []byte(payload)}
		}
		if err := scanner.Err(); err != nil {
			ch <- chatgpt.SSEEvent{Err: err}
		}
	}()
	return ch, nil
}

// Chat 发起非流式对话,返回完整的 assistant 消息内容。
func (c *Client) Chat(ctx context.Context, model string, messages []chatgpt.ChatMessage, maxTokens int) (string, int, int, error) {
	body, err := json.Marshal(chatRequest{
		Model:     model,
		Messages:  messages,
		Stream:    false,
		MaxTokens: maxTokens,
	})
	if err != nil {
		return "", 0, 0, fmt.Errorf("minimax: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", 0, 0, fmt.Errorf("minimax: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", 0, 0, fmt.Errorf("minimax: http do: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", 0, 0, fmt.Errorf("minimax: upstream HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", 0, 0, fmt.Errorf("minimax: decode response: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", 0, 0, fmt.Errorf("minimax: empty choices in response")
	}
	return out.Choices[0].Message.Content,
		out.Usage.PromptTokens,
		out.Usage.CompletionTokens,
		nil
}
