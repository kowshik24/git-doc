package diff

import (
	"fmt"
	"strconv"
	"strings"
)

type Diff struct {
	Files []FileDiff
}

type FileDiff struct {
	Path       string
	Hunks      []Hunk
	AddedLines int
	DelLines   int
}

type Hunk struct {
	OldStart int
	OldLines int
	NewStart int
	NewLines int
	Lines    []string
}

func ParseUnifiedDiff(raw string) (Diff, error) {
	if strings.TrimSpace(raw) == "" {
		return Diff{}, nil
	}

	lines := strings.Split(raw, "\n")
	result := Diff{Files: make([]FileDiff, 0)}

	var currentFile *FileDiff
	var currentHunk *Hunk

	flushHunk := func() {
		if currentFile == nil || currentHunk == nil {
			return
		}
		currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
		currentHunk = nil
	}

	flushFile := func() {
		flushHunk()
		if currentFile == nil {
			return
		}
		result.Files = append(result.Files, *currentFile)
		currentFile = nil
	}

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			flushFile()
			currentFile = &FileDiff{}
		case strings.HasPrefix(line, "+++ b/"):
			if currentFile != nil {
				currentFile.Path = strings.TrimPrefix(line, "+++ b/")
			}
		case strings.HasPrefix(line, "@@"):
			flushHunk()
			h, err := parseHunkHeader(line)
			if err != nil {
				return Diff{}, err
			}
			currentHunk = &h
		default:
			if currentHunk == nil || currentFile == nil {
				continue
			}
			currentHunk.Lines = append(currentHunk.Lines, line)
			if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
				currentFile.AddedLines++
			}
			if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
				currentFile.DelLines++
			}
		}
	}

	flushFile()
	return result, nil
}

func BuildSummary(d Diff) string {
	if len(d.Files) == 0 {
		return "No parseable file-level diff information available."
	}

	lines := make([]string, 0, len(d.Files)+1)
	lines = append(lines, fmt.Sprintf("Files changed: %d", len(d.Files)))
	for _, file := range d.Files {
		path := file.Path
		if strings.TrimSpace(path) == "" {
			path = "(unknown path)"
		}
		lines = append(lines, fmt.Sprintf("- %s (hunks=%d, +%d, -%d)", path, len(file.Hunks), file.AddedLines, file.DelLines))
	}

	return strings.Join(lines, "\n")
}

func TruncateText(content string, maxLen int) string {
	if maxLen <= 0 || len(content) <= maxLen {
		return content
	}
	return content[:maxLen]
}

func parseHunkHeader(header string) (Hunk, error) {
	// Expected format: @@ -a,b +c,d @@ optional-text
	parts := strings.Split(header, "@@")
	if len(parts) < 2 {
		return Hunk{}, fmt.Errorf("invalid hunk header: %s", header)
	}

	core := strings.TrimSpace(parts[1])
	fields := strings.Fields(core)
	if len(fields) < 2 {
		return Hunk{}, fmt.Errorf("invalid hunk header core: %s", header)
	}

	oldStart, oldLines, err := parseRange(fields[0], "-")
	if err != nil {
		return Hunk{}, err
	}
	newStart, newLines, err := parseRange(fields[1], "+")
	if err != nil {
		return Hunk{}, err
	}

	return Hunk{OldStart: oldStart, OldLines: oldLines, NewStart: newStart, NewLines: newLines}, nil
}

func parseRange(token string, prefix string) (int, int, error) {
	trimmed := strings.TrimPrefix(token, prefix)
	parts := strings.Split(trimmed, ",")
	if len(parts) == 0 {
		return 0, 0, fmt.Errorf("invalid range token: %s", token)
	}

	start, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid start in token %s: %w", token, err)
	}

	count := 1
	if len(parts) > 1 {
		count, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, fmt.Errorf("invalid count in token %s: %w", token, err)
		}
	}

	return start, count, nil
}
