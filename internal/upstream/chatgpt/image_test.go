package chatgpt

import (
	"context"
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
