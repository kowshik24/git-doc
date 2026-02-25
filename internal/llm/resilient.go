package llm

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type ResilientClient struct {
	clients    []Client
	maxRetries int
}

func NewResilientClient(clients []Client, maxRetries int) *ResilientClient {
	if maxRetries < 0 {
		maxRetries = 0
	}
	return &ResilientClient{clients: clients, maxRetries: maxRetries}
}

func (c *ResilientClient) Name() string {
	names := make([]string, 0, len(c.clients))
	for _, client := range c.clients {
		names = append(names, client.Name())
	}
	return "resilient(" + strings.Join(names, "->") + ")"
}

func (c *ResilientClient) Generate(ctx context.Context, prompt string) (string, error) {
	if len(c.clients) == 0 {
		return "", fmt.Errorf("no llm clients configured")
	}

	var lastErr error
	for _, provider := range c.clients {
		for attempt := 0; attempt <= c.maxRetries; attempt++ {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}

			result, err := provider.Generate(ctx, prompt)
			if err == nil {
				return result, nil
			}
			lastErr = fmt.Errorf("provider %s attempt %d failed: %w", provider.Name(), attempt+1, err)

			if attempt < c.maxRetries {
				delay := time.Duration(1<<attempt) * 150 * time.Millisecond
				select {
				case <-ctx.Done():
					return "", ctx.Err()
				case <-time.After(delay):
				}
			}
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("all llm providers failed")
	}
	return "", lastErr
}
