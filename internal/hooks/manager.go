package hooks

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var supportedHooks = []string{"post-commit", "post-merge", "post-rewrite"}

type Manager struct {
	repoRoot string
}

func NewManager(repoRoot string) *Manager {
	return &Manager{repoRoot: repoRoot}
}

func (m *Manager) Enable() error {
	hooksDir := filepath.Join(m.repoRoot, ".git", "hooks")
	if _, err := os.Stat(hooksDir); err != nil {
		return fmt.Errorf("git hooks directory not found: %w", err)
	}

	for _, hook := range supportedHooks {
		hookPath := filepath.Join(hooksDir, hook)
		if err := m.backupHookIfNeeded(hookPath); err != nil {
			return err
		}

		if err := os.WriteFile(hookPath, []byte(hookScript()), 0o755); err != nil {
			return fmt.Errorf("write hook %s: %w", hook, err)
		}
	}

	return nil
}

func (m *Manager) Disable() error {
	hooksDir := filepath.Join(m.repoRoot, ".git", "hooks")
	if _, err := os.Stat(hooksDir); err != nil {
		return fmt.Errorf("git hooks directory not found: %w", err)
	}

	for _, hook := range supportedHooks {
		hookPath := filepath.Join(hooksDir, hook)
		backupPath := m.backupPath(hookPath)

		if _, err := os.Stat(backupPath); err == nil {
			if rmErr := os.Remove(hookPath); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
				return fmt.Errorf("remove hook %s: %w", hook, rmErr)
			}
			if err := os.Rename(backupPath, hookPath); err != nil {
				return fmt.Errorf("restore hook backup %s: %w", hook, err)
			}
			continue
		}

		content, err := os.ReadFile(hookPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("read hook %s: %w", hook, err)
		}

		if strings.Contains(string(content), "git-doc update") {
			if err := os.Remove(hookPath); err != nil {
				return fmt.Errorf("remove hook %s: %w", hook, err)
			}
		}
	}

	return nil
}

func (m *Manager) backupHookIfNeeded(hookPath string) error {
	content, err := os.ReadFile(hookPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read existing hook: %w", err)
	}

	if strings.Contains(string(content), "git-doc update") {
		return nil
	}

	backupPath := m.backupPath(hookPath)
	if _, err := os.Stat(backupPath); err == nil {
		return nil
	}

	if err := os.WriteFile(backupPath, content, 0o755); err != nil {
		return fmt.Errorf("backup existing hook: %w", err)
	}

	return nil
}

func (m *Manager) backupPath(hookPath string) string {
	return hookPath + ".git-doc.bak"
}

func hookScript() string {
	return "#!/bin/sh\ngit-doc update --from-hook > /dev/null 2>&1 &\n"
}
