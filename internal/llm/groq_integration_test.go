package llm

import (
	"context"
	"net/http"
	"testing"

	"github.com/kowshik24/git-doc/internal/config"
)

func TestGroqGenerate_Success(t *testing.T) {
	server := newJSONTestServer(t, http.StatusOK, `{"choices":[{"message":{"content":"  groq output  "}}]}`, func(t *testing.T, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Fatalf("expected Authorization header to be set")
		}
	})
	defer server.Close()

	cfg := config.Default()
	cfg.LLM.Provider = "groq"
	cfg.LLM.APIKey = "test-key"
	cfg.LLM.Model = "llama-3.1-8b-instant"

	client := NewGroqClient(cfg)
	client.url = server.URL

	out, err := client.Generate(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if out != "groq output" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestGroqGenerate_HTTPError(t *testing.T) {
	server := newJSONTestServer(t, http.StatusTooManyRequests, `rate limited`, nil)
	defer server.Close()

	cfg := config.Default()
	cfg.LLM.Provider = "groq"
	cfg.LLM.APIKey = "test-key"

	client := NewGroqClient(cfg)
	client.url = server.URL

	_, err := client.Generate(context.Background(), "prompt")
	assertErrorContains(t, err, "groq request failed")
}
