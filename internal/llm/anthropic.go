package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/kowshik24/git-doc/internal/config"
)

type AnthropicClient struct {
	apiKey string
	model  string
	http   *http.Client
	url    string
}

func NewAnthropicClient(cfg *config.Config) *AnthropicClient {
	return &AnthropicClient{
		apiKey: cfg.LLM.APIKey,
		model:  cfg.LLM.Model,
		http: &http.Client{
			Timeout: time.Duration(cfg.LLM.Timeout) * time.Second,
		},
		url: "https://api.anthropic.com/v1/messages",
	}
}

func (a *AnthropicClient) Name() string {
	return "anthropic"
}

func (a *AnthropicClient) Generate(ctx context.Context, prompt string) (string, error) {
	requestBody := map[string]any{
		"model":      a.model,
		"max_tokens": 1024,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}

	b, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.url, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("x-api-key", a.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := a.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("anthropic request failed: %s", strings.TrimSpace(string(body)))
	}

	var parsed struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}

	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}

	for _, content := range parsed.Content {
		if content.Type == "text" && strings.TrimSpace(content.Text) != "" {
			return strings.TrimSpace(content.Text), nil
		}
	}

	return "", fmt.Errorf("anthropic response has no text content")
}
