package chatgpt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// HTTPTurnstileSolver calls an external HTTP service to solve Turnstile challenges.
// The service should accept POST /solve with {"dx": "...", "p": "..."} and return
// {"token": "..."}.
type HTTPTurnstileSolver struct {
	url    string
	client *http.Client
}

// NewHTTPTurnstileSolver creates a solver that delegates to the given URL.
func NewHTTPTurnstileSolver(url string) *HTTPTurnstileSolver {
	return &HTTPTurnstileSolver{url: url, client: &http.Client{}}
}

func (s *HTTPTurnstileSolver) Solve(ctx context.Context, dx string, token string) (string, error) {
	body, _ := json.Marshal(map[string]string{"dx": dx, "p": token})
	req, err := http.NewRequestWithContext(ctx, "POST", s.url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	buf, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("turnstile solver HTTP %d: %s", resp.StatusCode, string(buf))
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(buf, &result); err != nil {
		return "", fmt.Errorf("decode solver response: %w", err)
	}
	return result.Token, nil
}
