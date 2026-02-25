package llm

import (
	"context"
	"net/http"
	"testing"

	"github.com/kowshik/git-doc/internal/config"
)

func TestGeminiGenerate_Success(t *testing.T) {
	server := newJSONTestServer(t, http.StatusOK, `{"candidates":[{"content":{"parts":[{"text":"  gemini output  "}]}}]}`, nil)
	defer server.Close()

	cfg := config.Default()
	cfg.LLM.Provider = "gemini"
	cfg.LLM.APIKey = "test-key"
	cfg.LLM.Model = "gemini-1.5-flash"

	client := NewGeminiClient(cfg)
	client.base = server.URL

	out, err := client.Generate(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if out != "gemini output" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestGeminiGenerate_HTTPError(t *testing.T) {
	server := newJSONTestServer(t, http.StatusBadRequest, `invalid request`, nil)
	defer server.Close()

	cfg := config.Default()
	cfg.LLM.Provider = "gemini"
	cfg.LLM.APIKey = "test-key"

	client := NewGeminiClient(cfg)
	client.base = server.URL

	_, err := client.Generate(context.Background(), "prompt")
	assertErrorContains(t, err, "gemini request failed")
}
