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

	"github.com/kowshik/git-doc/internal/config"
)

type OllamaClient struct {
	model string
	http  *http.Client
	url   string
}

func NewOllamaClient(cfg *config.Config) *OllamaClient {
	return &OllamaClient{
		model: cfg.LLM.Model,
		http: &http.Client{
			Timeout: time.Duration(cfg.LLM.Timeout) * time.Second,
		},
		url: "http://localhost:11434/api/generate",
	}
}

func (o *OllamaClient) Name() string {
	return "ollama"
}

func (o *OllamaClient) Generate(ctx context.Context, prompt string) (string, error) {
	requestBody := map[string]any{
		"model":  o.model,
		"prompt": prompt,
		"stream": false,
	}

	b, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.url, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("content-type", "application/json")

	resp, err := o.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("ollama request failed: %s", strings.TrimSpace(string(body)))
	}

	var parsed struct {
		Response string `json:"response"`
	}

	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}

	if strings.TrimSpace(parsed.Response) == "" {
		return "", fmt.Errorf("ollama response is empty")
	}

	return strings.TrimSpace(parsed.Response), nil
}
