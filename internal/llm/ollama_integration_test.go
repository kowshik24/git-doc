package llm

import (
	"context"
	"net/http"
	"testing"

	"github.com/kowshik24/git-doc/internal/config"
)

func TestOllamaGenerate_Success(t *testing.T) {
	server := newJSONTestServer(t, http.StatusOK, `{"response":"  ollama output  "}`, nil)
	defer server.Close()

	cfg := config.Default()
	cfg.LLM.Provider = "ollama"
	cfg.LLM.Model = "llama3"

	client := NewOllamaClient(cfg)
	client.url = server.URL

	out, err := client.Generate(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if out != "ollama output" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestOllamaGenerate_HTTPError(t *testing.T) {
	server := newJSONTestServer(t, http.StatusInternalServerError, `server unavailable`, nil)
	defer server.Close()

	cfg := config.Default()
	cfg.LLM.Provider = "ollama"

	client := NewOllamaClient(cfg)
	client.url = server.URL

	_, err := client.Generate(context.Background(), "prompt")
	assertErrorContains(t, err, "ollama request failed")
}
