package orchestrator

import "testing"

func TestBuildPromptUsesDiffSummaryWhenParseable(t *testing.T) {
	diff := "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1,1 +1,2 @@\n-line1\n+line1\n+line2\n"
	prompt := buildPrompt("feat: update", diff)

	if !contains(prompt, "Files changed:") {
		t.Fatalf("expected prompt to include parsed diff summary, got: %s", prompt)
	}
}

func TestBuildPromptFallsBackToRawDiff(t *testing.T) {
	diff := "this-is-not-a-unified-diff"
	prompt := buildPrompt("feat: update", diff)

	if !contains(prompt, diff) {
		t.Fatalf("expected prompt to include raw diff fallback")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
