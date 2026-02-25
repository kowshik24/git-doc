package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kowshik24/git-doc/internal/config"
)

type GeminiClient struct {
	apiKey string
	model  string
	http   *http.Client
	base   string
}

func NewGeminiClient(cfg *config.Config) *GeminiClient {
	return &GeminiClient{
		apiKey: cfg.LLM.APIKey,
		model:  cfg.LLM.Model,
		http: &http.Client{
			Timeout: time.Duration(cfg.LLM.Timeout) * time.Second,
		},
		base: "https://generativelanguage.googleapis.com/v1beta/models",
	}
}

func (g *GeminiClient) Name() string {
	return "gemini"
}

func (g *GeminiClient) Generate(ctx context.Context, prompt string) (string, error) {
	requestBody := map[string]any{
		"contents": []map[string]any{
			{
				"parts": []map[string]string{{"text": prompt}},
			},
		},
	}

	b, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}

	endpoint := fmt.Sprintf("%s/%s:generateContent?key=%s", g.base, g.model, url.QueryEscape(g.apiKey))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
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
		return "", fmt.Errorf("gemini request failed: %s", strings.TrimSpace(string(body)))
	}

	var parsed struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}

	for _, candidate := range parsed.Candidates {
		for _, part := range candidate.Content.Parts {
			if strings.TrimSpace(part.Text) != "" {
				return strings.TrimSpace(part.Text), nil
			}
		}
	}

	return "", fmt.Errorf("gemini response has no text content")
}
