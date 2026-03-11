package gitutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseUnix(t *testing.T) {
	ts, err := parseUnix("1710000000")
	if err != nil {
		t.Fatalf("parseUnix returned error: %v", err)
	}

	if ts.Unix() != 1710000000 {
		t.Fatalf("unexpected unix timestamp: got %d", ts.Unix())
	}
}

func TestParseUnixInvalid(t *testing.T) {
	if _, err := parseUnix("not-a-number"); err == nil {
		t.Fatalf("expected parseUnix to fail for invalid input")
	}
}

func TestCLIHelperCommitLifecycle(t *testing.T) {
	repo := initTestRepo(t)
	h := NewHelper(repo)

	filePath := filepath.Join(repo, "a.txt")
	if err := os.WriteFile(filePath, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	firstHash, err := h.StageAndCommit([]string{"a.txt"}, "feat: add a")
	if err != nil {
		t.Fatalf("StageAndCommit first commit failed: %v", err)
	}
	if strings.TrimSpace(firstHash) == "" {
		t.Fatalf("expected first commit hash")
	}

	head, err := h.GetCurrentHEAD()
	if err != nil {
		t.Fatalf("GetCurrentHEAD failed: %v", err)
	}
	if head != firstHash {
		t.Fatalf("head mismatch: got %s want %s", head, firstHash)
	}

	msg, err := h.GetCommitMessage(firstHash)
	if err != nil {
		t.Fatalf("GetCommitMessage failed: %v", err)
	}
	if msg != "feat: add a" {
		t.Fatalf("unexpected commit message: %q", msg)
	}

	files, err := h.GetChangedFiles(firstHash)
	if err != nil {
		t.Fatalf("GetChangedFiles failed: %v", err)
	}
	if len(files) != 1 || files[0] != "a.txt" {
		t.Fatalf("unexpected changed files: %#v", files)
	}

	diff, err := h.GetCommitDiff(firstHash)
	if err != nil {
		t.Fatalf("GetCommitDiff failed: %v", err)
	}
	if !strings.Contains(diff, "+hello") {
		t.Fatalf("expected diff to include inserted content")
	}

	commits, err := h.GetLastProcessedRange("", firstHash)
	if err != nil {
		t.Fatalf("GetLastProcessedRange full failed: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits (seed + feature), got %d", len(commits))
	}
	if commits[1].Hash != firstHash || commits[1].Subject != "feat: add a" {
		t.Fatalf("unexpected commit info: %#v", commits[1])
	}
	if commits[1].Timestamp.IsZero() || commits[1].Timestamp.After(time.Now().Add(5*time.Minute)) {
		t.Fatalf("unexpected commit timestamp: %v", commits[1].Timestamp)
	}

	if err := os.WriteFile(filePath, []byte("hello amended\n"), 0o644); err != nil {
		t.Fatalf("rewrite file: %v", err)
	}

	amendedHash, err := h.StageAndAmend([]string{"a.txt"})
	if err != nil {
		t.Fatalf("StageAndAmend failed: %v", err)
	}
	if strings.TrimSpace(amendedHash) == "" || amendedHash == firstHash {
		t.Fatalf("expected amended commit hash to differ from first hash")
	}

	if err := os.WriteFile(filePath, []byte("hello second\n"), 0o644); err != nil {
		t.Fatalf("rewrite file second: %v", err)
	}

	secondHash, err := h.StageAndCommit([]string{"a.txt"}, "feat: second")
	if err != nil {
		t.Fatalf("StageAndCommit second commit failed: %v", err)
	}

	rangeCommits, err := h.GetLastProcessedRange(amendedHash, secondHash)
	if err != nil {
		t.Fatalf("GetLastProcessedRange between commits failed: %v", err)
	}
	if len(rangeCommits) != 1 || rangeCommits[0].Hash != secondHash {
		t.Fatalf("unexpected range commits: %#v", rangeCommits)
	}

	if err := h.RevertCommit(secondHash); err != nil {
		t.Fatalf("RevertCommit failed: %v", err)
	}

	revertMsg, err := h.GetCommitMessage("HEAD")
	if err != nil {
		t.Fatalf("GetCommitMessage(HEAD) failed: %v", err)
	}
	if !strings.Contains(revertMsg, "Revert") {
		t.Fatalf("expected revert commit message, got %q", revertMsg)
	}
}

func initTestRepo(t *testing.T) string {
	t.Helper()

	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.name", "git-doc test")
	runGit(t, repo, "config", "user.email", "git-doc-test@example.com")
	if err := os.WriteFile(filepath.Join(repo, ".seed"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
	runGit(t, repo, "add", ".seed")
	runGit(t, repo, "commit", "-m", "chore: seed")
	return repo
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out)
}
