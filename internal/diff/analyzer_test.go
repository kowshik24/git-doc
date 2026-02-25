package diff

import "testing"

func TestParseUnifiedDiff(t *testing.T) {
	raw := "diff --git a/a.go b/a.go\nindex 1..2 100644\n--- a/a.go\n+++ b/a.go\n@@ -1,2 +1,3 @@\n line1\n-line2\n+line2changed\n+line3\n"

	parsed, err := ParseUnifiedDiff(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if len(parsed.Files) != 1 {
		t.Fatalf("expected 1 file diff, got %d", len(parsed.Files))
	}

	file := parsed.Files[0]
	if file.Path != "a.go" {
		t.Fatalf("expected path a.go, got %q", file.Path)
	}
	if file.AddedLines != 2 || file.DelLines != 1 {
		t.Fatalf("unexpected line stats: +%d -%d", file.AddedLines, file.DelLines)
	}
	if len(file.Hunks) != 1 {
		t.Fatalf("expected one hunk, got %d", len(file.Hunks))
	}
}

func TestBuildSummaryAndTruncate(t *testing.T) {
	d := Diff{Files: []FileDiff{{Path: "a.go", AddedLines: 3, DelLines: 1, Hunks: []Hunk{{}}}}}
	summary := BuildSummary(d)
	if summary == "" {
		t.Fatalf("expected non-empty summary")
	}

	truncated := TruncateText(summary, 10)
	if len(truncated) != 10 {
		t.Fatalf("expected truncated length 10, got %d", len(truncated))
	}
}
