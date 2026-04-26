package chatgpt

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseImageSSECapturesAssistantRefusalMessage(t *testing.T) {
	stream := make(chan SSEEvent, 2)
	stream <- SSEEvent{Data: []byte(`{"v":{"conversation_id":"conv_123","message":{"author":{"role":"assistant"},"content":{"content_type":"text","parts":["非常抱歉，该提示可能违反了关于与第三方内容相似性的防护限制。"]},"metadata":{"finish_details":{"type":"stop"}}}}}`)}
	stream <- SSEEvent{Data: []byte("[DONE]")}
	close(stream)

	got := ParseImageSSE(stream)
	if got.ConversationID != "conv_123" {
		t.Fatalf("ConversationID = %q, want conv_123", got.ConversationID)
	}
	if got.AssistantText != "非常抱歉，该提示可能违反了关于与第三方内容相似性的防护限制。" {
		t.Fatalf("AssistantText = %q", got.AssistantText)
	}
	if len(got.FileIDs) != 0 || len(got.SedimentIDs) != 0 {
		t.Fatalf("unexpected image refs: file=%v sediment=%v", got.FileIDs, got.SedimentIDs)
	}
}

func TestParseImageSSECapturesAssistantTextFragments(t *testing.T) {
	stream := make(chan SSEEvent, 3)
	stream <- SSEEvent{Data: []byte(`{"p":"/message/content/parts/0","o":"append","v":"非常抱歉，"}`)}
	stream <- SSEEvent{Data: []byte(`{"p":"/message/content/parts/0","o":"append","v":"无法生成该图片。"}`)}
	stream <- SSEEvent{Data: []byte("[DONE]")}
	close(stream)

	got := ParseImageSSE(stream)
	if got.AssistantText != "非常抱歉，无法生成该图片。" {
		t.Fatalf("AssistantText = %q", got.AssistantText)
	}
}

func TestParseImageSSEPrefersAssistantMessageOverFragments(t *testing.T) {
	stream := make(chan SSEEvent, 3)
	stream <- SSEEvent{Data: []byte(`{"p":"/message/content/parts/0","o":"append","v":"转为宫崎骏动画风格"}`)}
	stream <- SSEEvent{Data: []byte(`{"v":{"message":{"author":{"role":"assistant"},"content":{"content_type":"text","parts":["非常抱歉，无法生成该图片。"]}}}}`)}
	stream <- SSEEvent{Data: []byte("[DONE]")}
	close(stream)

	got := ParseImageSSE(stream)
	if got.AssistantText != "非常抱歉，无法生成该图片。" {
		t.Fatalf("AssistantText = %q", got.AssistantText)
	}
}

func TestExtractAssistantTextMsgsIgnoresUserAndAssets(t *testing.T) {
	mapping := map[string]interface{}{
		"user": map[string]interface{}{
			"message": map[string]interface{}{
				"create_time": float64(1),
				"author":      map[string]interface{}{"role": "user"},
				"content":     map[string]interface{}{"content_type": "text", "parts": []interface{}{"转为宫崎骏动画风格"}},
			},
		},
		"tool": map[string]interface{}{
			"message": map[string]interface{}{
				"create_time": float64(2),
				"author":      map[string]interface{}{"role": "assistant"},
				"content":     map[string]interface{}{"content_type": "text", "parts": []interface{}{"file-service://abc123"}},
			},
		},
		"assistant": map[string]interface{}{
			"message": map[string]interface{}{
				"create_time": float64(3),
				"author":      map[string]interface{}{"role": "assistant"},
				"content":     map[string]interface{}{"content_type": "text", "parts": []interface{}{"如果你认为此判断有误，请重试或修改提示语。"}},
			},
		},
	}

	got := ExtractAssistantTextMsgs(mapping)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1: %#v", len(got), got)
	}
	if got[0] != "如果你认为此判断有误，请重试或修改提示语。" {
		t.Fatalf("message = %q", got[0])
	}
}

func TestPollConversationForImagesReturnsErrorWithoutConversationID(t *testing.T) {
	var c Client
	start := time.Now()
	status, fids, sids, assistantText := c.PollConversationForImages(
		context.Background(),
		"",
		PollOpts{MaxWait: time.Hour},
	)

	if status != PollStatusError {
		t.Fatalf("status = %q, want %q", status, PollStatusError)
	}
	if len(fids) != 0 || len(sids) != 0 || assistantText != "" {
		t.Fatalf("unexpected result: fids=%v sids=%v text=%q", fids, sids, assistantText)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("empty convID should fail fast, took %s", time.Since(start))
	}
}

func TestPollConversationForImagesReturnsRejectedOnAssistantRefusal(t *testing.T) {
	conversation := map[string]interface{}{
		"mapping": map[string]interface{}{
			"assistant": map[string]interface{}{
				"message": map[string]interface{}{
					"create_time": float64(1),
					"author":      map[string]interface{}{"role": "assistant"},
					"content": map[string]interface{}{
						"content_type": "text",
						"parts": []interface{}{
							"非常抱歉，该提示可能违反了关于与第三方内容相似性的防护限制。如果你认为此判断有误，请重试或修改提示语。",
						},
					},
				},
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/conversation/conv_rejected" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(conversation)
	}))
	defer srv.Close()

	c := &Client{opts: Options{BaseURL: srv.URL}, hc: srv.Client()}
	start := time.Now()
	status, fids, sids, assistantText := c.PollConversationForImages(
		context.Background(),
		"conv_rejected",
		PollOpts{MaxWait: time.Hour, Interval: time.Hour},
	)

	if status != PollStatusRejected {
		t.Fatalf("status = %q, want %q", status, PollStatusRejected)
	}
	if len(fids) != 0 || len(sids) != 0 {
		t.Fatalf("unexpected image refs: fids=%v sids=%v", fids, sids)
	}
	if assistantText == "" {
		t.Fatalf("assistantText is empty")
	}
	if time.Since(start) > time.Second {
		t.Fatalf("rejection should fail fast, took %s", time.Since(start))
	}
}

func TestPollConversationForImagesExcludesReferenceFileIDs(t *testing.T) {
	calls := 0
	conversations := []map[string]interface{}{
		{
			"mapping": map[string]interface{}{
				"tool_ref": imageToolMappingNode(1, []interface{}{
					map[string]interface{}{"asset_pointer": "file-service://file_uploaded_ref"},
				}),
			},
		},
		{
			"mapping": map[string]interface{}{
				"tool_ref": imageToolMappingNode(1, []interface{}{
					map[string]interface{}{"asset_pointer": "file-service://file_uploaded_ref"},
				}),
				"tool_result": imageToolMappingNode(2, []interface{}{
					map[string]interface{}{"asset_pointer": "file-service://file_generated_result"},
				}),
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/conversation/conv_image" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		idx := calls
		if idx >= len(conversations) {
			idx = len(conversations) - 1
		}
		calls++
		_ = json.NewEncoder(w).Encode(conversations[idx])
	}))
	defer srv.Close()

	c := &Client{opts: Options{BaseURL: srv.URL}, hc: srv.Client()}
	status, fids, sids, assistantText := c.PollConversationForImages(
		context.Background(),
		"conv_image",
		PollOpts{
			ExpectedN:      1,
			ExcludeFileIDs: map[string]struct{}{"file_uploaded_ref": {}},
			MaxWait:        time.Second,
			Interval:       10 * time.Millisecond,
		},
	)

	if status != PollStatusSuccess {
		t.Fatalf("status = %q, want %q (text=%q)", status, PollStatusSuccess, assistantText)
	}
	if len(fids) != 1 || fids[0] != "file_generated_result" {
		t.Fatalf("fids = %#v, want generated result only", fids)
	}
	if len(sids) != 0 {
		t.Fatalf("sids = %#v, want empty", sids)
	}
}

func imageToolMappingNode(createTime float64, parts []interface{}) map[string]interface{} {
	return map[string]interface{}{
		"message": map[string]interface{}{
			"create_time": createTime,
			"author":      map[string]interface{}{"role": "tool", "name": "image_gen"},
			"recipient":   "image_gen",
			"metadata":    map[string]interface{}{"async_task_type": "image_gen"},
			"content": map[string]interface{}{
				"content_type": "multimodal_text",
				"parts":        parts,
			},
		},
	}
}

func TestUpstreamErrorConversationIDFromHeaderAndBody(t *testing.T) {
	err := &UpstreamError{Header: http.Header{"Openai-Conversation-Id": []string{"conv_header"}}}
	if got := err.ConversationID(); got != "conv_header" {
		t.Fatalf("header ConversationID() = %q, want conv_header", got)
	}

	err = &UpstreamError{Body: `{"skipped_mainline":true,"conversation_id":"conv_body"}`}
	if got := err.ConversationID(); got != "conv_body" {
		t.Fatalf("body ConversationID() = %q, want conv_body", got)
	}
}
