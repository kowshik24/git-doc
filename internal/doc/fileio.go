package doc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func DetectLineEnding(content string) string {
	if strings.Contains(content, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

func NormalizeLineEndings(content, lineEnding string) string {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	if lineEnding == "\r\n" {
		return strings.ReplaceAll(normalized, "\n", "\r\n")
	}
	return normalized
}

func AtomicWriteFile(path string, content []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".git-doc-tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	cleanup := func() {
		_ = os.Remove(tmpPath)
	}

	if _, err := tmp.Write(content); err != nil {
		if closeErr := tmp.Close(); closeErr != nil {
			cleanup()
			return fmt.Errorf("close temp file after write failure: %w", closeErr)
		}
		cleanup()
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := tmp.Chmod(perm); err != nil {
		if closeErr := tmp.Close(); closeErr != nil {
			cleanup()
			return fmt.Errorf("close temp file after chmod failure: %w", closeErr)
		}
		cleanup()
		return fmt.Errorf("chmod temp file: %w", err)
	}

	if err := tmp.Sync(); err != nil {
		if closeErr := tmp.Close(); closeErr != nil {
			cleanup()
			return fmt.Errorf("close temp file after sync failure: %w", closeErr)
		}
		cleanup()
		return fmt.Errorf("sync temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return fmt.Errorf("atomic rename: %w", err)
	}

	return nil
}
