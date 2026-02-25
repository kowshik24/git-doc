package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kowshik/git-doc/internal/config"
	"github.com/kowshik/git-doc/internal/runlock"
)

func TestUpdateFromHookNoOpWhenLockHeld(t *testing.T) {
	repo := t.TempDir()
	initGitRepo(t, repo)
	writeDefaultConfig(t, repo)

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(originalWD)

	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}

	lock, err := runlock.Acquire(repo)
	if err != nil {
		t.Fatalf("failed to acquire lock for test setup: %v", err)
	}
	defer lock.Release()

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"update", "--from-hook"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected update --from-hook to no-op successfully when locked, got: %v", err)
	}
}

func initGitRepo(t *testing.T, repo string) {
	t.Helper()
	cmd := exec.Command("git", "init")
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git init failed: %v (%s)", err, string(out))
	}
}

func writeDefaultConfig(t *testing.T, repo string) {
	t.Helper()
	gitDocDir := filepath.Join(repo, ".git-doc")
	if err := os.MkdirAll(gitDocDir, 0o700); err != nil {
		t.Fatalf("create .git-doc dir: %v", err)
	}
	configPath := filepath.Join(gitDocDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(config.DefaultToml()), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}
