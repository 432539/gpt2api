// Package chatgpt - 图像生成协议
//
// 完整链路(和文字聊天共用 f/conversation,只通过 system_hints=["picture_v2"] 区分):
//
//  0. (可选) GET /                              → 拿 oai-did cookie
//  1. POST /backend-api/f/conversation/prepare      → conduit_token
//  2. POST /backend-api/sentinel/chat-requirements → chat_token + 可选 POW 挑战
//  3. POST /backend-api/f/conversation (SSE)         → 边解析边收 file-service://
//  4. SSE 没直出 file-service 时轮询 GET /backend-api/conversation/{id}
//     任何一条 tool 消息出现 file-service / sediment asset_pointer 即算成功,
//     够 N 张立即返回;IMG2 已正式上线,不再做"灰度命中判定"。
//  5. GET /backend-api/files/{fid}/download                   → 签名 URL(file-service)
//     GET /backend-api/conversation/{cid}/attachment/{sid}/download → 签名 URL(sediment)
//  6. GET 签名 URL → 图片字节
//
// 注意:不要调用 /backend-api/conversation/init——这是老客户端路径,在免费账号上会
// 直接 404 让整条链路失败,上游把 picture_v2 路由完全交给 f/conversation 的 payload。
package chatgpt

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ImageConvOpts 是图像会话的入参。
type ImageConvOpts struct {
	Prompt         string          // 用户提示词(已处理完的,含可选 CLARITY_SUFFIX)
	UpstreamModel  string          // 默认 "gpt-5-3"
	ConvID         string          // 复用会话时填,空则新建
	ParentMsgID    string          // 复用会话时从 GetConversationHead 取;新会话随机
	MessageID      string          // 可选,留空自动生成
	ChatToken      string          // 必传,来自 ChatRequirements
	ProofToken     string          // 可选
	ConduitToken   string          // 可选,来自 PrepareFConversation
	TimezoneOffset int             // 默认 -480(Asia/Shanghai)
	SSETimeout     time.Duration   // 默认 120s
	References     []*UploadedFile // 图生图/编辑:已上传的参考图,会插到 prompt 前面
}

// InitConversation 对应 /backend-api/conversation/init。
// 新会话必须调用,否则后续 /f/conversation 会 404。
// systemHints 为空串数组表示文字聊天;图像场景传 []string{"picture_v2"}。
func (c *Client) InitConversation(ctx context.Context, systemHints ...string) error {
	if systemHints == nil {
		systemHints = []string{}
	}
	payload := map[string]interface{}{
		"gizmo_id":                nil,
		"requested_default_model": nil,
		"conversation_id":         nil,
		"timezone_offset_min":     -480,
		"system_hints":            systemHints,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.opts.BaseURL+"/backend-api/conversation/init",
		strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	c.commonHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")

	res, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		buf, _ := io.ReadAll(res.Body)
		return &UpstreamError{Status: res.StatusCode, Message: "conversation/init failed", Body: string(buf)}
	}
	_, _ = io.Copy(io.Discard, res.Body)
	return nil
}

// PrepareFConversation 对应 /backend-api/f/conversation/prepare,返回 conduit_token。
//
// payload 对齐 HAR 抓包 /f/conversation/prepare(image 通路):
//   - client_prepare_state: "success"
//   - fork_from_shared_post: false
//   - partial_query: 完整的 user message(id+author+content,包含当前 prompt)
//   - system_hints: ["picture_v2"]   ← image 通路特有
//   - client_contextual_info: { "app_name": "chatgpt.com" }   ← prepare 阶段只带 app_name
func (c *Client) PrepareFConversation(ctx context.Context, opt ImageConvOpts) (string, error) {
	if opt.UpstreamModel == "" {
		opt.UpstreamModel = "auto"
	}
	if opt.MessageID == "" {
		opt.MessageID = uuid.NewString()
	}
	payload := map[string]interface{}{
		"action":                "next",
		"fork_from_shared_post": false,
		"parent_message_id":     opt.ParentMsgID,
		"model":                 opt.UpstreamModel,
		"client_prepare_state":  "success",
		"timezone_offset_min":   -480,
		"timezone":              "Asia/Shanghai",
		"conversation_mode":     map[string]string{"kind": "primary_assistant"},
		"system_hints":          []string{"picture_v2"},
		"partial_query": map[string]interface{}{
			"id":     uuid.NewString(),
			"author": map[string]string{"role": "user"},
			"content": map[string]interface{}{
				"content_type": "text",
				"parts":        []string{opt.Prompt},
			},
		},
		"supports_buffering":  true,
		"supported_encodings": []string{"v1"},
		"client_contextual_info": map[string]interface{}{
			"app_name": "chatgpt.com",
		},
	}
	// 只有已有会话才带 conversation_id;新会话不带这个 key(对齐浏览器抓包,
	// 带陌生 UUID 上游会 404)
	if opt.ConvID != "" {
		payload["conversation_id"] = opt.ConvID
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.opts.BaseURL+"/backend-api/f/conversation/prepare",
		strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	c.commonHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Openai-Sentinel-Chat-Requirements-Token", opt.ChatToken)
	if opt.ProofToken != "" {
		req.Header.Set("Openai-Sentinel-Proof-Token", opt.ProofToken)
	}

	res, err := c.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	buf, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		return "", &UpstreamError{Status: res.StatusCode, Message: "f/conversation/prepare failed", Body: string(buf)}
	}
	var out struct {
		ConduitToken string `json:"conduit_token"`
	}
	_ = json.Unmarshal(buf, &out)
	return out.ConduitToken, nil
}

// StreamFConversation 对应 /backend-api/f/conversation(图像走和文字同一端点)。
//
// payload 字段集参考社区维护的 OpenaiChat provider(它在免费/付费账号上实测可用):
// 不带 client_prepare_state / force_parallel_switch;message.metadata 只带
// serialization_metadata + system_hints(有图时再补 attachments)。
func (c *Client) StreamFConversation(ctx context.Context, opt ImageConvOpts) (<-chan SSEEvent, error) {
	if opt.UpstreamModel == "" {
		opt.UpstreamModel = "auto"
	}
	if opt.MessageID == "" {
		opt.MessageID = uuid.NewString()
	}
	if opt.ParentMsgID == "" {
		opt.ParentMsgID = uuid.NewString()
	}
	if opt.TimezoneOffset == 0 {
		opt.TimezoneOffset = -480
	}
	if opt.SSETimeout == 0 {
		opt.SSETimeout = 180 * time.Second
	}

	// 构造 messages[0] 的 content 与 metadata,按是否有 reference 图决定 multimodal_text。
	// metadata 严格对齐 HAR 抓包的 **image 通路**:
	//   developer_mode_connector_ids: []
	//   selected_github_repos: []
	//   selected_all_github_repos: false
	//   system_hints: ["picture_v2"]            ← image 通路特有
	//   serialization_metadata.custom_symbol_offsets: []
	// 注意:image 通路 **不带 selected_sources**(那是 text 通路专属)。写错会让
	// 上游识别为"客户端类型不匹配",触发 silent rejection。
	msgContent := map[string]interface{}{"content_type": "text", "parts": []string{opt.Prompt}}
	msgMeta := map[string]interface{}{
		"developer_mode_connector_ids": []interface{}{},
		"selected_github_repos":        []interface{}{},
		"selected_all_github_repos":    false,
		"system_hints":                 []string{"picture_v2"},
		"serialization_metadata": map[string]interface{}{
			"custom_symbol_offsets": []interface{}{},
		},
	}
	if len(opt.References) > 0 {
		parts := make([]interface{}, 0, len(opt.References)+1)
		atts := make([]Attachment, 0, len(opt.References))
		for _, u := range opt.References {
			if u == nil || u.FileID == "" {
				continue
			}
			parts = append(parts, u.ToAssetPointerPart())
			atts = append(atts, u.ToAttachment())
		}
		parts = append(parts, opt.Prompt)
		msgContent = map[string]interface{}{
			"content_type": "multimodal_text",
			"parts":        parts,
		}
		msgMeta["attachments"] = atts
	}

	// 顶层 payload 对齐 HAR /f/conversation(image 通路):
	//   client_prepare_state: "sent"
	//   system_hints: ["picture_v2"]
	//   force_parallel_switch: "auto"            ← 必带
	//   client_contextual_info: 7 个字段 + app_name
	payload := map[string]interface{}{
		"action": "next",
		"messages": []map[string]interface{}{{
			"id":          opt.MessageID,
			"author":      map[string]string{"role": "user"},
			"create_time": float64(time.Now().UnixMilli()) / 1000.0,
			"content":     msgContent,
			"metadata":    msgMeta,
		}},
		"parent_message_id":        opt.ParentMsgID,
		"model":                    opt.UpstreamModel,
		"client_prepare_state":     "sent",
		"timezone_offset_min":      opt.TimezoneOffset,
		"timezone":                 "Asia/Shanghai",
		"conversation_mode":        map[string]string{"kind": "primary_assistant"},
		"enable_message_followups": true,
		"system_hints":             []string{"picture_v2"},
		"supports_buffering":       true,
		"supported_encodings":      []string{"v1"},
		"client_contextual_info": map[string]interface{}{
			"is_dark_mode":      false,
			"time_since_loaded": 1200,
			"page_height":       1072,
			"page_width":        1724,
			"pixel_ratio":       1.2,
			"screen_height":     1440,
			"screen_width":      2560,
			"app_name":          "chatgpt.com",
		},
		"paragen_cot_summary_display_override": "allow",
		"force_parallel_switch":                "auto",
	}
	// 新会话不带 conversation_id(对齐浏览器抓包);已有会话才带
	if opt.ConvID != "" {
		payload["conversation_id"] = opt.ConvID
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.opts.BaseURL+"/backend-api/f/conversation",
		strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	c.commonHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	// X-Oai-Turn-Trace-Id:每 turn 一个新 UUID。见 fchat.go 说明。
	req.Header.Set("X-Oai-Turn-Trace-Id", uuid.NewString())
	req.Header.Set("Openai-Sentinel-Chat-Requirements-Token", opt.ChatToken)
	if opt.ProofToken != "" {
		req.Header.Set("Openai-Sentinel-Proof-Token", opt.ProofToken)
	}
	if opt.ConduitToken != "" {
		req.Header.Set("X-Conduit-Token", opt.ConduitToken)
	}

	local := *c.hc
	local.Timeout = 0 // 由 ctx 控制

	res, err := local.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 400 {
		buf, _ := io.ReadAll(res.Body)
		header := res.Header.Clone()
		res.Body.Close()
		return nil, &UpstreamError{Status: res.StatusCode, Message: "f/conversation failed", Body: string(buf), Header: header}
	}
	out := make(chan SSEEvent, 64)
	go parseSSE(res.Body, out, opt.SSETimeout)
	return out, nil
}

// ImageSSEResult 是 ParseImageSSE 的扫描结果。
type ImageSSEResult struct {
	ConversationID string   // SSE 里捕获到的新会话 id(可能和入参不同)
	FileIDs        []string // file-service:// 引用(直出灰度图就在这里)
	SedimentIDs    []string // sediment:// 引用(通常是预览,需要轮询)
	FinishType     string   // finish_details.type(interrupted/stop/...)
	ImageGenTaskID string
	AssistantText  string // 没有出图时,assistant 往往会在这里返回拒绝/风控原因
}

var (
	reFileRef = regexp.MustCompile(`file-service://([A-Za-z0-9_-]+)`)
	reSedRef  = regexp.MustCompile(`sediment://([A-Za-z0-9_-]+)`)
)

// ParseImageSSE 消费 SSE 事件流,把图像相关的字段提取出来。
// 调用方可以根据返回的 FileIDs 判断是否已灰度直出。
func ParseImageSSE(stream <-chan SSEEvent) ImageSSEResult {
	var r ImageSSEResult
	seenFile := map[string]struct{}{}
	seenSed := map[string]struct{}{}
	var textFragments strings.Builder
	var fullMessages []string

	for ev := range stream {
		if ev.Err != nil {
			r.AssistantText = firstNonEmpty(lastText(fullMessages), compactAssistantText(textFragments.String()))
			return r
		}
		data := ev.Data
		if len(data) == 0 {
			continue
		}
		if string(data) == "[DONE]" {
			r.AssistantText = firstNonEmpty(lastText(fullMessages), compactAssistantText(textFragments.String()))
			return r
		}
		// 文本正则先扫一遍(比 JSON 解析更健壮)
		for _, m := range reFileRef.FindAllSubmatch(data, -1) {
			fid := string(m[1])
			if _, ok := seenFile[fid]; !ok {
				seenFile[fid] = struct{}{}
				r.FileIDs = append(r.FileIDs, fid)
			}
		}
		for _, m := range reSedRef.FindAllSubmatch(data, -1) {
			sid := string(m[1])
			if _, ok := seenSed[sid]; !ok {
				seenSed[sid] = struct{}{}
				r.SedimentIDs = append(r.SedimentIDs, sid)
			}
		}

		var obj map[string]interface{}
		if err := json.Unmarshal(data, &obj); err != nil {
			continue
		}
		if p, _ := obj["p"].(string); isAssistantTextPath(p) {
			if v, ok := obj["v"].(string); ok && isUsefulAssistantText(v) {
				textFragments.WriteString(v)
			}
		}
		if v, ok := obj["v"].(map[string]interface{}); ok {
			if cid, ok := v["conversation_id"].(string); ok && cid != "" && r.ConversationID == "" {
				r.ConversationID = cid
			}
			if msg, ok := v["message"].(map[string]interface{}); ok {
				if txt := extractAssistantTextFromMessage(msg); txt != "" {
					fullMessages = append(fullMessages, txt)
				}
				if meta, ok := msg["metadata"].(map[string]interface{}); ok {
					if tid, ok := meta["image_gen_task_id"].(string); ok {
						r.ImageGenTaskID = tid
					}
					if fd, ok := meta["finish_details"].(map[string]interface{}); ok {
						if ft, ok := fd["type"].(string); ok {
							r.FinishType = ft
						}
					}
				}
			}
		}
	}
	r.AssistantText = firstNonEmpty(lastText(fullMessages), compactAssistantText(textFragments.String()))
	return r
}

func isAssistantTextPath(path string) bool {
	path = strings.ToLower(path)
	return strings.Contains(path, "message") &&
		strings.Contains(path, "content") &&
		strings.Contains(path, "parts")
}

func extractAssistantTextFromMessage(msg map[string]interface{}) string {
	author, _ := msg["author"].(map[string]interface{})
	if author == nil {
		return ""
	}
	if role, _ := author["role"].(string); role != "assistant" {
		return ""
	}
	content, _ := msg["content"].(map[string]interface{})
	return extractTextContent(content)
}

func extractTextContent(content map[string]interface{}) string {
	if content == nil {
		return ""
	}
	texts := make([]string, 0, 2)
	if text, _ := content["text"].(string); isUsefulAssistantText(text) {
		texts = append(texts, text)
	}
	switch parts := content["parts"].(type) {
	case []interface{}:
		for _, p := range parts {
			switch v := p.(type) {
			case string:
				if isUsefulAssistantText(v) {
					texts = append(texts, v)
				}
			case map[string]interface{}:
				if text, _ := v["text"].(string); isUsefulAssistantText(text) {
					texts = append(texts, text)
				}
			}
		}
	case []string:
		for _, p := range parts {
			if isUsefulAssistantText(p) {
				texts = append(texts, p)
			}
		}
	}
	return compactAssistantText(strings.Join(texts, "\n"))
}

func isUsefulAssistantText(s string) bool {
	s = strings.TrimSpace(s)
	return s != "" &&
		!strings.Contains(s, "file-service://") &&
		!strings.Contains(s, "sediment://")
}

func compactAssistantText(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

func lastText(items []string) string {
	for i := len(items) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(items[i]); s != "" {
			return s
		}
	}
	return ""
}

func firstNonEmpty(items ...string) string {
	for _, s := range items {
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
	}
	return ""
}

// ImageToolMsg 是 conversation.mapping 里一条 IMG2 tool 消息的关键字段。
type ImageToolMsg struct {
	MessageID     string
	CreateTime    float64
	ModelSlug     string
	Recipient     string
	AuthorName    string
	ImageGenTitle string
	FileIDs       []string // file-service
	SedimentIDs   []string // sediment
}

// GetConversationMapping 读取会话全量 mapping(轮询用)。
func (c *Client) GetConversationMapping(ctx context.Context, convID string) (map[string]interface{}, error) {
	if convID == "" {
		return nil, fmt.Errorf("conv_id required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.opts.BaseURL+"/backend-api/conversation/"+convID, nil)
	if err != nil {
		return nil, err
	}
	c.commonHeaders(req)
	req.Header.Set("Accept", "*/*")

	res, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	buf, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		return nil, &UpstreamError{Status: res.StatusCode, Message: "conversation get failed", Body: string(buf)}
	}
	var out map[string]interface{}
	if err := json.Unmarshal(buf, &out); err != nil {
		return nil, fmt.Errorf("decode conversation: %w", err)
	}
	return out, nil
}

// ExtractImageToolMsgs 从 conversation.mapping 里提取所有 IMG2 tool 消息。
func ExtractImageToolMsgs(mapping map[string]interface{}) []ImageToolMsg {
	out := make([]ImageToolMsg, 0, 4)
	for mid, raw := range mapping {
		node, _ := raw.(map[string]interface{})
		if node == nil {
			continue
		}
		msg, _ := node["message"].(map[string]interface{})
		if msg == nil {
			continue
		}
		author, _ := msg["author"].(map[string]interface{})
		meta, _ := msg["metadata"].(map[string]interface{})
		content, _ := msg["content"].(map[string]interface{})
		if author == nil || meta == nil || content == nil {
			continue
		}
		if s, _ := author["role"].(string); s != "tool" {
			continue
		}
		if s, _ := meta["async_task_type"].(string); s != "image_gen" {
			continue
		}
		if s, _ := content["content_type"].(string); s != "multimodal_text" {
			continue
		}

		tm := ImageToolMsg{MessageID: mid}
		if v, ok := msg["create_time"].(float64); ok {
			tm.CreateTime = v
		}
		if v, ok := meta["model_slug"].(string); ok {
			tm.ModelSlug = v
		}
		if v, ok := msg["recipient"].(string); ok {
			tm.Recipient = v
		}
		if v, ok := author["name"].(string); ok {
			tm.AuthorName = v
		}
		if v, ok := meta["image_gen_title"].(string); ok {
			tm.ImageGenTitle = v
		}

		parts, _ := content["parts"].([]interface{})
		seenF := map[string]struct{}{}
		seenS := map[string]struct{}{}
		extractAsset := func(text string) {
			for _, m := range reFileRef.FindAllStringSubmatch(text, -1) {
				if _, ok := seenF[m[1]]; !ok {
					seenF[m[1]] = struct{}{}
					tm.FileIDs = append(tm.FileIDs, m[1])
				}
			}
			for _, m := range reSedRef.FindAllStringSubmatch(text, -1) {
				if _, ok := seenS[m[1]]; !ok {
					seenS[m[1]] = struct{}{}
					tm.SedimentIDs = append(tm.SedimentIDs, m[1])
				}
			}
		}
		for _, p := range parts {
			switch v := p.(type) {
			case map[string]interface{}:
				if aid, _ := v["asset_pointer"].(string); aid != "" {
					extractAsset(aid)
				}
			case string:
				extractAsset(v)
			}
		}
		out = append(out, tm)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreateTime < out[j].CreateTime })
	return out
}

// ExtractAssistantTextMsgs 从 conversation.mapping 里提取 assistant 文本消息。
// 生图被上游安全策略/版权相似性等规则拒绝时,通常不会出现 image_gen tool 消息,
// 而是直接返回一条普通 assistant 文本;调用方用它作为 task.error 的可读原因。
func ExtractAssistantTextMsgs(mapping map[string]interface{}) []string {
	type textMsg struct {
		createTime float64
		text       string
	}
	msgs := make([]textMsg, 0, 4)
	for _, raw := range mapping {
		node, _ := raw.(map[string]interface{})
		if node == nil {
			continue
		}
		msg, _ := node["message"].(map[string]interface{})
		if msg == nil {
			continue
		}
		text := extractAssistantTextFromMessage(msg)
		if text == "" {
			continue
		}
		tm := textMsg{text: text}
		if v, ok := msg["create_time"].(float64); ok {
			tm.createTime = v
		}
		msgs = append(msgs, tm)
	}
	sort.SliceStable(msgs, func(i, j int) bool { return msgs[i].createTime < msgs[j].createTime })
	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, m.text)
	}
	return out
}

// PollOpts 控制 PollConversationForImages 的等待策略。
//
// IMG2 正式上线后不再做灰度命中判定,逻辑极简化:
//   - 任一 tool 消息里出现 file-service / sediment asset_pointer → 立即算成功
//   - 够 ExpectedN 张就立即返回,不等多余的
//   - 没够数继续轮询,到 MaxWait 仍有至少 1 张也算成功(速度优先)
//   - 全程没拿到图才是 timeout
type PollOpts struct {
	BaselineToolIDs map[string]struct{} // 发送前已存在的 tool 消息 id,本次回合只看新增
	ExpectedN       int                 // 期望返回的图片张数,够了立即短路,默认 1
	MaxWait         time.Duration       // 总超时,默认 300s(上游渲染慢时兜底补齐)
	Interval        time.Duration       // 轮询间隔,默认 3s
}

// PollStatus 是 PollConversationForImages 的结果状态。
type PollStatus string

const (
	PollStatusSuccess  PollStatus = "success"  // 至少拿到 1 张图
	PollStatusTimeout  PollStatus = "timeout"  // 等到 MaxWait 仍然 0 张
	PollStatusError    PollStatus = "error"    // 上游错误(429 熔断 / ctx 取消)
	PollStatusRejected PollStatus = "rejected" // 上游明确返回拒绝/安全策略文本
)

// PollConversationForImages 轮询会话直到图片可用。
//
// 返回 (status, file_ids, sediment_ids, assistant_text)。
// status=success 时 file_ids/sediment_ids 至少一个非空。
// file-service 优先(优先级更高),sediment 作为补充一并带出,调用方自行决定用几张。
func (c *Client) PollConversationForImages(ctx context.Context, convID string, opt PollOpts) (PollStatus, []string, []string, string) {
	if strings.TrimSpace(convID) == "" {
		return PollStatusError, nil, nil, ""
	}
	if opt.ExpectedN <= 0 {
		opt.ExpectedN = 1
	}
	if opt.MaxWait <= 0 {
		opt.MaxWait = 300 * time.Second
	}
	if opt.Interval <= 0 {
		opt.Interval = 3 * time.Second
	}
	baseline := opt.BaselineToolIDs

	deadline := time.Now().Add(opt.MaxWait)

	// 累计全程看到的 fid/sid,循环外可用(超时兜底:有图就算成功)
	var (
		allFile        []string
		allSed         []string
		seenFile       = map[string]struct{}{}
		seenSed        = map[string]struct{}{}
		consecutive429 int
		assistantText  string
	)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return PollStatusError, nil, nil, assistantText
		default:
		}

		mapping, err := c.getMappingRaw(ctx, convID)
		if err != nil {
			if ue, ok := err.(*UpstreamError); ok && ue.Status == 429 {
				consecutive429++
				if consecutive429 >= 3 {
					return PollStatusError, nil, nil, assistantText
				}
				sleep(ctx, 10*time.Second)
				continue
			}
			sleep(ctx, opt.Interval)
			continue
		}
		consecutive429 = 0
		if texts := ExtractAssistantTextMsgs(mapping); len(texts) > 0 {
			assistantText = texts[len(texts)-1]
		}

		msgs := ExtractImageToolMsgs(mapping)
		// baseline diff:只看本回合新增 tool 消息
		var newMsgs []ImageToolMsg
		if len(baseline) > 0 {
			for _, m := range msgs {
				if _, ok := baseline[m.MessageID]; !ok {
					newMsgs = append(newMsgs, m)
				}
			}
		} else {
			newMsgs = msgs
		}

		// 跨 tool 消息聚合 fid/sid。IMG2 一次调用可能先出 sediment 预览再补
		// file-service 终稿,或同一条消息里带 N 张 file id;这里都累计起来。
		for _, m := range newMsgs {
			for _, f := range m.FileIDs {
				if _, ok := seenFile[f]; !ok {
					seenFile[f] = struct{}{}
					allFile = append(allFile, f)
				}
			}
			for _, s := range m.SedimentIDs {
				if _, ok := seenSed[s]; !ok {
					seenSed[s] = struct{}{}
					allSed = append(allSed, s)
				}
			}
		}

		// 够 N 张立即短路返回(file-service 优先占配额,sediment 补位)
		if len(allFile)+len(allSed) >= opt.ExpectedN {
			return PollStatusSuccess, allFile, allSed, assistantText
		}
		if isTerminalImageRejectionText(assistantText) {
			return PollStatusRejected, nil, nil, assistantText
		}

		sleep(ctx, opt.Interval)
	}

	// 超时兜底:只要拿到过至少 1 张,就算成功(速度优先,不等齐 N)
	if len(allFile)+len(allSed) > 0 {
		return PollStatusSuccess, allFile, allSed, assistantText
	}
	return PollStatusTimeout, nil, nil, assistantText
}

func isTerminalImageRejectionText(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return false
	}
	strong := []string{
		"违反", "防护限制", "安全策略", "政策", "无法生成", "不能生成", "不可以生成",
		"policy", "safety", "cannot generate", "can't generate", "unable to generate",
	}
	for _, kw := range strong {
		if strings.Contains(s, kw) {
			return strings.Contains(s, "抱歉") ||
				strings.Contains(s, "无法") ||
				strings.Contains(s, "不能") ||
				strings.Contains(s, "sorry") ||
				strings.Contains(s, "cannot") ||
				strings.Contains(s, "can't") ||
				strings.Contains(s, "unable")
		}
	}
	return false
}

// getMappingRaw 拉 conversation 并返回 mapping。
func (c *Client) getMappingRaw(ctx context.Context, convID string) (map[string]interface{}, error) {
	full, err := c.GetConversationMapping(ctx, convID)
	if err != nil {
		return nil, err
	}
	mapping, _ := full["mapping"].(map[string]interface{})
	if mapping == nil {
		mapping = map[string]interface{}{}
	}
	return mapping, nil
}

// GetConversationHead 返回会话最新叶子消息 id(current_node),复用会话时做 parent_message_id。
func (c *Client) GetConversationHead(ctx context.Context, convID string) (string, error) {
	full, err := c.GetConversationMapping(ctx, convID)
	if err != nil {
		return "", err
	}
	head, _ := full["current_node"].(string)
	if head == "" {
		return "", fmt.Errorf("current_node missing")
	}
	return head, nil
}

// ImageDownloadURL 取单张图的签名下载 URL。
// fileRef 可以是:
//   - "xxxxxx"      → /backend-api/files/{fid}/download(file-service)
//   - "sed:xxxxxx"  → /backend-api/conversation/{cid}/attachment/{fid}/download(需要 convID)
func (c *Client) ImageDownloadURL(ctx context.Context, convID, fileRef string) (string, error) {
	var apiURL string
	if strings.HasPrefix(fileRef, "sed:") {
		if convID == "" {
			return "", fmt.Errorf("conv_id required for sediment")
		}
		fid := strings.TrimPrefix(fileRef, "sed:")
		apiURL = fmt.Sprintf("%s/backend-api/conversation/%s/attachment/%s/download",
			c.opts.BaseURL, url.PathEscape(convID), url.PathEscape(fid))
	} else {
		apiURL = fmt.Sprintf("%s/backend-api/files/%s/download",
			c.opts.BaseURL, url.PathEscape(fileRef))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	c.commonHeaders(req)
	req.Header.Set("Accept", "*/*")

	res, err := c.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	buf, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		return "", &UpstreamError{Status: res.StatusCode, Message: "files/download failed", Body: string(buf)}
	}
	var out struct {
		DownloadURL string `json:"download_url"`
		Status      string `json:"status"`
		FileName    string `json:"file_name"`
	}
	if err := json.Unmarshal(buf, &out); err != nil {
		return "", fmt.Errorf("decode files/download: %w", err)
	}
	if out.DownloadURL == "" {
		return "", fmt.Errorf("empty download_url (status=%s)", out.Status)
	}
	return out.DownloadURL, nil
}

// FetchImage 下载指定 URL 的图片字节(调用 ImageDownloadURL 返回的签名 URL)。
// 返回 (bytes, content_type)。超过 maxBytes 的响应会被截断报错。
//
// 鉴权策略:
//   - files.oaiusercontent.com / cdn.oaistatic.com 等**预签名直链**:不带 Authorization,
//     它们依赖 URL 自带的 sig 参数鉴权;带 Bearer 反而会被某些 CDN 因"双鉴权"400。
//   - chatgpt.com/backend-api/estuary/... (sediment 回源 URL):**必须** 带 Authorization:
//     Bearer 和完整 Oai-* 头,否则上游 403。这就是 IMG2 sediment 图 502 的根因。
func (c *Client) FetchImage(ctx context.Context, signedURL string, maxBytes int64) ([]byte, string, error) {
	if maxBytes <= 0 {
		maxBytes = 16 * 1024 * 1024 // 16MB 默认
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, signedURL, nil)
	if err != nil {
		return nil, "", err
	}

	// 判断是否需要完整 chatgpt 鉴权头:以 BaseURL(通常 https://chatgpt.com)开头的
	// estuary / attachment 回源 URL 必须带 Bearer + Oai-* 头;外部 CDN 直链不带。
	needAuth := strings.HasPrefix(signedURL, c.opts.BaseURL+"/")
	if needAuth {
		c.commonHeaders(req)
		req.Header.Set("Accept", "image/*,*/*;q=0.8")
	} else {
		req.Header.Set("User-Agent", c.opts.UserAgent)
	}

	res, err := c.hc.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return nil, "", &UpstreamError{Status: res.StatusCode, Message: "fetch image failed"}
	}
	ct := res.Header.Get("Content-Type")
	body, err := io.ReadAll(io.LimitReader(res.Body, maxBytes+1))
	if err != nil {
		return nil, ct, err
	}
	if int64(len(body)) > maxBytes {
		return nil, ct, fmt.Errorf("image exceeds max bytes (%d)", maxBytes)
	}
	return body, ct, nil
}

// sleep 可取消的 sleep。
func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
