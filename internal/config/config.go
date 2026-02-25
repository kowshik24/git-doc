package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	LLM      LLMConfig      `toml:"llm"`
	DocFiles []string       `toml:"doc_files"`
	Mappings []Mapping      `toml:"mappings"`
	Git      GitConfig      `toml:"git"`
	State    StateConfig    `toml:"state"`
	Runtime  RuntimeOptions `toml:"runtime"`
}

type LLMConfig struct {
	Provider          string   `toml:"provider"`
	APIKey            string   `toml:"api_key"`
	Model             string   `toml:"model"`
	Timeout           int      `toml:"timeout"`
	MaxRetries        int      `toml:"max_retries"`
	FailoverEnabled   bool     `toml:"failover_enabled"`
	FallbackProviders []string `toml:"fallback_providers"`
}

type Mapping struct {
	CodePattern string `toml:"code_pattern"`
	DocFile     string `toml:"doc_file"`
	Section     string `toml:"section"`
}

type GitConfig struct {
	CommitDocUpdates bool   `toml:"commit_doc_updates"`
	AmendOriginal    bool   `toml:"amend_original"`
	DocCommitMessage string `toml:"doc_commit_message"`
}

type StateConfig struct {
	DBPath string `toml:"db_path"`
}

type RuntimeOptions struct {
	DefaultSection string `toml:"default_section"`
}

func Load(path string) (*Config, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("config file %s not found: %w", path, err)
	}

	cfg := Default()
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg.expandEnv()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func Default() *Config {
	return &Config{
		LLM: LLMConfig{
			Provider:        "mock",
			Model:           "gpt-4o-mini",
			Timeout:         60,
			MaxRetries:      3,
			FailoverEnabled: true,
		},
		DocFiles: []string{"README.md", "docs/**/*.md"},
		Git: GitConfig{
			CommitDocUpdates: true,
			DocCommitMessage: "docs: auto-update for {hash}",
		},
		State:   StateConfig{DBPath: ".git-doc/state.db"},
		Runtime: RuntimeOptions{DefaultSection: "Recent Changes"},
	}
}

func DefaultToml() string {
	return `# LLM settings
[llm]
provider = "mock"
api_key = "${GITDOC_OPENAI_KEY}"
model = "gpt-4o-mini"
timeout = 60
max_retries = 3
failover_enabled = true
fallback_providers = []

doc_files = ["README.md", "docs/**/*.md"]

[git]
commit_doc_updates = true
amend_original = false
doc_commit_message = "docs: auto-update for {hash}"

[state]
db_path = ".git-doc/state.db"

[runtime]
default_section = "Recent Changes"
`
}

func (c *Config) Validate() error {
	if strings.TrimSpace(c.LLM.Provider) == "" {
		return errors.New("llm.provider is required")
	}

	provider := strings.ToLower(strings.TrimSpace(c.LLM.Provider))
	supported := map[string]bool{
		"mock":      true,
		"openai":    true,
		"anthropic": true,
		"google":    true,
		"gemini":    true,
		"groq":      true,
		"ollama":    true,
	}

	if !supported[provider] {
		return fmt.Errorf("unsupported llm.provider: %s", c.LLM.Provider)
	}

	for _, fallback := range c.LLM.FallbackProviders {
		fallbackProvider := strings.ToLower(strings.TrimSpace(fallback))
		if fallbackProvider == "" {
			continue
		}
		if !supported[fallbackProvider] {
			return fmt.Errorf("unsupported llm.fallback_provider: %s", fallback)
		}
	}

	if (provider == "openai" || provider == "anthropic" || provider == "google" || provider == "gemini" || provider == "groq") && strings.TrimSpace(c.LLM.APIKey) == "" {
		return fmt.Errorf("llm.api_key is required for %s provider", provider)
	}

	if strings.TrimSpace(c.State.DBPath) == "" {
		return errors.New("state.db_path is required")
	}

	if strings.TrimSpace(c.Runtime.DefaultSection) == "" {
		c.Runtime.DefaultSection = "Recent Changes"
	}

	if c.LLM.Timeout <= 0 {
		c.LLM.Timeout = 60
	}

	if c.LLM.MaxRetries <= 0 {
		c.LLM.MaxRetries = 3
	}

	return nil
}

func (c *Config) expandEnv() {
	c.LLM.APIKey = os.ExpandEnv(c.LLM.APIKey)
	c.State.DBPath = os.ExpandEnv(c.State.DBPath)

	for i := range c.DocFiles {
		c.DocFiles[i] = os.ExpandEnv(c.DocFiles[i])
	}

	for i := range c.Mappings {
		c.Mappings[i].CodePattern = os.ExpandEnv(c.Mappings[i].CodePattern)
		c.Mappings[i].DocFile = os.ExpandEnv(c.Mappings[i].DocFile)
		c.Mappings[i].Section = os.ExpandEnv(c.Mappings[i].Section)
	}
}
