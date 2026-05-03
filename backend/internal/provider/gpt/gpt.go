// Package gpt 实现 OpenAI 兼容的图像生成 provider（GPT 账号池 → /v1/images/generations）。
//
// 协议：完全对齐 OpenAI Images API，可对接 OpenAI 官方 / Azure / 任意网关。
//
//	POST {base_url}/v1/images/generations
//	Header: Authorization: Bearer {api_key}
//	Body  : {"model","prompt","n","size","response_format"}
//	Resp  : {"created":int,"data":[{"url":"..."} | {"b64_json":"..."}]}
//
// 错误处理：
//   - 4xx 标记账号失败并 30s 冷却（避免雪崩）；
//   - 5xx 标记账号失败并 5min 冷却；
//   - 超时同上。
package gpt

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kleinai/backend/internal/provider"
	"github.com/kleinai/backend/pkg/outbound"
	"golang.org/x/crypto/sha3"
)

const (
	defaultBaseURL = "https://api.openai.com"
	defaultTimeout = 6 * time.Minute
)

// Provider 实现 provider.Provider。
type Provider struct {
	client     *http.Client
	defaultURL string
	name       string
}

// New 构造。defaultBase 为空时使用 OpenAI 官方域名。
func New(defaultBase string) *Provider {
	if defaultBase == "" {
		defaultBase = defaultBaseURL
	}
	return &Provider{
		client: &http.Client{
			Timeout: defaultTimeout,
		},
		defaultURL: strings.TrimRight(defaultBase, "/"),
		name:       "gpt",
	}
}

// Name impl。
func (p *Provider) Name() string { return p.name }

type imgReq struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	N              int    `json:"n,omitempty"`
	Size           string `json:"size,omitempty"`
	Quality        string `json:"quality,omitempty"`
	Style          string `json:"style,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
}

type imgRespItem struct {
	URL     string `json:"url"`
	B64JSON string `json:"b64_json,omitempty"`
}
type imgResp struct {
	Created int           `json:"created"`
	Data    []imgRespItem `json:"data"`
	Error   *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

type responseInputItem struct {
	Type     string           `json:"type"`
	Role     string           `json:"role"`
	Content  []map[string]any `json:"content"`
	MetaData map[string]any   `json:"metadata,omitempty"`
}

type responseReq struct {
	Instructions      string           `json:"instructions"`
	Stream            bool             `json:"stream"`
	Reasoning         map[string]any   `json:"reasoning,omitempty"`
	ParallelToolCalls bool             `json:"parallel_tool_calls"`
	Include           []string         `json:"include,omitempty"`
	Model             string           `json:"model"`
	Store             bool             `json:"store"`
	ToolChoice        any              `json:"tool_choice,omitempty"`
	Input             any              `json:"input"`
	Tools             []map[string]any `json:"tools"`
}

type responseCompletedEvent struct {
	Type     string `json:"type"`
	Response struct {
		Output []responseOutputItem `json:"output"`
	} `json:"response"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

type responseOutputItem struct {
	Type          string `json:"type"`
	Result        string `json:"result"`
	B64JSON       string `json:"b64_json"`
	ImageB64      string `json:"image_b64"`
	URL           string `json:"url"`
	OutputFormat  string `json:"output_format"`
	Size          string `json:"size"`
	RevisedPrompt string `json:"revised_prompt"`
	Content       []struct {
		Type     string `json:"type"`
		Result   string `json:"result"`
		B64JSON  string `json:"b64_json"`
		ImageB64 string `json:"image_b64"`
		URL      string `json:"url"`
	} `json:"content"`
}

// Generate impl。仅支持 KindImage。
func (p *Provider) Generate(ctx context.Context, req *provider.Request) (*provider.Result, error) {
	if req.Kind != provider.KindImage {
		return nil, fmt.Errorf("gpt provider only supports image kind, got %s", req.Kind)
	}
	if req.Credential == "" {
		return nil, fmt.Errorf("gpt provider missing credential")
	}
	if isGPTImage2(req.ModelCode) {
		if shouldUseWebImage2(req) {
			return p.generateImage2Web(ctx, req)
		}
		return p.generateImage2(ctx, req)
	}

	base := req.BaseURL
	if base == "" {
		base = p.defaultURL
	}
	base = strings.TrimRight(base, "/")
	url := base + "/v1/images/generations"

	count := req.Count
	if count <= 0 {
		count = 1
	}

	body := imgReq{
		Model:          req.ModelCode,
		Prompt:         req.Prompt,
		N:              count,
		Size:           imageSize(req.Params, "1024x1024"),
		Quality:        strParam(req.Params, "quality", ""),
		Style:          strParam(req.Params, "style", ""),
		ResponseFormat: "url",
	}
	payload, _ := json.Marshal(body)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+req.Credential)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "kleinai/1.0")

	start := time.Now()
	client, err := p.httpClient(req.ProxyURL)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gpt http: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("gpt %d: %s", resp.StatusCode, snippet(raw, 240))
	}

	var out imgResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("gpt decode: %w (raw=%s)", err, snippet(raw, 240))
	}
	if out.Error != nil && out.Error.Message != "" {
		return nil, fmt.Errorf("gpt: %s", out.Error.Message)
	}
	if len(out.Data) == 0 {
		return nil, fmt.Errorf("gpt returned 0 image")
	}

	width, height := parseSize(body.Size)
	assets := make([]provider.Asset, 0, len(out.Data))
	for _, d := range out.Data {
		a := provider.Asset{
			URL:    d.URL,
			Width:  width,
			Height: height,
			Mime:   "image/png",
		}
		if a.URL == "" && d.B64JSON != "" {
			// 大多数网关会直接给 URL；b64 模式 caller 应自行落 OSS 后再回填。
			a.URL = "data:image/png;base64," + d.B64JSON
		}
		assets = append(assets, a)
	}

	return &provider.Result{
		TaskID:  req.TaskID,
		Assets:  assets,
		Latency: time.Since(start),
	}, nil
}

type webRequirement struct {
	Token      string
	ProofToken string
	SOToken    string
}

type webUploadMeta struct {
	FileID        string
	LibraryFileID string
	FileName      string
	FileSize      int
	Mime          string
	Width         int
	Height        int
}

func (p *Provider) generateImage2Web(ctx context.Context, req *provider.Request) (*provider.Result, error) {
	base := strings.TrimRight(req.BaseURL, "/")
	if base == "" || isCodexBase(base) || strings.Contains(base, "api.openai.com") {
		base = "https://chatgpt.com"
	}
	count := req.Count
	if count <= 0 {
		count = 1
	}
	size := imageSize(req.Params, "1024x1024")
	ratio := webRatioFromSize(size, strParam(req.Params, "ratio", strParam(req.Params, "aspect_ratio", "1:1")))
	prompt := webImagePromptV2(req.Prompt, ratio, size)
	webModel := webImageModelSlug(req)
	client, err := p.httpClient(req.ProxyURL)
	if err != nil {
		return nil, err
	}
	fp := newWebFP()
	start := time.Now()
	logUpstream(ctx, req, provider.UpstreamLogEntry{
		Provider: "gpt",
		Stage:    "web.start",
		Meta: map[string]any{
			"route":     "chatgpt_web",
			"model":     webModel,
			"ratio":     ratio,
			"count":     count,
			"ref_count": len(req.RefAssets),
		},
	})
	if err := p.webBootstrap(ctx, client, base, fp); err != nil {
		logUpstream(ctx, req, provider.UpstreamLogEntry{Provider: "gpt", Stage: "web.bootstrap", Method: "GET", URL: base + "/", Error: err.Error()})
		return nil, err
	}
	reqs, err := p.webRequirements(ctx, client, base, fp, req.Credential)
	if err != nil {
		logUpstream(ctx, req, provider.UpstreamLogEntry{Provider: "gpt", Stage: "web.requirements", Method: "POST", URL: base + "/backend-api/sentinel/chat-requirements", Error: err.Error()})
		return nil, err
	}
	logUpstream(ctx, req, provider.UpstreamLogEntry{
		Provider: "gpt",
		Stage:    "web.requirements",
		Method:   "POST",
		URL:      base + "/backend-api/sentinel/chat-requirements",
		Meta:     map[string]any{"has_token": reqs.Token != "", "has_proof_token": reqs.ProofToken != "", "has_so_token": reqs.SOToken != ""},
	})
	refs := make([]webUploadMeta, 0, len(req.RefAssets))
	for i, ref := range req.RefAssets {
		meta, err := p.webUploadImage(ctx, client, base, fp, req.Credential, strings.TrimSpace(ref), fmt.Sprintf("image_%d.png", i+1))
		if err != nil {
			logUpstream(ctx, req, provider.UpstreamLogEntry{Provider: "gpt", Stage: "web.upload", Method: "POST", URL: base + "/backend-api/files", Error: err.Error(), Meta: map[string]any{"ref_index": i + 1}})
			return nil, err
		}
		logUpstream(ctx, req, provider.UpstreamLogEntry{
			Provider: "gpt",
			Stage:    "web.upload",
			Method:   "POST",
			URL:      base + "/backend-api/files",
			Meta: map[string]any{
				"file_id":   meta.FileID,
				"mime":      meta.Mime,
				"size":      meta.FileSize,
				"width":     meta.Width,
				"height":    meta.Height,
				"ref_index": i + 1,
			},
		})
		refs = append(refs, meta)
	}
	width, height := parseSize(size)
	assets := make([]provider.Asset, 0, count)
	lastDiag := ""
	for i := 0; i < count && len(assets) < count; i++ {
		conduit, err := p.webPrepareImageConversation(ctx, client, base, fp, req.Credential, reqs, prompt, webModel, refs)
		if err != nil {
			logUpstream(ctx, req, provider.UpstreamLogEntry{Provider: "gpt", Stage: "web.prepare", Method: "POST", URL: base + "/backend-api/f/conversation/prepare", Error: err.Error()})
			return nil, err
		}
		logUpstream(ctx, req, provider.UpstreamLogEntry{Provider: "gpt", Stage: "web.prepare", Method: "POST", URL: base + "/backend-api/f/conversation/prepare", Meta: map[string]any{"has_conduit": conduit != ""}})
		conversationID, fileIDs, sedimentIDs, directURLs, lastText, err := p.webStartImageGeneration(ctx, client, base, fp, req.Credential, reqs, conduit, prompt, webModel, refs)
		if err != nil {
			logUpstream(ctx, req, provider.UpstreamLogEntry{Provider: "gpt", Stage: "web.conversation", Method: "POST", URL: base + "/backend-api/f/conversation", Error: err.Error()})
			return nil, err
		}
		fileIDs, sedimentIDs, directURLs = filterWebGeneratedAssetIDs(fileIDs, sedimentIDs, directURLs, refs)
		logUpstream(ctx, req, provider.UpstreamLogEntry{
			Provider:        "gpt",
			Stage:           "web.conversation",
			Method:          "POST",
			URL:             base + "/backend-api/f/conversation",
			ResponseExcerpt: lastText,
			Meta: map[string]any{
				"conversation_id": conversationID,
				"file_ids":        fileIDs,
				"sediment_ids":    sedimentIDs,
				"direct_urls":     len(directURLs),
			},
		})
		var urls, downloadErrs []string
		deadline := time.Now().Add(9 * time.Minute)
		pollCount := 0
		for {
			if conversationID != "" {
				pollFileIDs, pollSedimentIDs, pollURLs, _ := p.webConversationImageIDs(ctx, client, base, fp, req.Credential, conversationID, refs)
				pollCount++
				addUniqueString(&fileIDs, pollFileIDs...)
				addUniqueString(&sedimentIDs, pollSedimentIDs...)
				addUniqueString(&directURLs, pollURLs...)
				if pollCount == 1 || pollCount%6 == 0 {
					libFileIDs, _ := p.webLibraryImageIDs(ctx, client, base, fp, req.Credential, conversationID, refs)
					addUniqueString(&fileIDs, libFileIDs...)
				}
			}
			urls = p.webResolveImageURLs(ctx, client, base, fp, req.Credential, conversationID, fileIDs, sedimentIDs, refs)
			addUniqueWebAssetURLs(&urls, directURLs...)
			if pollCount == 1 || pollCount%12 == 0 {
				logUpstream(ctx, req, provider.UpstreamLogEntry{
					Provider:        "gpt",
					Stage:           "web.poll",
					ResponseExcerpt: snippet([]byte(lastText), 160),
					Meta: map[string]any{
						"poll_count":      pollCount,
						"conversation_id": conversationID,
						"file_ids":        len(fileIDs),
						"sediment_ids":    len(sedimentIDs),
						"direct_urls":     len(directURLs),
						"resolved_urls":   len(urls),
						"download_errors": len(downloadErrs),
					},
				})
			}
			for _, u := range urls {
				dataURL, mime, err := p.webDownloadAsDataURL(ctx, client, base, fp, req.Credential, u)
				if err != nil {
					errText := fmt.Sprintf("%s: %v", sanitizeDiagURL(u), err)
					before := len(downloadErrs)
					addUniqueString(&downloadErrs, errText)
					if len(downloadErrs) > before && len(downloadErrs) <= 3 {
						logUpstream(ctx, req, provider.UpstreamLogEntry{Provider: "gpt", Stage: "web.download", Method: "GET", URL: sanitizeDiagURL(u), Error: errText})
					}
					continue
				}
				logUpstream(ctx, req, provider.UpstreamLogEntry{
					Provider: "gpt",
					Stage:    "web.download",
					Method:   "GET",
					URL:      sanitizeDiagURL(u),
					Meta:     map[string]any{"mime": mime, "poll_count": pollCount},
				})
				assets = append(assets, provider.Asset{
					URL:    dataURL,
					Width:  width,
					Height: height,
					Mime:   mime,
					Meta:   map[string]any{"provider_route": "chatgpt_web", "size": "1K", "ratio": ratio},
				})
				if len(assets) >= count {
					break
				}
			}
			if len(assets) >= count || conversationID == "" || time.Now().After(deadline) {
				break
			}
			select {
			case <-ctx.Done():
				lastDiag = webImage2Diag(conversationID, fileIDs, sedimentIDs, directURLs, urls, downloadErrs, lastText)
				logUpstream(ctx, req, provider.UpstreamLogEntry{
					Provider:        "gpt",
					Stage:           "web.wait_timeout",
					ResponseExcerpt: lastDiag,
					Error:           ctx.Err().Error(),
					Meta: map[string]any{
						"poll_count":      pollCount,
						"asset_count":     len(assets),
						"resolved_urls":   len(urls),
						"download_errors": downloadErrs,
					},
				})
				return nil, fmt.Errorf("gpt image2 web wait: %w", ctx.Err())
			case <-time.After(5 * time.Second):
			}
		}
		lastDiag = webImage2Diag(conversationID, fileIDs, sedimentIDs, directURLs, urls, downloadErrs, lastText)
		logUpstream(ctx, req, provider.UpstreamLogEntry{
			Provider:        "gpt",
			Stage:           "web.resolve",
			ResponseExcerpt: lastDiag,
			Meta: map[string]any{
				"poll_count":      pollCount,
				"resolved_urls":   len(urls),
				"download_errors": downloadErrs,
				"asset_count":     len(assets),
			},
		})
		if len(assets) == 0 && conversationID == "" && lastText != "" {
			return nil, fmt.Errorf("gpt image2 web produced text instead of image: %s", snippet([]byte(lastText), 220))
		}
	}
	if len(assets) == 0 {
		if lastDiag != "" {
			logUpstream(ctx, req, provider.UpstreamLogEntry{Provider: "gpt", Stage: "web.failed", ResponseExcerpt: lastDiag})
			return nil, fmt.Errorf("gpt image2 web returned 0 image (%s)", lastDiag)
		}
		logUpstream(ctx, req, provider.UpstreamLogEntry{Provider: "gpt", Stage: "web.failed", ResponseExcerpt: "gpt image2 web returned 0 image"})
		return nil, fmt.Errorf("gpt image2 web returned 0 image")
	}
	return &provider.Result{TaskID: req.TaskID, Assets: assets, Latency: time.Since(start)}, nil
}

func (p *Provider) generateImage2(ctx context.Context, req *provider.Request) (*provider.Result, error) {
	base := strings.TrimRight(req.BaseURL, "/")
	if base == "" {
		base = p.defaultURL
	}
	url := responseEndpoint(base)
	count := req.Count
	if count <= 0 {
		count = 1
	}
	modelCode := req.ModelCode
	mainModel := strParam(req.Params, "main_model", mainModelForImage2(modelCode))
	toolModel := imageToolModel(modelCode)
	size := imageSize(req.Params, "1024x1024")
	action := "generate"
	if req.Mode == provider.ModeI2I || len(req.RefAssets) > 0 || strings.EqualFold(strParam(req.Params, "operation", ""), "edit") {
		action = "edit"
	}
	content := []map[string]any{{"type": "input_text", "text": req.Prompt}}
	for _, ref := range req.RefAssets {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		content = append(content, map[string]any{"type": "input_image", "image_url": ref})
	}
	input := []responseInputItem{{Type: "message", Role: "user", Content: content}}
	tool := map[string]any{
		"type":   "image_generation",
		"action": action,
		"model":  toolModel,
		"size":   size,
	}
	if quality := imageQuality(req.Params); quality != "" {
		tool["quality"] = quality
	}
	copyParam(tool, req.Params, "background")
	copyParam(tool, req.Params, "output_format")
	copyParam(tool, req.Params, "output_compression")
	copyParam(tool, req.Params, "partial_images")
	copyParam(tool, req.Params, "moderation")
	copyParam(tool, req.Params, "input_fidelity")
	if mask := firstStringParam(req.Params, "mask", "mask_image_url"); mask != "" {
		tool["input_image_mask"] = map[string]string{"image_url": mask}
	}
	body := responseReq{
		Instructions:      "You are an image generation assistant. Follow the user's prompt and return the generated image.",
		Stream:            true,
		Reasoning:         map[string]any{"effort": "medium", "summary": "auto"},
		ParallelToolCalls: true,
		Include:           []string{"reasoning.encrypted_content"},
		Model:             mainModel,
		Store:             false,
		ToolChoice:        "auto",
		Input:             input,
		Tools:             []map[string]any{tool},
	}

	start := time.Now()
	client, err := p.httpClient(req.ProxyURL)
	if err != nil {
		return nil, err
	}
	width, height := parseSize(size)
	assets := make([]provider.Asset, 0, count)
	logUpstream(ctx, req, provider.UpstreamLogEntry{
		Provider: "gpt",
		Stage:    "codex.start",
		Method:   "POST",
		URL:      url,
		Meta: map[string]any{
			"model":          modelCode,
			"main_model":     mainModel,
			"tool_model":     toolModel,
			"size":           size,
			"count":          count,
			"action":         action,
			"ref_count":      len(req.RefAssets),
			"proxy":          req.ProxyURL != "",
			"has_toolchoice": true,
		},
	})
	for i := 0; i < count && len(assets) < count; i++ {
		attemptBody := body
		retriedWithoutToolChoice := false
		for {
			payload, _ := json.Marshal(attemptBody)
			httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
			if err != nil {
				return nil, err
			}
			httpReq.Header.Set("Authorization", "Bearer "+req.Credential)
			httpReq.Header.Set("Content-Type", "application/json")
			httpReq.Header.Set("Accept", "text/event-stream")
			httpReq.Header.Set("User-Agent", userAgentForEndpoint(url))
			if isCodexEndpoint(url) {
				httpReq.Header.Set("Originator", "codex-tui")
				httpReq.Header.Set("Connection", "Keep-Alive")
			}
			resp, err := client.Do(httpReq)
			if err != nil {
				logUpstream(ctx, req, provider.UpstreamLogEntry{
					Provider:       "gpt",
					Stage:          "codex.request",
					Method:         "POST",
					URL:            url,
					RequestExcerpt: snippet(payload, 600),
					Error:          err.Error(),
					Meta: map[string]any{
						"model":      modelCode,
						"size":       size,
						"count":      count,
						"tool_model": toolModel,
						"action":     action,
					},
				})
				return nil, fmt.Errorf("gpt image2 http: %w", err)
			}
			if resp.StatusCode >= 400 {
				raw, _ := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if retriedWithoutToolChoice {
					logUpstream(ctx, req, provider.UpstreamLogEntry{
						Provider:        "gpt",
						Stage:           "codex.response",
						Method:          "POST",
						URL:             url,
						StatusCode:      resp.StatusCode,
						RequestExcerpt:  snippet(payload, 600),
						ResponseExcerpt: snippet(raw, 600),
						Meta: map[string]any{
							"model":      modelCode,
							"size":       size,
							"count":      count,
							"tool_model": toolModel,
							"action":     action,
						},
					})
				}
				if !retriedWithoutToolChoice && shouldRetryImage2WithoutToolChoice(raw) {
					logUpstream(ctx, req, provider.UpstreamLogEntry{
						Provider:        "gpt",
						Stage:           "codex.retry",
						Method:          "POST",
						URL:             url,
						StatusCode:      resp.StatusCode,
						RequestExcerpt:  snippet(payload, 600),
						ResponseExcerpt: snippet(raw, 600),
						Meta: map[string]any{
							"reason": "tool_choice_fallback",
						},
					})
					attemptBody.ToolChoice = nil
					retriedWithoutToolChoice = true
					continue
				}
				logUpstream(ctx, req, provider.UpstreamLogEntry{
					Provider:        "gpt",
					Stage:           "codex.failed",
					Method:          "POST",
					URL:             url,
					StatusCode:      resp.StatusCode,
					RequestExcerpt:  snippet(payload, 600),
					ResponseExcerpt: snippet(raw, 600),
					Meta: map[string]any{
						"model":      modelCode,
						"size":       size,
						"count":      count,
						"tool_model": toolModel,
						"action":     action,
					},
				})
				return nil, fmt.Errorf("gpt image2 %d: %s", resp.StatusCode, snippet(raw, 320))
			}
			completed, err := parseCompletedResponse(resp.Body)
			_ = resp.Body.Close()
			if err != nil {
				logUpstream(ctx, req, provider.UpstreamLogEntry{
					Provider:       "gpt",
					Stage:          "codex.decode",
					Method:         "POST",
					URL:            url,
					RequestExcerpt: snippet(payload, 600),
					Error:          err.Error(),
					Meta: map[string]any{
						"model":      modelCode,
						"size":       size,
						"count":      count,
						"tool_model": toolModel,
						"action":     action,
					},
				})
				return nil, err
			}
			if completed.Error != nil && completed.Error.Message != "" {
				logUpstream(ctx, req, provider.UpstreamLogEntry{
					Provider:        "gpt",
					Stage:           "codex.failed",
					Method:          "POST",
					URL:             url,
					RequestExcerpt:  snippet(payload, 600),
					ResponseExcerpt: completed.Error.Message,
					Meta: map[string]any{
						"model":      modelCode,
						"size":       size,
						"count":      count,
						"tool_model": toolModel,
						"action":     action,
					},
				})
				return nil, fmt.Errorf("gpt image2: %s", completed.Error.Message)
			}
			for _, out := range completed.Response.Output {
				imageData, imageURL := outputImagePayload(out)
				if out.Type != "image_generation_call" && imageData == "" && imageURL == "" {
					continue
				}
				mime := mimeForImageFormat(out.OutputFormat)
				assetWidth, assetHeight := width, height
				if out.Size != "" {
					assetWidth, assetHeight = parseSize(out.Size)
				}
				assetURL := imageURL
				if assetURL == "" {
					assetURL = "data:" + mime + ";base64," + imageData
				}
				assets = append(assets, provider.Asset{
					URL:    assetURL,
					Width:  assetWidth,
					Height: assetHeight,
					Mime:   mime,
					Meta:   map[string]any{"revised_prompt": out.RevisedPrompt, "provider_action": action, "size": size},
				})
				logUpstream(ctx, req, provider.UpstreamLogEntry{
					Provider:        "gpt",
					Stage:           "codex.asset",
					Method:          "POST",
					URL:             url,
					RequestExcerpt:  snippet(payload, 600),
					ResponseExcerpt: assetURL,
					Meta: map[string]any{
						"model":       modelCode,
						"size":        size,
						"count":       count,
						"tool_model":  toolModel,
						"action":      action,
						"asset_index": len(assets),
					},
				})
				if len(assets) >= count {
					break
				}
			}
			break
		}
	}
	if len(assets) == 0 {
		logUpstream(ctx, req, provider.UpstreamLogEntry{
			Provider:        "gpt",
			Stage:           "codex.failed",
			Method:          "POST",
			URL:             url,
			ResponseExcerpt: "gpt image2 returned 0 image",
			Meta: map[string]any{
				"model":      modelCode,
				"size":       size,
				"count":      count,
				"tool_model": toolModel,
				"action":     action,
			},
		})
		return nil, fmt.Errorf("gpt image2 returned 0 image")
	}
	logUpstream(ctx, req, provider.UpstreamLogEntry{
		Provider: "gpt",
		Stage:    "codex.success",
		Method:   "POST",
		URL:      url,
		Meta: map[string]any{
			"model":      modelCode,
			"size":       size,
			"count":      count,
			"tool_model": toolModel,
			"action":     action,
			"assets":     len(assets),
		},
	})
	return &provider.Result{TaskID: req.TaskID, Assets: assets, Latency: time.Since(start)}, nil
}

type webFP struct {
	UserAgent     string
	DeviceID      string
	SessionID     string
	ClientVersion string
	BuildNumber   string
	SecCHUA       string
}

func newWebFP() webFP {
	return webFP{
		UserAgent:     "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36 Edg/143.0.0.0",
		DeviceID:      uuid.NewString(),
		SessionID:     uuid.NewString(),
		ClientVersion: "prod-be885abbfcfe7b1f511e88b3003d9ee44757fbad",
		BuildNumber:   "5955942",
		SecCHUA:       `"Microsoft Edge";v="143", "Chromium";v="143", "Not A(Brand";v="24"`,
	}
}

func (p *Provider) webBootstrap(ctx context.Context, client *http.Client, base string, fp webFP) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/", nil)
	if err != nil {
		return err
	}
	for k, v := range webBaseHeaders(fp, "", "") {
		httpReq.Header.Set(k, v)
	}
	httpReq.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("gpt image2 web bootstrap: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 320))
		return fmt.Errorf("gpt image2 web bootstrap %d: %s", resp.StatusCode, string(raw))
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 512*1024))
	return nil
}

func (p *Provider) webRequirements(ctx context.Context, client *http.Client, base string, fp webFP, token string) (webRequirement, error) {
	path := "/backend-api/sentinel/chat-requirements"
	body := map[string]string{"p": buildLegacyRequirementsToken(fp.UserAgent)}
	payload, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, bytes.NewReader(payload))
	if err != nil {
		return webRequirement{}, err
	}
	for k, v := range webBaseHeaders(fp, token, path) {
		httpReq.Header.Set(k, v)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(httpReq)
	if err != nil {
		return webRequirement{}, fmt.Errorf("gpt image2 web requirements: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return webRequirement{}, fmt.Errorf("gpt image2 web requirements %d: %s", resp.StatusCode, snippet(raw, 320))
	}
	var out struct {
		Token       string `json:"token"`
		SOToken     string `json:"so_token"`
		ProofOfWork struct {
			Required   bool   `json:"required"`
			Seed       string `json:"seed"`
			Difficulty string `json:"difficulty"`
		} `json:"proofofwork"`
		Arkose struct {
			Required bool `json:"required"`
		} `json:"arkose"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return webRequirement{}, fmt.Errorf("gpt image2 web requirements decode: %w", err)
	}
	if out.Arkose.Required {
		return webRequirement{}, fmt.Errorf("gpt image2 web requires arkose")
	}
	if out.Token == "" {
		return webRequirement{}, fmt.Errorf("gpt image2 web requirements missing token")
	}
	proof := ""
	if out.ProofOfWork.Required && out.ProofOfWork.Seed != "" && out.ProofOfWork.Difficulty != "" {
		proof = buildProofToken(out.ProofOfWork.Seed, out.ProofOfWork.Difficulty, fp.UserAgent)
	}
	return webRequirement{Token: out.Token, ProofToken: proof, SOToken: out.SOToken}, nil
}

func (p *Provider) webPrepareImageConversation(ctx context.Context, client *http.Client, base string, fp webFP, token string, reqs webRequirement, prompt, modelSlug string, refs []webUploadMeta) (string, error) {
	path := "/backend-api/f/conversation/prepare"
	body := map[string]any{
		"action":                 "next",
		"fork_from_shared_post":  false,
		"parent_message_id":      "client-created-root",
		"model":                  modelSlug,
		"client_prepare_state":   "none",
		"timezone_offset_min":    -480,
		"timezone":               "Asia/Shanghai",
		"conversation_mode":      map[string]any{"kind": "primary_assistant"},
		"system_hints":           []string{"picture_v2"},
		"attachment_mime_types":  []string{"image/png"},
		"supports_buffering":     true,
		"supported_encodings":    []string{"v1"},
		"client_contextual_info": map[string]any{"app_name": "chatgpt.com"},
		"thinking_effort":        "standard",
	}
	payload, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	for k, v := range webImageHeaders(fp, token, path, reqs, "", "*/*") {
		httpReq.Header.Set(k, v)
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("gpt image2 web prepare: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("gpt image2 web prepare %d: %s", resp.StatusCode, snippet(raw, 320))
	}
	var out struct {
		ConduitToken string `json:"conduit_token"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("gpt image2 web prepare decode: %w", err)
	}
	if out.ConduitToken == "" {
		return "", fmt.Errorf("gpt image2 web prepare missing conduit token")
	}
	return out.ConduitToken, nil
}

func (p *Provider) webStartImageGeneration(ctx context.Context, client *http.Client, base string, fp webFP, token string, reqs webRequirement, conduit, prompt, modelSlug string, refs []webUploadMeta) (string, []string, []string, []string, string, error) {
	path := "/backend-api/f/conversation"
	content, metadata := webImageMessageContent(prompt, refs)
	messageID := uuid.NewString()
	body := map[string]any{
		"action":                   "next",
		"fork_from_shared_post":    false,
		"parent_message_id":        "client-created-root",
		"model":                    modelSlug,
		"client_prepare_state":     "success",
		"timezone_offset_min":      -480,
		"timezone":                 "Asia/Shanghai",
		"conversation_mode":        map[string]any{"kind": "primary_assistant"},
		"enable_message_followups": true,
		"system_hints":             []string{},
		"supports_buffering":       true,
		"supported_encodings":      []string{"v1"},
		"client_contextual_info": map[string]any{
			"is_dark_mode": false, "time_since_loaded": 51, "page_height": 1111, "page_width": 1731,
			"pixel_ratio": 1.5, "screen_height": 1440, "screen_width": 2560, "app_name": "chatgpt.com",
		},
		"paragen_cot_summary_display_override": "allow",
		"force_parallel_switch":                "auto",
		"thinking_effort":                      "standard",
		"messages": []map[string]any{{
			"id":          messageID,
			"author":      map[string]string{"role": "user"},
			"create_time": time.Now().Unix(),
			"content":     content,
			"metadata":    metadata,
		}},
	}
	payload, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, bytes.NewReader(payload))
	if err != nil {
		return "", nil, nil, nil, "", err
	}
	for k, v := range webImageHeaders(fp, token, path, reqs, conduit, "text/event-stream") {
		httpReq.Header.Set(k, v)
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", nil, nil, nil, "", fmt.Errorf("gpt image2 web conversation: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return "", nil, nil, nil, "", fmt.Errorf("gpt image2 web conversation %d: %s", resp.StatusCode, snippet(raw, 320))
	}
	conversationID, fileIDs, sedimentIDs, directURLs, lastText, err := parseWebImageSSE(resp.Body)
	if err != nil {
		return "", nil, nil, nil, "", err
	}
	return conversationID, fileIDs, sedimentIDs, directURLs, lastText, nil
}

func webImageMessageContent(prompt string, refs []webUploadMeta) (map[string]any, map[string]any) {
	parts := make([]any, 0, len(refs)+1)
	attachments := make([]map[string]any, 0, len(refs))
	for _, ref := range refs {
		parts = append(parts, map[string]any{
			"content_type":  "image_asset_pointer",
			"asset_pointer": "sediment://file_" + strings.TrimPrefix(ref.FileID, "file_"),
			"width":         ref.Width,
			"height":        ref.Height,
			"size_bytes":    ref.FileSize,
		})
		attachment := map[string]any{
			"id":           ref.FileID,
			"mime_type":    ref.Mime,
			"name":         ref.FileName,
			"size":         ref.FileSize,
			"width":        ref.Width,
			"height":       ref.Height,
			"source":       "library",
			"is_big_paste": false,
		}
		if ref.LibraryFileID != "" {
			attachment["library_file_id"] = ref.LibraryFileID
		}
		attachments = append(attachments, attachment)
	}
	if len(refs) > 0 {
		parts = append(parts, prompt)
	}
	content := map[string]any{"content_type": "text", "parts": []string{prompt}}
	if len(refs) > 0 {
		content = map[string]any{"content_type": "multimodal_text", "parts": parts}
	}
	metadata := map[string]any{
		"developer_mode_connector_ids": []string{},
		"selected_github_repos":        []string{},
		"selected_all_github_repos":    false,
		"system_hints":                 []string{"picture_v2"},
		"serialization_metadata":       map[string]any{"custom_symbol_offsets": []any{}},
	}
	if len(attachments) > 0 {
		metadata["attachments"] = attachments
	}
	return content, metadata
}

func (p *Provider) webPollImageResults(ctx context.Context, client *http.Client, base string, fp webFP, token, conversationID string, timeout time.Duration, refs []webUploadMeta) ([]string, []string, []string, error) {
	if conversationID == "" {
		return nil, nil, nil, nil
	}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		fileIDs, sedimentIDs, directURLs, err := p.webConversationImageIDs(ctx, client, base, fp, token, conversationID, refs)
		if err == nil && (len(fileIDs) > 0 || len(sedimentIDs) > 0 || len(directURLs) > 0) {
			return fileIDs, sedimentIDs, directURLs, nil
		}
		lastErr = err
		time.Sleep(4 * time.Second)
	}
	return nil, nil, nil, lastErr
}

func (p *Provider) webConversationImageIDs(ctx context.Context, client *http.Client, base string, fp webFP, token, conversationID string, refs []webUploadMeta) ([]string, []string, []string, error) {
	path := "/backend-api/conversation/" + conversationID
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return nil, nil, nil, err
	}
	for k, v := range webBaseHeaders(fp, token, path) {
		httpReq.Header.Set(k, v)
	}
	httpReq.Header.Set("Accept", "application/json")
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, nil, nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, nil, nil, fmt.Errorf("gpt image2 web poll %d: %s", resp.StatusCode, snippet(raw, 240))
	}
	fileIDs, sedimentIDs := extractWebImageToolIDs(raw)
	_, _, _, directURLs := extractWebImageIDs(string(raw))
	fileIDs, sedimentIDs, directURLs = filterWebGeneratedAssetIDs(fileIDs, sedimentIDs, directURLs, refs)
	return fileIDs, sedimentIDs, directURLs, nil
}

func (p *Provider) webLibraryImageIDs(ctx context.Context, client *http.Client, base string, fp webFP, token, conversationID string, refs []webUploadMeta) ([]string, error) {
	path := "/backend-api/files/library"
	body := map[string]any{"limit": 20, "cursor": nil}
	payload, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	for k, v := range webBaseHeaders(fp, token, path) {
		httpReq.Header.Set(k, v)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("gpt image2 web library %d: %s", resp.StatusCode, snippet(raw, 240))
	}
	var out struct {
		Items []struct {
			FileID               string `json:"file_id"`
			MimeType             string `json:"mime_type"`
			LibraryFileCategory  string `json:"library_file_category"`
			State                string `json:"state"`
			OriginationThreadID  string `json:"origination_thread_id"`
			OriginationMessageID string `json:"origination_message_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	var ids []string
	for _, item := range out.Items {
		if item.FileID == "" || item.OriginationThreadID != conversationID {
			continue
		}
		if item.State != "" && !strings.EqualFold(item.State, "ready") {
			continue
		}
		if item.LibraryFileCategory != "" && !strings.EqualFold(item.LibraryFileCategory, "image") {
			continue
		}
		if item.MimeType != "" && !strings.HasPrefix(strings.ToLower(item.MimeType), "image/") {
			continue
		}
		addUniqueString(&ids, item.FileID)
	}
	ids, _, _ = filterWebGeneratedAssetIDs(ids, nil, nil, refs)
	return ids, nil
}

func (p *Provider) webResolveImageURLs(ctx context.Context, client *http.Client, base string, fp webFP, token, conversationID string, fileIDs, sedimentIDs []string, refs []webUploadMeta) []string {
	var out []string
	seen := map[string]bool{}
	exclude := map[string]bool{}
	for _, ref := range refs {
		if ref.FileID != "" {
			exclude[ref.FileID] = true
		}
		if ref.LibraryFileID != "" {
			exclude[ref.LibraryFileID] = true
		}
	}
	for _, id := range fileIDs {
		if id == "" || id == "file_upload" || seen["file:"+id] || exclude[id] {
			continue
		}
		seen["file:"+id] = true
		path := "/backend-api/files/download/" + id
		if conversationID != "" {
			path += "?conversation_id=" + url.QueryEscape(conversationID) + "&inline=false"
		}
		if u := p.webDownloadURL(ctx, client, base, fp, token, path); u != "" {
			out = append(out, u)
		}
	}
	if conversationID == "" {
		return out
	}
	for _, id := range sedimentIDs {
		if id == "" || seen["sed:"+id] || exclude[id] {
			continue
		}
		seen["sed:"+id] = true
		if u := p.webDownloadURL(ctx, client, base, fp, token, "/backend-api/conversation/"+conversationID+"/attachment/"+id+"/download"); u != "" {
			out = append(out, u)
		}
	}
	return out
}

func filterWebGeneratedAssetIDs(fileIDs, sedimentIDs, directURLs []string, refs []webUploadMeta) ([]string, []string, []string) {
	exclude := map[string]bool{}
	for _, ref := range refs {
		if ref.FileID != "" {
			exclude[ref.FileID] = true
		}
		if ref.LibraryFileID != "" {
			exclude[ref.LibraryFileID] = true
		}
	}
	filter := func(in []string) []string {
		out := make([]string, 0, len(in))
		seen := map[string]bool{}
		for _, v := range in {
			if v == "" || exclude[v] || seen[v] {
				continue
			}
			seen[v] = true
			out = append(out, v)
		}
		return out
	}
	return filter(fileIDs), filter(sedimentIDs), filter(directURLs)
}

func (p *Provider) webDownloadURL(ctx context.Context, client *http.Client, base string, fp webFP, token, path string) string {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return ""
	}
	for k, v := range webBaseHeaders(fp, token, path) {
		httpReq.Header.Set(k, v)
	}
	httpReq.Header.Set("Accept", "application/json")
	resp, err := client.Do(httpReq)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return ""
	}
	var out map[string]any
	if json.Unmarshal(raw, &out) != nil {
		return ""
	}
	for _, k := range []string{"download_url", "url"} {
		if s, ok := out[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func (p *Provider) webDownloadAsDataURL(ctx context.Context, client *http.Client, base string, fp webFP, token, rawURL string) (string, string, error) {
	downloadURL := rawURL
	if strings.HasPrefix(downloadURL, "/") {
		downloadURL = strings.TrimRight(base, "/") + downloadURL
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return "", "", err
	}
	if shouldUseWebDownloadHeaders(base, downloadURL) {
		targetPath := "/"
		if parsed, err := url.Parse(downloadURL); err == nil && parsed.Path != "" {
			targetPath = parsed.Path
		}
		for k, v := range webBaseHeaders(fp, token, targetPath) {
			httpReq.Header.Set(k, v)
		}
		httpReq.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode >= 400 {
		return "", "", fmt.Errorf("download image %d: %s", resp.StatusCode, snippet(data, 160))
	}
	if len(data) == 0 {
		return "", "", fmt.Errorf("download image empty body")
	}
	mime := resp.Header.Get("Content-Type")
	if idx := strings.Index(mime, ";"); idx >= 0 {
		mime = mime[:idx]
	}
	if mime == "" || !strings.HasPrefix(mime, "image/") {
		mime = http.DetectContentType(data)
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), mime, nil
}

func (p *Provider) webUploadImage(ctx context.Context, client *http.Client, base string, fp webFP, token, ref, name string) (webUploadMeta, error) {
	data, mime, err := readRefImage(ctx, client, ref)
	if err != nil {
		return webUploadMeta{}, err
	}
	width, height := 1024, 1024
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
		width, height = cfg.Width, cfg.Height
	}
	path := "/backend-api/files"
	metaBody := map[string]any{"file_name": name, "file_size": len(data), "use_case": "multimodal", "width": width, "height": height}
	payload, _ := json.Marshal(metaBody)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, bytes.NewReader(payload))
	if err != nil {
		return webUploadMeta{}, err
	}
	for k, v := range webBaseHeaders(fp, token, path) {
		httpReq.Header.Set(k, v)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	resp, err := client.Do(httpReq)
	if err != nil {
		return webUploadMeta{}, err
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode >= 400 {
		return webUploadMeta{}, fmt.Errorf("gpt image2 web upload meta %d: %s", resp.StatusCode, snippet(raw, 240))
	}
	var meta struct {
		FileID    string `json:"file_id"`
		UploadURL string `json:"upload_url"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return webUploadMeta{}, err
	}
	if meta.FileID == "" || meta.UploadURL == "" {
		return webUploadMeta{}, fmt.Errorf("gpt image2 web upload missing file metadata")
	}
	putReq, err := http.NewRequestWithContext(ctx, http.MethodPut, meta.UploadURL, bytes.NewReader(data))
	if err != nil {
		return webUploadMeta{}, err
	}
	putReq.Header.Set("Content-Type", mime)
	putReq.Header.Set("x-ms-blob-type", "BlockBlob")
	putReq.Header.Set("x-ms-version", "2020-04-08")
	putReq.Header.Set("Origin", base)
	putReq.Header.Set("Referer", base+"/")
	putReq.Header.Set("User-Agent", fp.UserAgent)
	resp, err = client.Do(putReq)
	if err != nil {
		return webUploadMeta{}, err
	}
	raw, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode >= 400 {
		return webUploadMeta{}, fmt.Errorf("gpt image2 web upload blob %d: %s", resp.StatusCode, snippet(raw, 240))
	}
	path = "/backend-api/files/" + meta.FileID + "/uploaded"
	doneReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, strings.NewReader("{}"))
	if err != nil {
		return webUploadMeta{}, err
	}
	for k, v := range webBaseHeaders(fp, token, path) {
		doneReq.Header.Set(k, v)
	}
	doneReq.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(doneReq)
	if err != nil {
		return webUploadMeta{}, err
	}
	raw, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode >= 400 {
		return webUploadMeta{}, fmt.Errorf("gpt image2 web upload confirm %d: %s", resp.StatusCode, snippet(raw, 240))
	}
	libraryFileID, err := p.webProcessUploadStream(ctx, client, base, fp, token, meta.FileID, name)
	if err != nil {
		return webUploadMeta{}, err
	}
	return webUploadMeta{FileID: meta.FileID, LibraryFileID: libraryFileID, FileName: name, FileSize: len(data), Mime: mime, Width: width, Height: height}, nil
}

func (p *Provider) webProcessUploadStream(ctx context.Context, client *http.Client, base string, fp webFP, token, fileID, fileName string) (string, error) {
	path := "/backend-api/files/process_upload_stream"
	body := map[string]any{
		"file_id":                  fileID,
		"use_case":                 "multimodal",
		"index_for_retrieval":      false,
		"file_name":                fileName,
		"library_persistence_mode": "opportunistic",
		"metadata":                 map[string]any{"store_in_library": true},
		"entry_surface":            "chat_composer",
	}
	payload, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	for k, v := range webBaseHeaders(fp, token, path) {
		httpReq.Header.Set(k, v)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("gpt image2 web process upload %d: %s", resp.StatusCode, snippet(raw, 240))
	}
	libraryFileID := ""
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var ev struct {
			Extra struct {
				MetadataObjectID string `json:"metadata_object_id"`
			} `json:"extra"`
			Event string `json:"event"`
		}
		if json.Unmarshal([]byte(line), &ev) == nil && ev.Extra.MetadataObjectID != "" {
			libraryFileID = ev.Extra.MetadataObjectID
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return libraryFileID, nil
}

// === helpers ===

func strParam(p map[string]any, key, def string) string {
	if p == nil {
		return def
	}
	if v, ok := p[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return def
}

func (p *Provider) httpClient(proxyURL string) (*http.Client, error) {
	if strings.TrimSpace(proxyURL) == "" {
		return p.client, nil
	}
	return outbound.NewClient(outbound.Options{
		ProxyURL: proxyURL,
		Timeout:  defaultTimeout,
		Mode:     outbound.ModeUTLS,
		Profile:  outbound.ProfileChrome,
	})
}

func firstStringParam(p map[string]any, keys ...string) string {
	for _, key := range keys {
		if v := strParam(p, key, ""); v != "" {
			return v
		}
	}
	return ""
}

func copyParam(dst map[string]any, src map[string]any, key string) {
	if src == nil {
		return
	}
	if v, ok := src[key]; ok {
		switch t := v.(type) {
		case string:
			if t != "" {
				dst[key] = t
			}
		default:
			dst[key] = v
		}
	}
}

func shouldUseWebImage2(req *provider.Request) bool {
	tier := strings.ToUpper(strings.TrimSpace(strParam(req.Params, "resolution", strParam(req.Params, "size_tier", ""))))
	if tier == "" {
		size := strParam(req.Params, "size", "")
		w, h := parseSize(size)
		if size == "" || w*h <= 1500000 {
			return true
		}
		return false
	}
	return tier == "1K" || tier == "1"
}

func isGPTImage2(model string) bool {
	return imageToolModel(model) == "gpt-image-2"
}

func imageToolModel(model string) string {
	model = strings.TrimSpace(model)
	if idx := strings.LastIndex(model, "/"); idx >= 0 {
		model = model[idx+1:]
	}
	return model
}

func shouldRetryImage2WithoutToolChoice(raw []byte) bool {
	msg := strings.ToLower(string(raw))
	return strings.Contains(msg, "tool choice") &&
		strings.Contains(msg, "image_generation") &&
		strings.Contains(msg, "not found") &&
		strings.Contains(msg, "tools")
}

func webImage2Diag(conversationID string, fileIDs, sedimentIDs, directURLs, urls, downloadErrs []string, text string) string {
	return fmt.Sprintf("conversation_id=%s file_ids=%d sediment_ids=%d direct_urls=%d resolved_urls=%d download_errors=%d first_download_error=%s text=%s", conversationID, len(fileIDs), len(sedimentIDs), len(directURLs), len(urls), len(downloadErrs), firstString(downloadErrs), snippet([]byte(text), 120))
}

func mainModelForImage2(model string) string {
	model = strings.TrimSpace(model)
	if idx := strings.LastIndex(model, "/"); idx > 0 {
		return model[:idx] + "/gpt-5.5"
	}
	return "gpt-5.5"
}

func responseEndpoint(base string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if strings.HasSuffix(base, "/responses") {
		return base
	}
	if strings.Contains(base, "/backend-api/codex") {
		return base + "/responses"
	}
	return base + "/v1/responses"
}

func isCodexBase(base string) bool {
	return strings.Contains(strings.ToLower(base), "/backend-api/codex")
}

func isCodexEndpoint(url string) bool {
	return strings.Contains(strings.ToLower(url), "chatgpt.com/backend-api/codex")
}

func userAgentForEndpoint(url string) string {
	if isCodexEndpoint(url) {
		return "codex-tui/0.118.0 (Mac OS 26.3.1; arm64) iTerm.app/3.6.9 (codex-tui; 0.118.0)"
	}
	return "kleinai/1.0"
}

func imageSize(params map[string]any, def string) string {
	if size := strParam(params, "size", ""); size != "" {
		return size
	}
	ratio := strParam(params, "ratio", strParam(params, "aspect_ratio", "1:1"))
	tier := strings.ToUpper(strParam(params, "resolution", strParam(params, "size_tier", "1K")))
	sizes := map[string]map[string]string{
		"1K": {
			"1:1":  "1024x1024",
			"3:2":  "1216x832",
			"2:3":  "832x1216",
			"4:3":  "1152x864",
			"3:4":  "864x1152",
			"5:4":  "1120x896",
			"4:5":  "896x1120",
			"16:9": "1344x768",
			"9:16": "768x1344",
			"21:9": "1536x640",
		},
		"2K": {
			"1:1":  "1248x1248",
			"3:2":  "1536x1024",
			"2:3":  "1024x1536",
			"4:3":  "1440x1088",
			"3:4":  "1088x1440",
			"5:4":  "1392x1120",
			"4:5":  "1120x1392",
			"16:9": "1664x928",
			"9:16": "928x1664",
			"21:9": "1904x816",
		},
		"4K": {
			"1:1":  "2480x2480",
			"3:2":  "3056x2032",
			"2:3":  "2032x3056",
			"4:3":  "2880x2160",
			"3:4":  "2160x2880",
			"5:4":  "2784x2224",
			"4:5":  "2224x2784",
			"16:9": "3312x1872",
			"9:16": "1872x3312",
			"21:9": "3808x1632",
		},
	}
	if byRatio, ok := sizes[tier]; ok {
		if size := byRatio[ratio]; size != "" {
			return size
		}
		return byRatio["1:1"]
	}
	if byRatio := sizes["1K"]; byRatio != nil {
		if size := byRatio[ratio]; size != "" {
			return size
		}
	}
	return def
}

func imageQuality(params map[string]any) string {
	switch strings.ToLower(strParam(params, "quality", "")) {
	case "draft", "low":
		return "low"
	case "standard", "medium":
		return "medium"
	case "hd", "high":
		return "high"
	default:
		return ""
	}
}

func webImageModelSlug(req *provider.Request) string {
	if req != nil {
		if v := strParam(req.Params, "web_model", ""); v != "" {
			return v
		}
	}
	return "gpt-5-5-thinking"
}

func webBaseHeaders(fp webFP, token, path string) map[string]string {
	h := map[string]string{
		"User-Agent":                 fp.UserAgent,
		"Origin":                     "https://chatgpt.com",
		"Referer":                    "https://chatgpt.com/",
		"Accept-Language":            "zh-CN,zh;q=0.9,en;q=0.8,en-US;q=0.7",
		"Cache-Control":              "no-cache",
		"Pragma":                     "no-cache",
		"Priority":                   "u=1, i",
		"Sec-Ch-Ua":                  fp.SecCHUA,
		"Sec-Ch-Ua-Arch":             `"x86"`,
		"Sec-Ch-Ua-Bitness":          `"64"`,
		"Sec-Ch-Ua-Mobile":           "?0",
		"Sec-Ch-Ua-Model":            `""`,
		"Sec-Ch-Ua-Platform":         `"Windows"`,
		"Sec-Ch-Ua-Platform-Version": `"19.0.0"`,
		"Sec-Fetch-Dest":             "empty",
		"Sec-Fetch-Mode":             "cors",
		"Sec-Fetch-Site":             "same-origin",
		"OAI-Device-Id":              fp.DeviceID,
		"OAI-Session-Id":             fp.SessionID,
		"OAI-Language":               "zh-CN",
		"OAI-Client-Version":         fp.ClientVersion,
		"OAI-Client-Build-Number":    fp.BuildNumber,
		"X-OpenAI-Target-Path":       path,
		"X-OpenAI-Target-Route":      path,
	}
	if token != "" {
		h["Authorization"] = "Bearer " + token
	}
	return h
}

func webImageHeaders(fp webFP, token, path string, reqs webRequirement, conduit, accept string) map[string]string {
	h := webBaseHeaders(fp, token, path)
	h["Content-Type"] = "application/json"
	h["Accept"] = accept
	h["OpenAI-Sentinel-Chat-Requirements-Token"] = reqs.Token
	if reqs.ProofToken != "" {
		h["OpenAI-Sentinel-Proof-Token"] = reqs.ProofToken
	}
	if reqs.SOToken != "" {
		h["OpenAI-Sentinel-SO-Token"] = reqs.SOToken
	}
	if conduit != "" {
		h["X-Conduit-Token"] = conduit
	}
	if accept == "text/event-stream" {
		h["X-Oai-Turn-Trace-Id"] = uuid.NewString()
	}
	return h
}

func webImagePrompt(prompt, ratio string) string {
	prompt = strings.TrimSpace(prompt)
	ratio = strings.TrimSpace(ratio)
	if ratio == "" || ratio == "1:1" {
		return prompt
	}
	hints := map[string]string{
		"16:9": "输出一张 16:9 横屏构图的图片。",
		"9:16": "输出一张 9:16 竖屏构图的图片。",
		"4:3":  "输出一张 4:3 比例的图片。",
		"3:4":  "输出一张 3:4 竖向比例的图片。",
	}
	if h, ok := hints[ratio]; ok {
		return prompt + "\n\n" + h
	}
	return prompt + "\n\n输出图片，宽高比为 " + ratio + "。"
}

func webImagePromptV2(prompt, ratio, size string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		prompt = "生成一张高质量图片"
	}
	ratio = webRatioFromSize(size, ratio)
	ratio = strings.TrimSpace(ratio)
	if ratio == "" || ratio == "1:1" {
		return prompt
	}
	return prompt + "\n\n将宽高比设为 " + ratio
}

func webRatioFromSize(size, fallback string) string {
	size = strings.TrimSpace(size)
	if size == "" {
		return strings.TrimSpace(fallback)
	}
	switch size {
	case "1024x1024", "1248x1248", "2480x2480":
		return "1:1"
	case "1216x832", "1536x1024", "3056x2032":
		return "3:2"
	case "832x1216", "1024x1536", "2032x3056":
		return "2:3"
	case "1152x864", "1440x1088", "2880x2160":
		return "4:3"
	case "864x1152", "1088x1440", "2160x2880":
		return "3:4"
	case "1120x896", "1392x1120", "2784x2224":
		return "5:4"
	case "896x1120", "1120x1392", "2224x2784":
		return "4:5"
	case "1344x768", "1664x928", "3312x1872":
		return "16:9"
	case "768x1344", "928x1664", "1872x3312":
		return "9:16"
	case "1536x640", "1904x816", "3808x1632":
		return "21:9"
	default:
		return strings.TrimSpace(fallback)
	}
}

func readRefImage(ctx context.Context, client *http.Client, ref string) ([]byte, string, error) {
	if ref == "" {
		return nil, "", fmt.Errorf("empty reference image")
	}
	if strings.HasPrefix(ref, "data:") {
		header, data, ok := strings.Cut(ref, ",")
		if !ok {
			return nil, "", fmt.Errorf("invalid data url image")
		}
		raw, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return nil, "", err
		}
		mime := strings.TrimPrefix(strings.Split(strings.TrimPrefix(header, "data:"), ";")[0], "data:")
		if mime == "" {
			mime = http.DetectContentType(raw)
		}
		return raw, mime, nil
	}
	if strings.HasPrefix(ref, "/api/v1/gen/cached/") {
		rel := strings.TrimPrefix(ref, "/api/v1/gen/cached/")
		if rel == "" || strings.Contains(rel, "..") || strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, `\`) {
			return nil, "", fmt.Errorf("invalid cached reference image")
		}
		root := strings.TrimSpace(os.Getenv("KLEIN_STORAGE_ROOT"))
		if root == "" {
			root = "/app/storage/public"
		}
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return nil, "", fmt.Errorf("read cached reference image: %w", err)
		}
		if len(raw) == 0 {
			return nil, "", fmt.Errorf("empty cached reference image")
		}
		return raw, http.DetectContentType(raw), nil
	}
	u, err := url.Parse(ref)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, "", fmt.Errorf("reference image must be data/http url")
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, ref, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("reference image download %d: %s", resp.StatusCode, snippet(data, 160))
	}
	mime := resp.Header.Get("Content-Type")
	if idx := strings.Index(mime, ";"); idx >= 0 {
		mime = mime[:idx]
	}
	if mime == "" || !strings.HasPrefix(mime, "image/") {
		mime = http.DetectContentType(data)
	}
	return data, mime, nil
}

func parseWebImageSSE(r io.Reader) (string, []string, []string, []string, string, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	var dataLines []string
	conversationID := ""
	lastText := ""
	var fileIDs, sedimentIDs, directURLs []string
	flush := func() {
		if len(dataLines) == 0 {
			return
		}
		data := strings.TrimSpace(strings.Join(dataLines, "\n"))
		dataLines = nil
		if data == "" || data == "[DONE]" {
			return
		}
		cid, _, _, _ := extractWebImageIDs(data)
		if cid != "" && conversationID == "" {
			conversationID = cid
		}
		if toolFileIDs, toolSedimentIDs := extractWebImageToolIDs([]byte(data)); len(toolFileIDs) > 0 || len(toolSedimentIDs) > 0 {
			addUniqueString(&fileIDs, toolFileIDs...)
			addUniqueString(&sedimentIDs, toolSedimentIDs...)
		}
		addUniqueWebAssetURLs(&directURLs, extractWebImageDirectURLs(data)...)
		if text := extractWebAssistantText(data); text != "" {
			lastText = text
		}

		var ev responseCompletedEvent
		_ = json.Unmarshal([]byte(data), &ev)
		var direct struct {
			Output []responseOutputItem `json:"output"`
			Item   responseOutputItem   `json:"item"`
		}
		if err := json.Unmarshal([]byte(data), &direct); err == nil {
			if len(ev.Response.Output) == 0 && len(direct.Output) > 0 {
				ev.Type = "response.completed"
				ev.Response.Output = direct.Output
			}
			if direct.Item.Type != "" && ev.Type == "" {
				ev.Type = "response.output_item.done"
			}
		}
		switch ev.Type {
		case "response.output_item.done":
			if direct.Item.Type != "" {
				if dataURL, imageURL := outputImagePayload(direct.Item); dataURL != "" || imageURL != "" {
					if imageURL != "" {
						addUniqueWebAssetURLs(&directURLs, imageURL)
					} else {
						mime := mimeForImageFormat(direct.Item.OutputFormat)
						if mime == "" {
							mime = "image/png"
						}
						addUniqueString(&directURLs, "data:"+mime+";base64,"+dataURL)
					}
				}
			}
		case "response.completed":
			for _, out := range ev.Response.Output {
				if dataURL, imageURL := outputImagePayload(out); dataURL != "" || imageURL != "" {
					if imageURL != "" {
						addUniqueWebAssetURLs(&directURLs, imageURL)
						continue
					}
					mime := mimeForImageFormat(out.OutputFormat)
					if mime == "" {
						mime = "image/png"
					}
					addUniqueString(&directURLs, "data:"+mime+";base64,"+dataURL)
				}
			}
		case "response.image_generation_call.partial_image":
			var partial struct {
				OutputFormat string `json:"output_format"`
				PartialB64   string `json:"partial_image_b64"`
			}
			if err := json.Unmarshal([]byte(data), &partial); err == nil && partial.PartialB64 != "" {
				mime := mimeForImageFormat(partial.OutputFormat)
				if mime == "" {
					mime = "image/png"
				}
				addUniqueString(&directURLs, "data:"+mime+";base64,"+partial.PartialB64)
			}
		}
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	flush()
	if err := scanner.Err(); err != nil {
		return "", nil, nil, nil, "", fmt.Errorf("gpt image2 web stream read: %w", err)
	}
	return conversationID, fileIDs, sedimentIDs, directURLs, lastText, nil
}

var (
	webConversationIDRe = regexp.MustCompile(`"conversation_id"\s*:\s*"([^"]+)"`)
	webFileIDRe         = regexp.MustCompile(`file[-_][A-Za-z0-9][A-Za-z0-9_-]{7,}`)
	webSedimentIDRe     = regexp.MustCompile(`sediment://([A-Za-z0-9_-]+)`)
	webAssetURLRe       = regexp.MustCompile(`https:\\?/\\?/(?:files\.oaiusercontent\.com|oaidalleapiprodscus\.blob\.core\.windows\.net)[^"\\]+`)
)

func extractWebImageIDs(payload string) (string, []string, []string, []string) {
	conversationID := ""
	if m := webConversationIDRe.FindStringSubmatch(payload); len(m) > 1 {
		conversationID = m[1]
	}
	var fileIDs, sedimentIDs, directURLs []string
	for _, id := range webFileIDRe.FindAllString(payload, -1) {
		addUniqueString(&fileIDs, id)
	}
	for _, m := range webSedimentIDRe.FindAllStringSubmatch(payload, -1) {
		if len(m) > 1 {
			addUniqueString(&sedimentIDs, m[1])
		}
	}
	for _, raw := range webAssetURLRe.FindAllString(payload, -1) {
		u := strings.ReplaceAll(raw, `\/`, `/`)
		u = strings.ReplaceAll(u, `\u0026`, `&`)
		if strings.Contains(u, "openaiassets.blob.core.windows.net/$web/chatgpt/") {
			continue
		}
		addUniqueWebAssetURLs(&directURLs, u)
	}
	return conversationID, fileIDs, sedimentIDs, directURLs
}

func extractWebImageDirectURLs(payload string) []string {
	_, _, _, directURLs := extractWebImageIDs(payload)
	return directURLs
}

func extractWebImageToolIDs(raw []byte) ([]string, []string) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, nil
	}
	var fileIDs, sedimentIDs []string
	walkWebImageToolMessages(v, &fileIDs, &sedimentIDs)
	return fileIDs, sedimentIDs
}

func walkWebImageToolMessages(v any, fileIDs, sedimentIDs *[]string) {
	switch t := v.(type) {
	case map[string]any:
		if msg, ok := asWebMessageMap(t); ok && isWebImageAssetMessage(msg) {
			extractWebAssetPointersFromMessage(msg, fileIDs, sedimentIDs)
		}
		for _, val := range t {
			walkWebImageToolMessages(val, fileIDs, sedimentIDs)
		}
	case []any:
		for _, val := range t {
			walkWebImageToolMessages(val, fileIDs, sedimentIDs)
		}
	}
}

func asWebMessageMap(m map[string]any) (map[string]any, bool) {
	if msg, ok := m["message"].(map[string]any); ok {
		return msg, true
	}
	if _, ok := m["author"].(map[string]any); ok {
		return m, true
	}
	return nil, false
}

func isWebImageAssetMessage(msg map[string]any) bool {
	author, _ := msg["author"].(map[string]any)
	metadata, _ := msg["metadata"].(map[string]any)
	content, _ := msg["content"].(map[string]any)
	role := strings.ToLower(strings.TrimSpace(fmt.Sprint(author["role"])))
	taskType := strings.ToLower(strings.TrimSpace(fmt.Sprint(metadata["async_task_type"])))
	contentType := strings.ToLower(strings.TrimSpace(fmt.Sprint(content["content_type"])))
	if role != "tool" && role != "assistant" {
		return false
	}
	if taskType == "" {
		taskType = strings.ToLower(strings.TrimSpace(fmt.Sprint(metadata["task_type"])))
	}
	if taskType != "" && !strings.Contains(taskType, "image") && !strings.Contains(taskType, "picture") {
		return false
	}
	return strings.Contains(contentType, "text") || strings.Contains(contentType, "image")
}

func extractWebAssetPointersFromMessage(msg map[string]any, fileIDs, sedimentIDs *[]string) {
	content, _ := msg["content"].(map[string]any)
	walkWebAssetPointers(content, fileIDs, sedimentIDs)
}

func walkWebAssetPointers(v any, fileIDs, sedimentIDs *[]string) {
	switch t := v.(type) {
	case map[string]any:
		if ptr := strings.TrimSpace(fmt.Sprint(t["asset_pointer"])); ptr != "" {
			addWebAssetPointer(ptr, fileIDs, sedimentIDs)
		}
		for _, val := range t {
			walkWebAssetPointers(val, fileIDs, sedimentIDs)
		}
	case []any:
		for _, val := range t {
			walkWebAssetPointers(val, fileIDs, sedimentIDs)
		}
	case string:
		addWebAssetPointer(t, fileIDs, sedimentIDs)
	}
}

func addWebAssetPointer(ptr string, fileIDs, sedimentIDs *[]string) {
	switch {
	case strings.HasPrefix(ptr, "file-service://"):
		id := strings.TrimPrefix(ptr, "file-service://")
		if id != "" && id != "file_upload" {
			addUniqueString(fileIDs, id)
		}
	case strings.HasPrefix(ptr, "sediment://"):
		id := strings.TrimPrefix(ptr, "sediment://")
		if id != "" {
			addUniqueString(sedimentIDs, id)
		}
	}
}

func extractWebAssistantText(payload string) string {
	var ev any
	if err := json.Unmarshal([]byte(payload), &ev); err != nil {
		return ""
	}
	return findFirstStringByKey(ev, "parts")
}

func findFirstStringByKey(v any, key string) string {
	switch t := v.(type) {
	case map[string]any:
		if val, ok := t[key]; ok {
			if arr, ok := val.([]any); ok {
				for _, item := range arr {
					if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
						return strings.TrimSpace(s)
					}
				}
			}
		}
		for _, val := range t {
			if s := findFirstStringByKey(val, key); s != "" {
				return s
			}
		}
	case []any:
		for _, val := range t {
			if s := findFirstStringByKey(val, key); s != "" {
				return s
			}
		}
	}
	return ""
}

func addUniqueString(dst *[]string, vals ...string) {
	for _, v := range vals {
		if v == "" {
			continue
		}
		exists := false
		for _, cur := range *dst {
			if cur == v {
				exists = true
				break
			}
		}
		if !exists {
			*dst = append(*dst, v)
		}
	}
}

func addUniqueWebAssetURLs(dst *[]string, vals ...string) {
	for _, v := range vals {
		if isGeneratedWebAssetURL(v) {
			addUniqueString(dst, v)
		}
	}
}

func isGeneratedWebAssetURL(rawURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Host)
	path := strings.ToLower(u.EscapedPath())
	if strings.Contains(host, "openaiassets.blob.core.windows.net") {
		return false
	}
	if strings.Contains(path, "/$web/chatgpt/") ||
		strings.Contains(path, "filled-plus-icon") ||
		strings.Contains(path, "icon") ||
		strings.Contains(path, "logo") {
		return false
	}
	return strings.Contains(host, "files.oaiusercontent.com") ||
		strings.Contains(host, "oaidalleapiprodscus.blob.core.windows.net") ||
		(strings.HasSuffix(host, ".blob.core.windows.net") && !strings.Contains(path, "/$web/"))
}

func firstString(vals []string) string {
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

func sanitizeDiagURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return snippet([]byte(rawURL), 180)
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func shouldUseWebDownloadHeaders(base, rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if u.Scheme == "" && strings.HasPrefix(u.Path, "/backend-api/") {
		return true
	}
	if !strings.Contains(u.Path, "/backend-api/") {
		return false
	}
	b, err := url.Parse(base)
	if err != nil || b.Host == "" {
		return strings.Contains(u.Host, "chatgpt.com")
	}
	return strings.EqualFold(u.Host, b.Host)
}

func logUpstream(ctx context.Context, req *provider.Request, entry provider.UpstreamLogEntry) {
	if req == nil || req.UpstreamLog == nil {
		return
	}
	if entry.Provider == "" {
		entry.Provider = "gpt"
	}
	req.UpstreamLog(ctx, entry)
}

func buildLegacyRequirementsToken(userAgent string) string {
	seed := fmt.Sprintf("%0.16f", rand.Float64())
	config := []any{
		3000 + rand.Intn(3)*1000,
		time.Now().In(time.FixedZone("EST", -5*3600)).Format("Mon Jan 02 2006 15:04:05") + " GMT-0500 (Eastern Standard Time)",
		4294705152,
		0,
		userAgent,
		"https://chatgpt.com/backend-api/sentinel/sdk.js",
		"",
		"en-US",
		"en-US,es-US,en,es",
		0,
		"webdriver≭false",
		"location",
		"window",
		float64(time.Now().UnixNano()) / 1e6,
		uuid.NewString(),
		"",
		16,
		float64(time.Now().UnixNano()) / 1e6,
	}
	answer, _ := powGenerate(seed, "0fffff", config)
	return "gAAAAAC" + answer
}

func buildProofToken(seed, difficulty, userAgent string) string {
	config := []any{
		3000 + rand.Intn(3)*1000,
		time.Now().In(time.FixedZone("EST", -5*3600)).Format("Mon Jan 02 2006 15:04:05") + " GMT-0500 (Eastern Standard Time)",
		4294705152,
		0,
		userAgent,
		"https://chatgpt.com/backend-api/sentinel/sdk.js",
		"",
		"en-US",
		"en-US,es-US,en,es",
		0,
		"webdriver≭false",
		"location",
		"window",
		float64(time.Now().UnixNano()) / 1e6,
		uuid.NewString(),
		"",
		16,
		float64(time.Now().UnixNano()) / 1e6,
	}
	answer, solved := powGenerate(seed, difficulty, config)
	if !solved {
		return "gAAAAAB" + base64.StdEncoding.EncodeToString([]byte(`"`+seed+`"`))
	}
	return "gAAAAAB" + answer
}

func powGenerate(seed, difficulty string, config []any) (string, bool) {
	target := difficulty
	diffBytes, err := hexToBytes(target)
	if err != nil || len(diffBytes) == 0 {
		return base64.StdEncoding.EncodeToString([]byte(`"` + seed + `"`)), false
	}
	static1 := mustJSON(config[:3])
	static1 = strings.TrimSuffix(static1, "]") + ","
	static2 := "," + strings.TrimPrefix(strings.TrimSuffix(mustJSON(config[4:9]), "]"), "[") + ","
	static3 := "," + strings.TrimPrefix(mustJSON(config[10:]), "[")
	seedBytes := []byte(seed)
	for i := 0; i < 500000; i++ {
		final := static1 + fmt.Sprint(i) + static2 + fmt.Sprint(i>>1) + static3
		encoded := base64.StdEncoding.EncodeToString([]byte(final))
		h := sha3.Sum512(append(seedBytes, []byte(encoded)...))
		if bytes.Compare(h[:len(diffBytes)], diffBytes) <= 0 {
			return encoded, true
		}
	}
	return base64.StdEncoding.EncodeToString([]byte(`"` + seed + `"`)), false
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func hexToBytes(s string) ([]byte, error) {
	if len(s)%2 == 1 {
		s = "0" + s
	}
	out := make([]byte, len(s)/2)
	for i := 0; i < len(out); i++ {
		var x byte
		for j := 0; j < 2; j++ {
			c := s[i*2+j]
			x <<= 4
			switch {
			case c >= '0' && c <= '9':
				x |= c - '0'
			case c >= 'a' && c <= 'f':
				x |= c - 'a' + 10
			case c >= 'A' && c <= 'F':
				x |= c - 'A' + 10
			default:
				return nil, fmt.Errorf("invalid hex")
			}
		}
		out[i] = x
	}
	return out, nil
}

func outputImagePayload(out responseOutputItem) (string, string) {
	if out.Result != "" {
		return out.Result, ""
	}
	if out.B64JSON != "" {
		return out.B64JSON, ""
	}
	if out.ImageB64 != "" {
		return out.ImageB64, ""
	}
	if out.URL != "" {
		return "", out.URL
	}
	for _, content := range out.Content {
		if content.Result != "" {
			return content.Result, ""
		}
		if content.B64JSON != "" {
			return content.B64JSON, ""
		}
		if content.ImageB64 != "" {
			return content.ImageB64, ""
		}
		if content.URL != "" {
			return "", content.URL
		}
	}
	return "", ""
}

func parseCompletedResponse(r io.Reader) (*responseCompletedEvent, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	var dataLines []string
	var last *responseCompletedEvent
	var outputItems []responseOutputItem
	var partialItems []responseOutputItem
	flush := func() error {
		if len(dataLines) == 0 {
			return nil
		}
		data := strings.TrimSpace(strings.Join(dataLines, "\n"))
		dataLines = nil
		if data == "" || data == "[DONE]" {
			return nil
		}
		var ev responseCompletedEvent
		err := json.Unmarshal([]byte(data), &ev)
		var direct struct {
			Output []responseOutputItem `json:"output"`
			Item   responseOutputItem   `json:"item"`
		}
		if err2 := json.Unmarshal([]byte(data), &direct); err2 == nil {
			if len(ev.Response.Output) == 0 && len(direct.Output) > 0 {
				ev.Type = "response.completed"
				ev.Response.Output = direct.Output
			}
			if direct.Item.Type != "" && ev.Type == "" {
				ev.Type = "response.output_item.done"
			}
		}
		if err != nil && len(ev.Response.Output) == 0 && direct.Item.Type == "" {
			return err
		}
		switch ev.Type {
		case "response.output_item.done":
			if direct.Item.Type != "" {
				outputItems = append(outputItems, direct.Item)
			}
		case "response.image_generation_call.partial_image":
			var partial struct {
				OutputFormat string `json:"output_format"`
				PartialB64   string `json:"partial_image_b64"`
			}
			if err := json.Unmarshal([]byte(data), &partial); err == nil && partial.PartialB64 != "" {
				partialItems = append(partialItems, responseOutputItem{
					Type:         "image_generation_call",
					Result:       partial.PartialB64,
					OutputFormat: partial.OutputFormat,
				})
			}
		}
		if ev.Type == "response.completed" || len(ev.Response.Output) > 0 || ev.Error != nil {
			last = &ev
		}
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := flush(); err != nil {
				return nil, fmt.Errorf("gpt image2 stream decode: %w", err)
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("gpt image2 stream read: %w", err)
	}
	if err := flush(); err != nil {
		return nil, fmt.Errorf("gpt image2 stream decode: %w", err)
	}
	if last == nil {
		last = &responseCompletedEvent{Type: "response.completed"}
	}
	if len(last.Response.Output) == 0 && len(outputItems) > 0 {
		last.Response.Output = outputItems
	}
	if len(last.Response.Output) == 0 && len(partialItems) > 0 {
		last.Response.Output = partialItems
	}
	return last, nil
}

func mimeForImageFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "jpeg", "jpg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	default:
		return "image/png"
	}
}

func parseSize(size string) (int, int) {
	if size == "" {
		return 1024, 1024
	}
	parts := strings.SplitN(size, "x", 2)
	if len(parts) != 2 {
		return 1024, 1024
	}
	var w, h int
	fmt.Sscanf(parts[0], "%d", &w)
	fmt.Sscanf(parts[1], "%d", &h)
	if w <= 0 {
		w = 1024
	}
	if h <= 0 {
		h = 1024
	}
	return w, h
}

func snippet(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	r := []rune(string(b))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "...(truncated)"
}
