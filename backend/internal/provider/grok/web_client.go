package grok

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/kleinai/backend/internal/provider"
	"github.com/kleinai/backend/pkg/outbound"
)

var grokCFCache = struct {
	sync.Mutex
	state  grokRuntimeCFState
	loaded time.Time
}{}

type grokRuntimeCFState struct {
	Cookies     string `json:"cookies"`
	CFClearance string `json:"cf_clearance"`
	UserAgent   string `json:"user_agent"`
	Browser     string `json:"browser"`
	UpdatedAt   int64  `json:"updated_at"`
}

const (
	webBaseURL             = "https://grok.com"
	chatEndpoint           = "/rest/app-chat/conversations/new"
	uploadEndpoint         = "/rest/app-chat/upload-file"
	mediaEndpoint          = "/rest/media/post/create"
	mediaGetEndpoint       = "/rest/media/post/get"
	videoModelName         = "imagine-video-gen"
	grokUA                 = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36"
	imageMediaType         = "MEDIA_POST_TYPE_IMAGE"
	videoMediaType         = "MEDIA_POST_TYPE_VIDEO"
	defaultVideoSize       = "1280x720"
	defaultVideoMode       = "custom"
	defaultVideoResolution = "480p"
)

type chatModelParams struct {
	upstream string
	mode     string
}

var chatModels = map[string]chatModelParams{
	"grok-3":          {upstream: "grok-3", mode: "MODEL_MODE_GROK_3"},
	"grok-3-mini":     {upstream: "grok-3", mode: "MODEL_MODE_GROK_3_MINI_THINKING"},
	"grok-3-thinking": {upstream: "grok-3", mode: "MODEL_MODE_GROK_3_THINKING"},
	"grok-4":          {upstream: "grok-4", mode: "MODEL_MODE_GROK_4"},
	"grok-4-thinking": {upstream: "grok-4", mode: "MODEL_MODE_GROK_4_THINKING"},
	"grok-4-heavy":    {upstream: "grok-4", mode: "MODEL_MODE_HEAVY"},
	"grok-4.1-mini":   {upstream: "grok-4-1-thinking-1129", mode: "MODEL_MODE_GROK_4_1_MINI_THINKING"},
	"grok-4.1-fast":   {upstream: "grok-4-1-thinking-1129", mode: "MODEL_MODE_FAST"},
	"grok-4.1-expert": {upstream: "grok-4-1-thinking-1129", mode: "MODEL_MODE_EXPERT"},
	"grok-4.1-thinking": {
		upstream: "grok-4-1-thinking-1129",
		mode:     "MODEL_MODE_GROK_4_1_THINKING",
	},
	"grok-4.20-beta": {upstream: "grok-420", mode: "MODEL_MODE_GROK_420"},

	// Backward-compatible aliases used by the current frontend.
	"grok-4.20-fast":   {upstream: "grok-4-1-thinking-1129", mode: "MODEL_MODE_FAST"},
	"grok-4.20-auto":   {upstream: "grok-4", mode: "MODEL_MODE_GROK_4"},
	"grok-4.20-expert": {upstream: "grok-4-1-thinking-1129", mode: "MODEL_MODE_EXPERT"},
	"grok-4.20-heavy":  {upstream: "grok-4", mode: "MODEL_MODE_HEAVY"},
	"grok-4.3-beta":    {upstream: "grok-420", mode: "MODEL_MODE_GROK_420"},
}

// ChatModelIDs returns downstream chat models backed by Grok Web.
func ChatModelIDs() []string {
	return []string{
		"grok-4.20-fast",
		"grok-4.20-auto",
		"grok-4.20-expert",
		"grok-4.20-heavy",
		"grok-4.3-beta",
		"grok-3",
	}
}

func IsChatModel(modelCode string) bool {
	_, ok := chatModels[strings.ToLower(strings.TrimSpace(modelCode))]
	return ok
}

func ModeForChatModel(modelCode string) string {
	if params, ok := chatModels[strings.ToLower(strings.TrimSpace(modelCode))]; ok {
		return params.mode
	}
	return "MODEL_MODE_FAST"
}

func UpstreamForChatModel(modelCode string) string {
	if params, ok := chatModels[strings.ToLower(strings.TrimSpace(modelCode))]; ok {
		return params.upstream
	}
	return strings.TrimSpace(modelCode)
}

func NormalizeVideoModel(modelCode string) string {
	switch strings.ToLower(strings.TrimSpace(modelCode)) {
	case "", "vid-v1", "vid-i2v", "grok-video", "grok-i2v", "grok-imagine-video":
		return "grok-imagine-video"
	default:
		return modelCode
	}
}

type WebClient struct {
	baseURL        string
	proxyURL       string
	upstreamLogger provider.UpstreamLogger
}

func NewWebClient(base string) *WebClient {
	return NewWebClientWithProxy(base, "")
}

func NewWebClientWithProxy(base, proxyURL string) *WebClient {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" || strings.Contains(base, "api.x.ai") {
		base = webBaseURL
	}
	return &WebClient{baseURL: base, proxyURL: strings.TrimSpace(proxyURL)}
}

func (c *WebClient) WithUpstreamLogger(logger provider.UpstreamLogger) *WebClient {
	if c == nil {
		return nil
	}
	clone := *c
	clone.upstreamLogger = logger
	return &clone
}

type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ChatResult struct {
	Raw    []byte
	Status int
	Usage  *OpenAIUsage
}

func (c *WebClient) logUpstream(ctx context.Context, entry provider.UpstreamLogEntry) {
	if c == nil || c.upstreamLogger == nil {
		return
	}
	if entry.Provider == "" {
		entry.Provider = "grok"
	}
	c.upstreamLogger(ctx, entry)
}

func (c *WebClient) ChatComplete(ctx context.Context, token, modelCode string, body map[string]any) (*ChatResult, error) {
	prompt, files := buildGrokPromptAndFiles(body)
	reqBody := c.chatPayload(prompt, modelCode)
	if len(files) > 0 {
		attachments, err := c.uploadChatFiles(ctx, token, files)
		if err != nil {
			return nil, err
		}
		reqBody["fileAttachments"] = attachments
	}
	resp, err := c.doJSONStream(ctx, token, chatEndpoint, reqBody, 10*time.Minute)
	if err != nil {
		return nil, err
	}
	resp.Body = decodeResponseBody(resp)
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return &ChatResult{Raw: raw, Status: resp.StatusCode}, nil
	}
	rawText, _, _, _, err := collectGrokStream(resp.Body, nil)
	if err != nil {
		return nil, err
	}
	rawText = cleanGrokText(rawText)
	usage := estimateOpenAIUsage(prompt, rawText)
	raw, _ := json.Marshal(map[string]any{
		"id":      "chatcmpl_" + shortID(),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   modelCode,
		"choices": []map[string]any{{
			"index":         0,
			"message":       map[string]any{"role": "assistant", "content": rawText},
			"finish_reason": "stop",
		}},
		"usage": usage,
	})
	return &ChatResult{Raw: raw, Status: http.StatusOK, Usage: usage}, nil
}

func (c *WebClient) ChatStream(ctx context.Context, token, modelCode string, body map[string]any, w io.Writer, flusher http.Flusher) (*OpenAIUsage, error) {
	prompt, files := buildGrokPromptAndFiles(body)
	reqBody := c.chatPayload(prompt, modelCode)
	if len(files) > 0 {
		attachments, err := c.uploadChatFiles(ctx, token, files)
		if err != nil {
			return nil, err
		}
		reqBody["fileAttachments"] = attachments
	}
	resp, err := c.doJSONStream(ctx, token, chatEndpoint, reqBody, 10*time.Minute)
	if err != nil {
		return nil, err
	}
	resp.Body = decodeResponseBody(resp)
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return nil, fmt.Errorf("grok chat HTTP %d: %s", resp.StatusCode, snippet(raw, 240))
	}
	var full strings.Builder
	_, _, _, _, err = collectGrokStream(resp.Body, func(delta string) {
		if delta == "" {
			return
		}
		if looksLikeXAIToolMarkup(delta) {
			return
		}
		full.WriteString(delta)
		payload, _ := json.Marshal(map[string]any{
			"id":      "chatcmpl_" + shortID(),
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   modelCode,
			"choices": []map[string]any{{"index": 0, "delta": map[string]any{"content": delta}}},
		})
		_, _ = io.WriteString(w, "data: "+string(payload)+"\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	})
	if err != nil {
		return nil, err
	}
	usage := estimateOpenAIUsage(prompt, full.String())
	payload, _ := json.Marshal(map[string]any{
		"id":      "chatcmpl_" + shortID(),
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   modelCode,
		"choices": []map[string]any{},
		"usage":   usage,
	})
	_, _ = io.WriteString(w, "data: "+string(payload)+"\n\n")
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
	return usage, nil
}

type VideoRequest struct {
	ModelCode   string
	Prompt      string
	Refs        []string
	DurationSec int
	Size        string
	AspectRatio string
	Quality     string
	Count       int
}

type VideoAsset struct {
	URL        string
	ThumbURL   string
	Width      int
	Height     int
	DurationMs int
}

type uploadedVideoRef struct {
	fileID   string
	assetURL string
}

func (c *WebClient) GenerateVideo(ctx context.Context, token string, req VideoRequest) ([]VideoAsset, error) {
	if req.DurationSec <= 0 {
		req.DurationSec = 6
	}
	req.DurationSec = normalizeVideoDuration(req.DurationSec)
	if req.Size == "" && strings.TrimSpace(req.AspectRatio) == "" {
		req.Size = defaultVideoSize
	}
	aspect, resolution, width, height := videoConfig(req.Size, req.AspectRatio)
	c.logUpstream(ctx, provider.UpstreamLogEntry{
		Provider: "grok",
		Stage:    "video.start",
		Meta: map[string]any{
			"model":          req.ModelCode,
			"duration_sec":   req.DurationSec,
			"aspect_ratio":   aspect,
			"resolution":     resolution,
			"size":           req.Size,
			"refs_count":     len(req.Refs),
			"has_proxy":      c.proxyURL != "",
			"has_ref_prompt": strings.TrimSpace(req.Prompt) != "",
		},
	})

	parentPostID := ""
	refs := make([]uploadedVideoRef, 0, len(req.Refs))
	for _, ref := range req.Refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		uploaded, err := c.prepareVideoRef(ctx, token, ref)
		if err != nil {
			c.logUpstream(ctx, provider.UpstreamLogEntry{
				Provider: "grok",
				Stage:    "video.ref",
				Error:    err.Error(),
				Meta:     map[string]any{"ref_index": len(refs) + 1, "ref": sanitizeDiagURL(ref)},
			})
			return nil, err
		}
		if uploaded.assetURL != "" {
			refs = append(refs, uploaded)
		}
	}
	if len(refs) == 1 {
		imagePostID, err := c.createMediaPost(ctx, token, imageMediaType, "", refs[0].assetURL)
		if err != nil {
			c.logUpstream(ctx, provider.UpstreamLogEntry{Provider: "grok", Stage: "video.parent_post", Error: err.Error(), Meta: map[string]any{"media_type": imageMediaType, "refs_count": len(refs), "has_media_url": refs[0].assetURL != ""}})
			return nil, err
		}
		parentPostID = imagePostID
	}
	if len(refs) != 1 {
		var err error
		parentPostID, err = c.createMediaPost(ctx, token, videoMediaType, req.Prompt, "")
		if err != nil {
			c.logUpstream(ctx, provider.UpstreamLogEntry{Provider: "grok", Stage: "video.parent_post", Error: err.Error(), Meta: map[string]any{"media_type": videoMediaType, "refs_count": len(refs)}})
			return nil, err
		}
	}

	message := strings.TrimSpace(req.Prompt)
	if message == "" {
		message = "Generate a video"
	}
	fileAttachments := []any{}
	if len(refs) == 1 {
		message = strings.TrimSpace(refs[0].assetURL + "  " + message)
	} else if len(refs) > 1 {
		mentions := make([]string, 0, len(refs))
		for _, ref := range refs {
			if ref.fileID != "" {
				mentions = append(mentions, "@"+ref.fileID)
				fileAttachments = append(fileAttachments, ref.fileID)
			}
		}
		if len(mentions) > 0 {
			message = strings.TrimSpace(strings.Join(mentions, " ") + " " + message)
		}
	}
	message = strings.TrimSpace(message + " --mode=" + defaultVideoMode)
	payload := c.chatPayload(message, "grok-3")
	payload["fileAttachments"] = fileAttachments
	payload["modelMode"] = defaultVideoMode
	payload["enableImageGeneration"] = true
	payload["returnImageBytes"] = false
	payload["toolOverrides"] = map[string]any{"videoGen": true}
	payload["responseMetadata"] = map[string]any{
		"modelConfigOverride": map[string]any{
			"modelMap": map[string]any{
				"videoGenModelConfig": map[string]any{
					"parentPostId":   parentPostID,
					"aspectRatio":    aspect,
					"videoLength":    req.DurationSec,
					"resolutionName": resolution,
					"isVideoEdit":    false,
					"mode":           defaultVideoMode,
					"originalPrompt": strings.TrimSpace(req.Prompt),
				},
			},
		},
	}
	cfg := payload["responseMetadata"].(map[string]any)["modelConfigOverride"].(map[string]any)["modelMap"].(map[string]any)["videoGenModelConfig"].(map[string]any)
	if len(refs) > 1 {
		imageReferences := make([]string, 0, len(refs))
		for _, ref := range refs {
			if ref.assetURL != "" {
				imageReferences = append(imageReferences, ref.assetURL)
			}
		}
		cfg["isReferenceToVideo"] = true
		cfg["imageReferences"] = imageReferences
	}

	resp, err := c.doJSONStream(ctx, token, chatEndpoint, payload, 15*time.Minute)
	if err != nil {
		c.logUpstream(ctx, provider.UpstreamLogEntry{
			Provider:       "grok",
			Stage:          "video.conversation",
			Method:         "POST",
			URL:            c.baseURL + chatEndpoint,
			RequestExcerpt: jsonSnippet(payload, 600),
			Error:          err.Error(),
			Meta:           map[string]any{"refs_count": len(refs), "duration_sec": req.DurationSec, "resolution": resolution, "aspect_ratio": aspect},
		})
		return nil, err
	}
	resp.Body = decodeResponseBody(resp)
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		c.logUpstream(ctx, provider.UpstreamLogEntry{
			Provider:        "grok",
			Stage:           "video.conversation",
			Method:          "POST",
			URL:             c.baseURL + chatEndpoint,
			StatusCode:      resp.StatusCode,
			RequestExcerpt:  jsonSnippet(payload, 600),
			ResponseExcerpt: snippet(raw, 600),
			Meta:            map[string]any{"refs_count": len(refs), "duration_sec": req.DurationSec, "resolution": resolution, "aspect_ratio": aspect},
		})
		return nil, fmt.Errorf("grok video HTTP %d: %s", resp.StatusCode, snippet(raw, 240))
	}
	var assets []VideoAsset
	_, videoURL, thumbURL, videoPostID, err := collectGrokStream(resp.Body, func(_ string) {})
	if err != nil {
		c.logUpstream(ctx, provider.UpstreamLogEntry{
			Provider: "grok",
			Stage:    "video.stream",
			Error:    err.Error(),
			Meta:     map[string]any{"refs_count": len(refs), "duration_sec": req.DurationSec, "resolution": resolution, "aspect_ratio": aspect},
		})
		return nil, err
	}
	if videoURL == "" && videoPostID != "" {
		videoURL, thumbURL, err = c.fetchVideoAssetFromPost(ctx, token, videoPostID, thumbURL)
		if err != nil {
			c.logUpstream(ctx, provider.UpstreamLogEntry{
				Provider: "grok",
				Stage:    "video.post_fetch",
				Error:    err.Error(),
				Meta:     map[string]any{"post_id": videoPostID, "fallback_thumb": sanitizeDiagURL(thumbURL)},
			})
			return nil, err
		}
	}
	if videoURL != "" {
		if thumbURL == "" {
			thumbURL = derivePreviewImageURL(videoURL)
		}
		assets = append(assets, VideoAsset{URL: videoURL, ThumbURL: thumbURL, Width: width, Height: height, DurationMs: req.DurationSec * 1000})
	}
	if len(assets) == 0 {
		c.logUpstream(ctx, provider.UpstreamLogEntry{
			Provider: "grok",
			Stage:    "video.failed",
			Error:    "grok video finished without video url",
			Meta:     map[string]any{"refs_count": len(refs), "duration_sec": req.DurationSec, "resolution": resolution, "aspect_ratio": aspect, "parent_post_id": parentPostID},
		})
		return nil, fmt.Errorf("grok video finished without video url")
	}
	c.logUpstream(ctx, provider.UpstreamLogEntry{
		Provider: "grok",
		Stage:    "video.success",
		Meta:     map[string]any{"assets": len(assets), "duration_sec": req.DurationSec, "resolution": resolution, "aspect_ratio": aspect, "parent_post_id": parentPostID},
	})
	return assets, nil
}

func (c *WebClient) fetchVideoAssetFromPost(ctx context.Context, token, postID, fallbackThumb string) (string, string, error) {
	postID = strings.TrimSpace(postID)
	if postID == "" {
		return "", "", nil
	}
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", "", ctx.Err()
			case <-time.After(time.Duration(attempt*5) * time.Second):
			}
		}
		resp, err := c.doJSON(ctx, token, mediaGetEndpoint, map[string]any{"id": postID}, 45*time.Second)
		if err != nil {
			lastErr = err
			continue
		}
		resp.Body = decodeResponseBody(resp)
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		_ = resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			lastErr = fmt.Errorf("grok media post get HTTP %d: %s", resp.StatusCode, snippet(raw, 240))
			continue
		}
		var obj any
		if err := json.Unmarshal(raw, &obj); err != nil {
			lastErr = err
			continue
		}
		videoURL := firstVideoURL(obj)
		if videoURL != "" {
			thumbURL := firstStringByKeys(obj, []string{"thumbnailImageUrl", "thumbnailUrl", "coverUrl"})
			if thumbURL == "" {
				thumbURL = fallbackThumb
			}
			return normalizeAssetURL(videoURL), normalizeAssetURL(thumbURL), nil
		}
		lastErr = fmt.Errorf("grok media post get missing video url")
	}
	return "", fallbackThumb, lastErr
}

func (c *WebClient) prepareVideoRef(ctx context.Context, token, ref string) (uploadedVideoRef, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return uploadedVideoRef{}, nil
	}
	if strings.HasPrefix(ref, "data:") {
		fileID, assetURL, err := c.uploadDataURLMeta(ctx, token, ref)
		if err != nil {
			return uploadedVideoRef{}, err
		}
		return uploadedVideoRef{fileID: fileID, assetURL: assetURL}, nil
	}
	if strings.HasPrefix(ref, "/api/v1/gen/cached/") {
		fileID, assetURL, err := c.uploadCachedLocalImageMeta(ctx, token, ref)
		if err != nil {
			return uploadedVideoRef{}, err
		}
		return uploadedVideoRef{fileID: fileID, assetURL: assetURL}, nil
	}
	if !isGrokAssetURL(ref) {
		fileID, assetURL, err := c.uploadRemoteImageMeta(ctx, token, ref)
		if err != nil {
			return uploadedVideoRef{}, err
		}
		return uploadedVideoRef{fileID: fileID, assetURL: assetURL}, nil
	}
	return uploadedVideoRef{assetURL: normalizeAssetURL(ref)}, nil
}

func (c *WebClient) chatPayload(message, modelCode string) map[string]any {
	upstreamModel := UpstreamForChatModel(modelCode)
	return map[string]any{
		"deviceEnvInfo":               map[string]any{"darkModeEnabled": false, "devicePixelRatio": 2, "screenHeight": 1329, "screenWidth": 2056, "viewportHeight": 1083, "viewportWidth": 2056},
		"disableMemory":               true,
		"disableSearch":               true,
		"disableSelfHarmShortCircuit": false,
		"disableTextFollowUps":        false,
		"enableImageGeneration":       true,
		"enableImageStreaming":        true,
		"enableSideBySide":            true,
		"fileAttachments":             []any{},
		"forceConcise":                false,
		"forceSideBySide":             false,
		"imageAttachments":            []any{},
		"imageGenerationCount":        2,
		"isAsyncChat":                 false,
		"isReasoning":                 false,
		"message":                     message,
		"modelMode":                   ModeForChatModel(modelCode),
		"modelName":                   upstreamModel,
		"responseMetadata":            map[string]any{"requestModelDetails": map[string]any{"modelId": upstreamModel}},
		"returnImageBytes":            false,
		"returnRawGrokInXaiRequest":   false,
		"sendFinalMetadata":           true,
		"temporary":                   true,
		"toolOverrides":               map[string]any{"webSearch": false, "xSearch": false, "x_keyword_search": false},
		"enable420":                   upstreamModel == "grok-420",
	}
}

func (c *WebClient) doJSONStream(ctx context.Context, token, endpoint string, body map[string]any, timeout time.Duration) (*http.Response, error) {
	payload, _ := json.Marshal(body)
	client, err := outbound.NewClient(outbound.Options{Timeout: timeout, ProxyURL: c.proxyURL, Mode: outbound.ModeUTLS, Profile: outbound.ProfileChrome})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	setGrokHeaders(req, token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream, application/json, */*")
	return client.Do(req)
}

func (c *WebClient) createMediaPost(ctx context.Context, token, mediaType, prompt, mediaURL string) (string, error) {
	body := map[string]any{"mediaType": mediaType, "prompt": prompt}
	if mediaURL != "" {
		body["mediaUrl"] = mediaURL
	}
	resp, err := c.doJSON(ctx, token, mediaEndpoint, body, 30*time.Second)
	if err != nil {
		return "", err
	}
	resp.Body = decodeResponseBody(resp)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("grok media post HTTP %d: %s", resp.StatusCode, snippet(raw, 240))
	}
	var obj map[string]any
	_ = json.Unmarshal(raw, &obj)
	for _, key := range []string{"postId", "id", "mediaPostId"} {
		if s, _ := obj[key].(string); s != "" {
			return s, nil
		}
	}
	if s := firstStringByKey(obj, "id"); s != "" {
		return s, nil
	}
	return "", fmt.Errorf("grok media post missing id: %s", snippet(raw, 240))
}

func (c *WebClient) doJSON(ctx context.Context, token, endpoint string, body map[string]any, timeout time.Duration) (*http.Response, error) {
	payload, _ := json.Marshal(body)
	client, err := outbound.NewClient(outbound.Options{Timeout: timeout, ProxyURL: c.proxyURL, Mode: outbound.ModeUTLS, Profile: outbound.ProfileChrome})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	setGrokHeaders(req, token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, */*")
	return client.Do(req)
}

func (c *WebClient) uploadDataURL(ctx context.Context, token, dataURL string) (string, error) {
	_, url, err := c.uploadDataURLMeta(ctx, token, dataURL)
	if err != nil {
		return "", err
	}
	return url, nil
}

func (c *WebClient) uploadDataURLMeta(ctx context.Context, token, dataURL string) (string, string, error) {
	comma := strings.Index(dataURL, ",")
	if comma < 0 {
		return "", "", fmt.Errorf("invalid data url")
	}
	meta, b64 := dataURL[:comma], dataURL[comma+1:]
	mimeType := "image/png"
	if strings.HasPrefix(meta, "data:") {
		if semi := strings.Index(meta, ";"); semi > len("data:") {
			mimeType = meta[len("data:"):semi]
		}
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", "", fmt.Errorf("decode data url: %w", err)
	}
	return c.uploadImageBytesMeta(ctx, token, mimeType, data)
}

func (c *WebClient) uploadCachedLocalImageMeta(ctx context.Context, token, cachedURL string) (string, string, error) {
	rel := strings.TrimPrefix(strings.TrimSpace(cachedURL), "/api/v1/gen/cached/")
	if rel == "" || strings.Contains(rel, "..") || strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, `\`) {
		return "", "", fmt.Errorf("invalid cached reference path")
	}
	root := strings.TrimSpace(os.Getenv("KLEIN_STORAGE_ROOT"))
	if root == "" {
		root = "/app/storage/public"
	}
	filePath := filepath.Join(root, filepath.FromSlash(rel))
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", "", fmt.Errorf("read cached reference image: %w", err)
	}
	if len(data) == 0 {
		return "", "", fmt.Errorf("empty cached reference image")
	}
	mimeType := mime.TypeByExtension(filepath.Ext(filePath))
	if mimeType == "" {
		mimeType = detectImageMime(data)
	}
	return c.uploadImageBytesMeta(ctx, token, mimeType, data)
}

func (c *WebClient) uploadRemoteImageMeta(ctx context.Context, token, imageURL string) (string, string, error) {
	imageURL = strings.TrimSpace(imageURL)
	if imageURL == "" {
		return "", "", fmt.Errorf("empty image url")
	}
	client, err := outbound.NewClient(outbound.Options{Timeout: 90 * time.Second, ProxyURL: c.proxyURL, Mode: outbound.ModeUTLS, Profile: outbound.ProfileChrome})
	if err != nil {
		return "", "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("download reference image: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", "", fmt.Errorf("download reference image HTTP %d: %s", resp.StatusCode, snippet(raw, 180))
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 25<<20))
	if err != nil {
		return "", "", fmt.Errorf("read reference image: %w", err)
	}
	if len(raw) == 0 {
		return "", "", fmt.Errorf("empty reference image")
	}
	mimeType := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	if mimeType == "" || !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		mimeType = detectImageMime(raw)
	}
	if !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		return "", "", fmt.Errorf("reference is not image: %s", mimeType)
	}
	return c.uploadImageBytesMeta(ctx, token, mimeType, raw)
}

func (c *WebClient) uploadImageBytesMeta(ctx context.Context, token, mimeType string, data []byte) (string, string, error) {
	mimeType = strings.TrimSpace(mimeType)
	if mimeType == "" {
		mimeType = detectImageMime(data)
	}
	if !strings.HasPrefix(strings.ToLower(mimeType), "image/") || strings.EqualFold(mimeType, "application/octet-stream") {
		if detected := detectImageMime(data); strings.HasPrefix(strings.ToLower(detected), "image/") {
			mimeType = detected
		}
	}
	if !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		return "", "", fmt.Errorf("unsupported image mime: %s", mimeType)
	}
	if shouldNormalizeGrokUpload() {
		if normalized, ok := normalizeImageForGrokUpload(data); ok {
			data = normalized
			mimeType = "image/jpeg"
		}
	}
	exts, _ := mime.ExtensionsByType(mimeType)
	ext := ".png"
	if len(exts) > 0 {
		ext = exts[0]
	}
	client, err := outbound.NewClient(outbound.Options{Timeout: 2 * time.Minute, ProxyURL: c.proxyURL, Mode: outbound.ModeUTLS, Profile: outbound.ProfileChrome})
	if err != nil {
		return "", "", err
	}
	payload, _ := json.Marshal(map[string]any{
		"fileName":     "image" + filepath.Ext(ext),
		"fileMimeType": mimeType,
		"content":      base64.StdEncoding.EncodeToString(data),
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+uploadEndpoint, bytes.NewReader(payload))
	if err != nil {
		return "", "", err
	}
	setGrokHeaders(req, token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	resp.Body = decodeResponseBody(resp)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode/100 != 2 {
		return "", "", fmt.Errorf("grok upload HTTP %d: %s", resp.StatusCode, snippet(raw, 240))
	}
	var obj map[string]any
	_ = json.Unmarshal(raw, &obj)
	fileID := firstStringByKeys(obj, []string{"fileMetadataId", "file_id", "fileId", "id"})
	for _, key := range []string{"fileUri", "file_uri", "fileUrl", "url", "mediaUrl"} {
		if s, _ := obj[key].(string); s != "" {
			return fileID, normalizeAssetURL(s), nil
		}
	}
	return fileID, normalizeAssetURL(firstStringByKey(obj, "url")), nil
}

func (c *WebClient) uploadChatFiles(ctx context.Context, token string, files []string) ([]any, error) {
	out := make([]any, 0, len(files))
	for _, item := range files {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if strings.HasPrefix(item, "data:") {
			fileID, fileURL, err := c.uploadDataURLMeta(ctx, token, item)
			if err != nil {
				return nil, err
			}
			if fileID != "" {
				out = append(out, fileID)
			} else if fileURL != "" {
				out = append(out, fileURL)
			}
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

func jsonSnippet(v any, limit int) string {
	raw, _ := json.Marshal(v)
	return snippet(raw, limit)
}

func sanitizeDiagURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	if i := strings.Index(rawURL, "?"); i >= 0 {
		rawURL = rawURL[:i]
	}
	return rawURL
}

func normalizeAssetURL(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
		return v
	}
	if strings.HasPrefix(v, "/") {
		return "https://assets.grok.com" + v
	}
	return "https://assets.grok.com/" + v
}

func isGrokAssetURL(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	return strings.Contains(v, "://assets.grok.com/") || strings.Contains(v, "://imagine-public.x.ai/")
}

func detectImageMime(data []byte) string {
	if len(data) >= 12 {
		switch {
		case bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}):
			return "image/png"
		case bytes.HasPrefix(data, []byte{0xff, 0xd8, 0xff}):
			return "image/jpeg"
		case bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a")):
			return "image/gif"
		case bytes.HasPrefix(data, []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
			return "image/webp"
		case bytes.Equal(data[4:8], []byte("ftyp")) && (bytes.Equal(data[8:12], []byte("avif")) || bytes.Equal(data[8:12], []byte("avis"))):
			return "image/avif"
		case bytes.Equal(data[4:8], []byte("ftyp")) && (bytes.Equal(data[8:12], []byte("heic")) || bytes.Equal(data[8:12], []byte("heix")) || bytes.Equal(data[8:12], []byte("hevc")) || bytes.Equal(data[8:12], []byte("hevx"))):
			return "image/heic"
		}
	}
	if len(data) >= 4 {
		switch {
		case bytes.HasPrefix(data, []byte("BM")):
			return "image/bmp"
		case bytes.HasPrefix(data, []byte("II*\x00")) || bytes.HasPrefix(data, []byte("MM\x00*")):
			return "image/tiff"
		}
	}
	return http.DetectContentType(data)
}

func setGrokHeaders(req *http.Request, token string) {
	cookie := buildGrokCookie(token)
	ua := grokUserAgent()
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Baggage", "sentry-environment=production,sentry-release=d6add6fb0460641fd482d767a335ef72b9b6abb8,sentry-public_key=b311e0f2690c81f25e2c4cf6d4f7ce1c")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Origin", webBaseURL)
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Priority", "u=1, i")
	req.Header.Set("Referer", webBaseURL+"/")
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Sec-Ch-Ua", grokSecCHUA(ua))
	req.Header.Set("Sec-Ch-Ua-Arch", "x86")
	req.Header.Set("Sec-Ch-Ua-Bitness", "64")
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Model", "")
	req.Header.Set("Sec-Ch-Ua-Platform", grokSecPlatform(ua))
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("X-Statsig-ID", grokStatsigID())
	req.Header.Set("X-XAI-Request-ID", uuid.NewString())
}

func normalizeGrokToken(cred string) string {
	cred = strings.TrimSpace(cred)
	if strings.Contains(cred, "sso=") {
		parts := strings.Split(cred, ";")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if strings.HasPrefix(p, "sso=") {
				return strings.TrimPrefix(p, "sso=")
			}
		}
	}
	return cred
}

func buildGrokCookie(cred string) string {
	cred = strings.TrimSpace(cred)
	state := readGrokRuntimeCFState()
	cf := strings.TrimSpace(firstNonEmpty(state.CFClearance, os.Getenv("KLEIN_GROK_CF_CLEARANCE")))
	extraCookies := normalizeCookieEnv(firstNonEmpty(state.Cookies, os.Getenv("KLEIN_GROK_CF_COOKIES")))
	if strings.Contains(cred, "=") {
		if !strings.Contains(cred, "sso-rw=") {
			token := normalizeGrokToken(cred)
			if token != "" {
				cred = strings.TrimRight(cred, "; ") + "; sso-rw=" + token
			}
		}
		if cf != "" && !strings.Contains(cred, "cf_clearance=") {
			cred = strings.TrimRight(cred, "; ") + "; cf_clearance=" + cf
		}
		return appendMissingCookies(cred, extraCookies)
	}
	cookie := "sso=" + cred + "; sso-rw=" + cred
	if cf != "" {
		cookie += "; cf_clearance=" + cf
	}
	return appendMissingCookies(cookie, extraCookies)
}

func readGrokRuntimeCFState() grokRuntimeCFState {
	grokCFCache.Lock()
	defer grokCFCache.Unlock()
	if time.Since(grokCFCache.loaded) < 30*time.Second {
		return grokCFCache.state
	}
	grokCFCache.loaded = time.Now()
	grokCFCache.state = grokRuntimeCFState{}
	path := strings.TrimSpace(os.Getenv("KLEIN_GROK_CF_STATE_PATH"))
	if path == "" {
		path = "/app/storage/grok_cf.json"
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return grokCFCache.state
	}
	_ = json.Unmarshal(raw, &grokCFCache.state)
	return grokCFCache.state
}

func grokUserAgent() string {
	state := readGrokRuntimeCFState()
	if ua := strings.TrimSpace(firstNonEmpty(state.UserAgent, os.Getenv("KLEIN_GROK_USER_AGENT"))); ua != "" {
		return ua
	}
	return grokUA
}

func grokSecCHUA(ua string) string {
	v := "136"
	if m := regexp.MustCompile(`(?:Chrome|Chromium)/(\d+)`).FindStringSubmatch(ua); len(m) == 2 {
		v = m[1]
	}
	return fmt.Sprintf(`"Google Chrome";v="%s", "Chromium";v="%s", "Not(A:Brand";v="24"`, v, v)
}

func grokSecPlatform(ua string) string {
	ua = strings.ToLower(ua)
	switch {
	case strings.Contains(ua, "windows"):
		return `"Windows"`
	case strings.Contains(ua, "mac os"):
		return `"macOS"`
	case strings.Contains(ua, "linux"):
		return `"Linux"`
	default:
		return `"Windows"`
	}
}

func grokStatsigID() string {
	if !envBool("KLEIN_GROK_DYNAMIC_STATSIG", true) {
		return "ZTpUeXBlRXJyb3I6IENhbm5vdCByZWFkIHByb3BlcnRpZXMgb2YgdW5kZWZpbmVkIChyZWFkaW5nICdjaGlsZE5vZGVzJyk="
	}
	messages := []string{
		"Cannot read properties of undefined (reading 'childNodes')",
		"Cannot read properties of null (reading 'parentNode')",
		"Cannot read properties of undefined (reading 'firstChild')",
	}
	msg := messages[int(time.Now().UnixNano()%int64(len(messages)))]
	return base64.StdEncoding.EncodeToString([]byte("e:TypeError: " + msg))
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func envBool(key string, fallback bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func shouldNormalizeGrokUpload() bool {
	return envBool("KLEIN_GROK_UPLOAD_NORMALIZE_JPEG", true)
}

func normalizeImageForGrokUpload(data []byte) ([]byte, bool) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil || img == nil {
		return nil, false
	}
	b := img.Bounds()
	canvas := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(canvas, canvas.Bounds(), img, b.Min, draw.Over)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, canvas, &jpeg.Options{Quality: 92}); err != nil {
		return nil, false
	}
	return buf.Bytes(), true
}

func normalizeCookieEnv(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	v = strings.TrimPrefix(v, "Cookie:")
	v = strings.TrimSpace(v)
	return strings.TrimRight(v, "; ")
}

func appendMissingCookies(cookie, extra string) string {
	if extra == "" {
		return cookie
	}
	out := strings.TrimRight(strings.TrimSpace(cookie), "; ")
	for _, part := range strings.Split(extra, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name := part
		if i := strings.Index(part, "="); i >= 0 {
			name = strings.TrimSpace(part[:i])
		}
		if name == "" || strings.Contains(out, name+"=") {
			continue
		}
		if out != "" {
			out += "; "
		}
		out += part
	}
	return out
}

func decodeResponseBody(resp *http.Response) io.ReadCloser {
	if resp == nil || resp.Body == nil {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding"))) {
	case "gzip":
		zr, err := gzip.NewReader(resp.Body)
		if err == nil {
			resp.Header.Del("Content-Encoding")
			return &joinedReadCloser{Reader: zr, closers: []io.Closer{zr, resp.Body}}
		}
	}
	return resp.Body
}

type joinedReadCloser struct {
	io.Reader
	closers []io.Closer
}

func (r *joinedReadCloser) Close() error {
	var first error
	for _, closer := range r.closers {
		if err := closer.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func collectGrokStream(r io.Reader, onText func(string)) (string, string, string, string, error) {
	var out strings.Builder
	videoURL := ""
	thumbURL := ""
	videoPostID := ""
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		}
		if line == "" || line == "[DONE]" {
			continue
		}
		var obj any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			continue
		}
		if errMsg := firstStringByKey(obj, "error"); errMsg != "" {
			return out.String(), videoURL, thumbURL, videoPostID, fmt.Errorf("grok stream error: %s", errMsg)
		}
		if s := firstStringByKeys(obj, []string{"videoPostId", "video_post_id", "assetId", "asset_id", "videoId", "video_id", "postId", "post_id"}); s != "" {
			videoPostID = s
		}
		if u := firstVideoURL(obj); u != "" {
			videoURL = normalizeAssetURL(u)
			if videoPostID == "" {
				videoPostID = extractUUID(videoURL)
			}
		}
		if u := firstStringByKeys(obj, []string{"thumbnailImageUrl", "thumbnailUrl", "coverUrl"}); u != "" {
			thumbURL = normalizeAssetURL(u)
		}
		delta := extractTextDelta(obj)
		if delta != "" {
			out.WriteString(delta)
			if onText != nil {
				onText(delta)
			}
		}
	}
	return out.String(), videoURL, thumbURL, videoPostID, sc.Err()
}

func firstVideoURL(v any) string {
	for _, key := range []string{"videoUrl", "videoURL", "video_url", "mediaUrl", "result_url"} {
		if s := firstStringByKey(v, key); isVideoURLCandidate(s) {
			return s
		}
	}
	return firstVideoString(v)
}

func firstVideoString(v any) string {
	switch x := v.(type) {
	case map[string]any:
		for _, child := range x {
			if s := firstVideoString(child); s != "" {
				return s
			}
		}
	case []any:
		for _, child := range x {
			if s := firstVideoString(child); s != "" {
				return s
			}
		}
	case string:
		if isVideoURLCandidate(x) {
			return x
		}
	}
	return ""
}

func isVideoURLCandidate(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	lower := strings.ToLower(s)
	if strings.Contains(lower, "preview_image") || strings.Contains(lower, "thumbnail") ||
		strings.HasSuffix(lower, ".jpg") || strings.HasSuffix(lower, ".jpeg") ||
		strings.HasSuffix(lower, ".png") || strings.HasSuffix(lower, ".webp") {
		return false
	}
	return strings.Contains(lower, ".mp4") ||
		strings.Contains(lower, ".webm") ||
		strings.Contains(lower, "generated_video") ||
		strings.Contains(lower, "/video/")
}

func derivePreviewImageURL(videoURL string) string {
	v := strings.TrimSpace(videoURL)
	if v == "" {
		return ""
	}
	lower := strings.ToLower(v)
	for _, marker := range []string{"/generated_video.mp4", "/generated_video.webm", "/generated_video"} {
		if idx := strings.LastIndex(lower, marker); idx >= 0 {
			return v[:idx] + "/preview_image.jpg"
		}
	}
	if strings.HasSuffix(lower, ".mp4") || strings.HasSuffix(lower, ".webm") {
		if idx := strings.LastIndex(v, "/"); idx >= 0 {
			return v[:idx] + "/preview_image.jpg"
		}
	}
	return ""
}

var uuidRe = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

func extractUUID(s string) string {
	matches := uuidRe.FindAllString(strings.TrimSpace(s), -1)
	if len(matches) == 0 {
		return ""
	}
	return matches[len(matches)-1]
}

func extractTextDelta(v any) string {
	switch x := v.(type) {
	case map[string]any:
		for _, k := range []string{"token", "responseToken", "text"} {
			if s, ok := x[k].(string); ok && looksLikeTextDelta(s) {
				return s
			}
		}
		for _, child := range x {
			if s := extractTextDelta(child); s != "" {
				return s
			}
		}
	case []any:
		for _, child := range x {
			if s := extractTextDelta(child); s != "" {
				return s
			}
		}
	}
	return ""
}

func looksLikeTextDelta(s string) bool {
	if s == "" || strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return false
	}
	return true
}

var xaiToolCardPattern = regexp.MustCompile(`(?s)<xai:tool_usage_card>.*?</xai:tool_usage_card>`)

func cleanGrokText(s string) string {
	if s == "" {
		return ""
	}
	s = xaiToolCardPattern.ReplaceAllString(s, "")
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if looksLikeXAIToolMarkup(line) {
			continue
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func looksLikeXAIToolMarkup(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "<xai:") ||
		strings.HasPrefix(s, "</xai:") ||
		strings.Contains(s, "<xai:tool_usage_card") ||
		strings.Contains(s, "<xai:tool_name>") ||
		strings.Contains(s, "<xai:tool_args>")
}

func firstStringByKeys(v any, keys []string) string {
	for _, key := range keys {
		if s := firstStringByKey(v, key); s != "" {
			return s
		}
	}
	return ""
}

func firstStringByKey(v any, key string) string {
	switch x := v.(type) {
	case map[string]any:
		if s, ok := x[key].(string); ok && s != "" {
			return s
		}
		for _, child := range x {
			if s := firstStringByKey(child, key); s != "" {
				return s
			}
		}
	case []any:
		for _, child := range x {
			if s := firstStringByKey(child, key); s != "" {
				return s
			}
		}
	}
	return ""
}

func buildGrokPrompt(body map[string]any) string {
	prompt, _ := buildGrokPromptAndFiles(body)
	return prompt
}

func buildGrokPromptAndFiles(body map[string]any) (string, []string) {
	msgs := normalizeAnySlice(body["messages"])
	if len(msgs) == 0 {
		return "", nil
	}
	var b strings.Builder
	var files []string
	for _, item := range msgs {
		m, _ := item.(map[string]any)
		role, _ := m["role"].(string)
		content, imgs := messageContentAndFiles(m["content"])
		files = append(files, imgs...)
		if strings.TrimSpace(content) == "" {
			continue
		}
		if role == "" {
			role = "user"
		}
		b.WriteString(role)
		b.WriteString(": ")
		b.WriteString(content)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String()), files
}

func messageContent(v any) string {
	content, _ := messageContentAndFiles(v)
	return content
}

func messageContentAndFiles(v any) (string, []string) {
	switch c := v.(type) {
	case string:
		return c, nil
	case []map[string]any:
		items := make([]any, 0, len(c))
		for _, item := range c {
			items = append(items, item)
		}
		return messageContentAndFiles(items)
	case []any:
		parts := make([]string, 0, len(c))
		files := make([]string, 0, 4)
		for _, p := range c {
			m, _ := p.(map[string]any)
			if m == nil {
				continue
			}
			typ, _ := m["type"].(string)
			if s, _ := m["text"].(string); s != "" {
				parts = append(parts, s)
			}
			if typ == "image_url" {
				if im, _ := m["image_url"].(map[string]any); im != nil {
					if u, _ := im["url"].(string); strings.TrimSpace(u) != "" {
						files = append(files, strings.TrimSpace(u))
					}
				}
			}
		}
		return strings.Join(parts, "\n"), files
	default:
		b, _ := json.Marshal(c)
		return string(b), nil
	}
}

func normalizeAnySlice(v any) []any {
	switch x := v.(type) {
	case []any:
		return x
	case []map[string]any:
		out := make([]any, 0, len(x))
		for _, item := range x {
			out = append(out, item)
		}
		return out
	default:
		return nil
	}
}

func estimateOpenAIUsage(prompt, completion string) *OpenAIUsage {
	u := &OpenAIUsage{
		PromptTokens:     len([]rune(prompt))/4 + 1,
		CompletionTokens: len([]rune(completion))/4 + 1,
	}
	u.TotalTokens = u.PromptTokens + u.CompletionTokens
	return u
}

func normalizeVideoDuration(sec int) int {
	for _, v := range []int{6, 10} {
		if sec <= v {
			return v
		}
	}
	return 10
}

func videoConfig(size, aspect string) (string, string, int, int) {
	switch strings.TrimSpace(aspect) {
	case "9:16":
		return "9:16", defaultVideoResolution, 720, 1280
	case "1:1":
		return "1:1", defaultVideoResolution, 1024, 1024
	case "16:9":
		return "16:9", defaultVideoResolution, 1280, 720
	}
	switch size {
	case "720x1280", "1024x1792":
		return "9:16", defaultVideoResolution, 720, 1280
	case "1024x1024":
		return "1:1", defaultVideoResolution, 1024, 1024
	case "1280x720", "1792x1024":
		return "16:9", defaultVideoResolution, 1280, 720
	}
	return "16:9", defaultVideoResolution, 1280, 720
}

func shortID() string {
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	if len(id) > 26 {
		return id[:26]
	}
	return id
}
