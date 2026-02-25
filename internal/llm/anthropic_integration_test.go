package llm

import (
	"context"
	"net/http"
	"testing"

	"github.com/kowshik24/git-doc/internal/config"
)

func TestAnthropicGenerate_Success(t *testing.T) {
	server := newJSONTestServer(t, http.StatusOK, `{"content":[{"type":"text","text":"  updated section content  "}]}`, func(t *testing.T, r *http.Request) {
		if r.Header.Get("x-api-key") == "" {
			t.Fatalf("expected x-api-key header to be set")
		}
	})
	defer server.Close()

	cfg := config.Default()
	cfg.LLM.Provider = "anthropic"
	cfg.LLM.APIKey = "test-key"
	cfg.LLM.Model = "claude-3-5-haiku-latest"

	client := NewAnthropicClient(cfg)
	client.url = server.URL

	out, err := client.Generate(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if out != "updated section content" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestAnthropicGenerate_HTTPError(t *testing.T) {
	server := newJSONTestServer(t, http.StatusTooManyRequests, `rate limited`, nil)
	defer server.Close()

	cfg := config.Default()
	cfg.LLM.Provider = "anthropic"
	cfg.LLM.APIKey = "test-key"

	client := NewAnthropicClient(cfg)
	client.url = server.URL

	_, err := client.Generate(context.Background(), "prompt")
	assertErrorContains(t, err, "anthropic request failed")
}
