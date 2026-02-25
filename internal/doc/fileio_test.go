package doc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectLineEnding(t *testing.T) {
	if got := DetectLineEnding("a\r\nb\r\n"); got != "\r\n" {
		t.Fatalf("expected CRLF, got %q", got)
	}
	if got := DetectLineEnding("a\nb\n"); got != "\n" {
		t.Fatalf("expected LF, got %q", got)
	}
}

func TestNormalizeLineEndings(t *testing.T) {
	input := "a\nb\n"
	if out := NormalizeLineEndings(input, "\r\n"); out != "a\r\nb\r\n" {
		t.Fatalf("expected CRLF normalized output, got %q", out)
	}

	input2 := "a\r\nb\r\n"
	if out := NormalizeLineEndings(input2, "\n"); out != "a\nb\n" {
		t.Fatalf("expected LF normalized output, got %q", out)
	}
}

func TestAtomicWriteFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "README.md")

	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := AtomicWriteFile(target, []byte("new-content"), 0o644); err != nil {
		t.Fatalf("atomic write failed: %v", err)
	}

	b, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "new-content" {
		t.Fatalf("unexpected file content after atomic write: %q", string(b))
	}
}
