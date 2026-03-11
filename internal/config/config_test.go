package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigWithEnvExpansion(t *testing.T) {
	t.Setenv("GITDOC_TEST_KEY", "abc123")

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	content := `
[llm]
provider = "openai"
api_key = "${GITDOC_TEST_KEY}"
model = "gpt-4o-mini"
timeout = 30
max_retries = 2

[state]
db_path = ".git-doc/state.db"

[runtime]
default_section = "Recent Changes"
`

	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if cfg.LLM.APIKey != "abc123" {
		t.Fatalf("expected API key to expand env variable")
	}
}

func TestLoadConfigWithInvalidFallbackProvider(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	content := `
[llm]
provider = "mock"
model = "gpt-4o-mini"
timeout = 30
max_retries = 2
failover_enabled = true
fallback_providers = ["unknown-provider"]

[state]
db_path = ".git-doc/state.db"

[runtime]
default_section = "Recent Changes"
`

	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(configPath); err == nil {
		t.Fatalf("expected load to fail for invalid fallback provider")
	}
}

func TestDefaultTomlAllowsTopLevelDocFilesOverride(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	content := strings.Replace(
		DefaultToml(),
		`doc_files = ["README.md", "docs/**/*.md"]`,
		`doc_files = ["GUIDE.md"]`,
		1,
	)

	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if len(cfg.DocFiles) != 1 || cfg.DocFiles[0] != "GUIDE.md" {
		t.Fatalf("expected top-level doc_files override to be loaded, got %#v", cfg.DocFiles)
	}
}
