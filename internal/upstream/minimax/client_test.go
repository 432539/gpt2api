package minimax_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/432539/gpt2api/internal/upstream/chatgpt"
	"github.com/432539/gpt2api/internal/upstream/minimax"
)

// roundTripFunc is an http.RoundTripper implemented as a function for test mocking.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestNew verifies that New returns an error on missing api_key.
func TestNew_MissingAPIKey(t *testing.T) {
	_, err := minimax.New(minimax.Options{})
	if err == nil {
		t.Fatal("expected error for empty api_key, got nil")
	}
}

// TestNew_OK checks that New succeeds with a valid api_key.
func TestNew_OK(t *testing.T) {
	cli, err := minimax.New(minimax.Options{APIKey: "test-key"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cli == nil {
		t.Fatal("expected non-nil client")
	}
}

// TestChat_NonStream verifies non-streaming response parsing.
func TestChat_NonStream(t *testing.T) {
	const wantContent = "Hello from MiniMax!"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header.
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": wantContent,
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     10,
				"completion_tokens": 5,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cli, err := minimax.New(minimax.Options{
		APIKey:  "test-key",
		BaseURL: srv.URL,
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	msgs := []chatgpt.ChatMessage{{Role: "user", Content: "hi"}}
	content, promptTok, completionTok, err := cli.Chat(context.Background(), "MiniMax-M2.7", msgs, 0)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if content != wantContent {
		t.Errorf("content = %q, want %q", content, wantContent)
	}
	if promptTok != 10 {
		t.Errorf("promptTok = %d, want 10", promptTok)
	}
	if completionTok != 5 {
		t.Errorf("completionTok = %d, want 5", completionTok)
	}
}

// TestChat_UpstreamError verifies error propagation on non-200 responses.
func TestChat_UpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"rate limited"}`, http.StatusTooManyRequests)
	}))
	defer srv.Close()

	cli, _ := minimax.New(minimax.Options{APIKey: "k", BaseURL: srv.URL})
	_, _, _, err := cli.Chat(context.Background(), "MiniMax-M2.7", nil, 0)
	if err == nil {
		t.Fatal("expected error on 429, got nil")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("error should mention 429, got: %v", err)
	}
}

// TestStreamChat_Basic verifies streaming SSE response parsing.
func TestStreamChat_Basic(t *testing.T) {
	chunks := []string{
		`{"choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}`,
		`{"choices":[{"delta":{"content":" world"},"finish_reason":null}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fl := w.(http.Flusher)
		for _, chunk := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			fl.Flush()
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
		fl.Flush()
	}))
	defer srv.Close()

	cli, _ := minimax.New(minimax.Options{APIKey: "k", BaseURL: srv.URL})
	stream, err := cli.StreamChat(context.Background(), "MiniMax-M2.7",
		[]chatgpt.ChatMessage{{Role: "user", Content: "hi"}}, 0)
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}

	var got strings.Builder
	for ev := range stream {
		if ev.Err != nil {
			t.Fatalf("stream error: %v", ev.Err)
		}
		// ev.Data is raw JSON, extract content
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(ev.Data, &chunk); err == nil && len(chunk.Choices) > 0 {
			got.WriteString(chunk.Choices[0].Delta.Content)
		}
	}

	if got.String() != "Hello world" {
		t.Errorf("streamed content = %q, want %q", got.String(), "Hello world")
	}
}
