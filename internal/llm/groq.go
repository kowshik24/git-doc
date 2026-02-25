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

type GroqClient struct {
	apiKey string
	model  string
	http   *http.Client
	url    string
}

func NewGroqClient(cfg *config.Config) *GroqClient {
	return &GroqClient{
		apiKey: cfg.LLM.APIKey,
		model:  cfg.LLM.Model,
		http: &http.Client{
			Timeout: time.Duration(cfg.LLM.Timeout) * time.Second,
		},
		url: "https://api.groq.com/openai/v1/chat/completions",
	}
}

func (g *GroqClient) Name() string {
	return "groq"
}

func (g *GroqClient) Generate(ctx context.Context, prompt string) (string, error) {
	requestBody := map[string]any{
		"model": g.model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}

	b, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.url, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+g.apiKey)
	req.Header.Set("content-type", "application/json")

	resp, err := g.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("groq request failed: %s", strings.TrimSpace(string(body)))
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}

	if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("groq response has no choices")
	}

	return strings.TrimSpace(parsed.Choices[0].Message.Content), nil
}
