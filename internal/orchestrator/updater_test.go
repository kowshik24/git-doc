package orchestrator

import (
	"testing"

	"github.com/kowshik24/git-doc/internal/config"
)

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

func TestMatchCodePattern_Globs(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		want    bool
	}{
		{
			name:    "double-star recursive match",
			pattern: "src/api/**/*.py",
			path:    "src/api/v1/user/handler.py",
			want:    true,
		},
		{
			name:    "single-segment star match",
			pattern: "internal/*/updater.go",
			path:    "internal/orchestrator/updater.go",
			want:    true,
		},
		{
			name:    "no match",
			pattern: "src/models/*.go",
			path:    "src/models/user.py",
			want:    false,
		},
		{
			name:    "double-star can match zero segments",
			pattern: "docs/**/README.md",
			path:    "docs/README.md",
			want:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := matchCodePattern(tc.pattern, tc.path)
			if got != tc.want {
				t.Fatalf("matchCodePattern(%q, %q) = %v, want %v", tc.pattern, tc.path, got, tc.want)
			}
		})
	}
}

func TestResolveTarget_UsesFirstMatchingMapping(t *testing.T) {
	u := &Updater{
		deps: Dependencies{
			Config: &config.Config{
				DocFiles: []string{"README.md"},
				Mappings: []config.Mapping{
					{
						CodePattern: "src/api/**/*.py",
						DocFile:     "docs/api.md",
						Section:     "API Reference",
					},
				},
				Runtime: config.RuntimeOptions{DefaultSection: "Recent Changes"},
			},
		},
	}

	docFile, section := u.resolveTarget([]string{"src/api/v2/payments/client.py"})
	if docFile != "docs/api.md" || section != "API Reference" {
		t.Fatalf("resolveTarget() = (%q, %q), want (%q, %q)", docFile, section, "docs/api.md", "API Reference")
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
