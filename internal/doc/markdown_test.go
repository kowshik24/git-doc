package doc

import "testing"

func TestReplaceSectionExisting(t *testing.T) {
	u := NewMarkdownUpdater()
	input := "# Title\n\n## Recent Changes\nold\n\n## Next\nnext"
	out, err := u.ReplaceSection(input, "Recent Changes", "new content")
	if err != nil {
		t.Fatal(err)
	}

	expectedContains := "## Recent Changes\nnew content"
	if !contains(out, expectedContains) {
		t.Fatalf("expected updated content to contain %q, got %q", expectedContains, out)
	}
}

func TestReplaceSectionAppendWhenMissing(t *testing.T) {
	u := NewMarkdownUpdater()
	input := "# Title\n\nSome text"
	out, err := u.ReplaceSection(input, "Recent Changes", "new entry")
	if err != nil {
		t.Fatal(err)
	}

	if !contains(out, "## Recent Changes") || !contains(out, "new entry") {
		t.Fatalf("expected section append behavior, got %q", out)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || stringContains(haystack, needle))
}

func stringContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
