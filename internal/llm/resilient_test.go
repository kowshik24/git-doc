package llm

import (
	"context"
	"errors"
	"testing"
)

type flakyClient struct {
	name      string
	failCount int
	called    int
}

func (f *flakyClient) Name() string {
	return f.name
}

func (f *flakyClient) Generate(ctx context.Context, prompt string) (string, error) {
	_ = ctx
	_ = prompt
	f.called++
	if f.called <= f.failCount {
		return "", errors.New("transient failure")
	}
	return "ok", nil
}

func TestResilientClientRetriesThenSucceeds(t *testing.T) {
	primary := &flakyClient{name: "primary", failCount: 2}
	client := NewResilientClient([]Client{primary}, 3)

	out, err := client.Generate(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("expected success after retries, got err: %v", err)
	}
	if out != "ok" {
		t.Fatalf("expected ok output, got %q", out)
	}
	if primary.called != 3 {
		t.Fatalf("expected 3 calls, got %d", primary.called)
	}
}

func TestResilientClientFallsBack(t *testing.T) {
	primary := &flakyClient{name: "primary", failCount: 10}
	fallback := &flakyClient{name: "fallback", failCount: 0}
	client := NewResilientClient([]Client{primary, fallback}, 1)

	out, err := client.Generate(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("expected fallback success, got err: %v", err)
	}
	if out != "ok" {
		t.Fatalf("expected ok output, got %q", out)
	}
	if fallback.called == 0 {
		t.Fatalf("expected fallback provider to be called")
	}
}
