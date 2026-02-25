package hooks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnableDisableWithBackupRestore(t *testing.T) {
	repo := t.TempDir()
	hooksDir := filepath.Join(repo, ".git", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}

	existing := filepath.Join(hooksDir, "post-commit")
	original := []byte("#!/bin/sh\necho original\n")
	if err := os.WriteFile(existing, original, 0o755); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(repo)
	if err := mgr.Enable(); err != nil {
		t.Fatalf("enable failed: %v", err)
	}

	enabledContent, err := os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	if string(enabledContent) != hookScript() {
		t.Fatalf("expected hook script to be installed")
	}

	backup := existing + ".git-doc.bak"
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("expected backup to exist: %v", err)
	}

	if err := mgr.Disable(); err != nil {
		t.Fatalf("disable failed: %v", err)
	}

	restored, err := os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != string(original) {
		t.Fatalf("expected original hook to be restored")
	}
}
