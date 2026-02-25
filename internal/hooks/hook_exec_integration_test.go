package hooks

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInstalledHookInvokesGitDocWithFromHookFlag(t *testing.T) {
	repo := t.TempDir()
	initGitRepo(t, repo)

	mgr := NewManager(repo)
	if err := mgr.Enable(); err != nil {
		t.Fatalf("enable hooks failed: %v", err)
	}

	binDir := filepath.Join(repo, "test-bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}

	logPath := filepath.Join(repo, "git-doc-invocation.log")
	fakeGitDocPath := filepath.Join(binDir, "git-doc")
	fakeScript := "#!/bin/sh\necho \"$@\" >> \"$GIT_DOC_TEST_LOG\"\n"
	if err := os.WriteFile(fakeGitDocPath, []byte(fakeScript), 0o755); err != nil {
		t.Fatal(err)
	}

	hookPath := filepath.Join(repo, ".git", "hooks", "post-commit")
	cmd := exec.Command("sh", hookPath)
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GIT_DOC_TEST_LOG="+logPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("running hook failed: %v (%s)", err, string(out))
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		if time.Now().After(deadline) {
			break
		}
		if _, err := os.Stat(logPath); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("expected invocation log to be created: %v", err)
	}

	line := strings.TrimSpace(string(b))
	if !strings.Contains(line, "update") || !strings.Contains(line, "--from-hook") {
		t.Fatalf("expected hook to invoke 'git-doc update --from-hook', got: %q", line)
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
