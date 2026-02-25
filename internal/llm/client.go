package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/kowshik24/git-doc/internal/config"
)

type Client interface {
	Name() string
	Generate(ctx context.Context, prompt string) (string, error)
}

func NewClient(cfg *config.Config) (Client, error) {
	primary := strings.ToLower(strings.TrimSpace(cfg.LLM.Provider))
	if primary == "" {
		primary = "mock"
	}

	providers := []string{primary}
	if cfg.LLM.FailoverEnabled {
		for _, fallback := range cfg.LLM.FallbackProviders {
			fallbackProvider := strings.ToLower(strings.TrimSpace(fallback))
			if fallbackProvider == "" || containsProvider(providers, fallbackProvider) {
				continue
			}
			providers = append(providers, fallbackProvider)
		}
	}

	clients := make([]Client, 0, len(providers))
	for _, provider := range providers {
		client, err := buildProviderClient(provider, cfg)
		if err != nil {
			return nil, err
		}
		clients = append(clients, client)
	}

	if len(clients) == 1 && cfg.LLM.MaxRetries <= 0 {
		return clients[0], nil
	}

	return NewResilientClient(clients, cfg.LLM.MaxRetries), nil
}

func buildProviderClient(provider string, cfg *config.Config) (Client, error) {
	switch provider {
	case "mock":
		return NewMockClient(), nil
	case "openai":
		return NewOpenAIClient(cfg), nil
	case "anthropic":
		return NewAnthropicClient(cfg), nil
	case "google", "gemini":
		return NewGeminiClient(cfg), nil
	case "groq":
		return NewGroqClient(cfg), nil
	case "ollama":
		return NewOllamaClient(cfg), nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

func containsProvider(providers []string, candidate string) bool {
	for _, provider := range providers {
		if provider == candidate {
			return true
		}
	}
	return false
}
