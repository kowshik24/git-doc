package llm

import (
	"testing"

	"github.com/kowshik/git-doc/internal/config"
)

func TestNewClientSupportsAdditionalProviders(t *testing.T) {
	providers := []string{"anthropic", "gemini", "google", "groq", "ollama"}
	for _, provider := range providers {
		cfg := config.Default()
		cfg.LLM.Provider = provider
		cfg.LLM.APIKey = "test-key"

		client, err := NewClient(cfg)
		if err != nil {
			t.Fatalf("expected provider %s to be supported, got error: %v", provider, err)
		}
		if client == nil {
			t.Fatalf("expected non-nil client for provider %s", provider)
		}
	}
}
