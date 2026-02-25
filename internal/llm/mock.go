package llm

import (
	"context"
	"strings"
)

type MockClient struct{}

func NewMockClient() *MockClient {
	return &MockClient{}
}

func (m *MockClient) Name() string {
	return "mock"
}

func (m *MockClient) Generate(ctx context.Context, prompt string) (string, error) {
	_ = ctx
	line := strings.TrimSpace(prompt)
	if line == "" {
		return "No changes detected.", nil
	}

	if len(line) > 180 {
		line = line[:180]
	}

	return "- Auto-generated update\n\n" + line, nil
}
