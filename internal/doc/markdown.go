package doc

import (
	"fmt"
	"strings"

	"github.com/yuin/goldmark"
)

type Updater interface {
	ExtractSection(content, section string) (string, error)
	ReplaceSection(content, section, newSectionContent string) (string, error)
}

type MarkdownUpdater struct {
	md goldmark.Markdown
}

func NewMarkdownUpdater() *MarkdownUpdater {
	return &MarkdownUpdater{md: goldmark.New()}
}

func (u *MarkdownUpdater) ExtractSection(content, section string) (string, error) {
	lines := strings.Split(content, "\n")
	start, end, found := findSectionBounds(lines, section)
	if !found {
		return "", fmt.Errorf("section %q not found", section)
	}

	return strings.Join(lines[start:end], "\n"), nil
}

func (u *MarkdownUpdater) ReplaceSection(content, section, newSectionContent string) (string, error) {
	lines := strings.Split(content, "\n")
	start, end, found := findSectionBounds(lines, section)
	if !found {
		builder := strings.Builder{}
		builder.WriteString(strings.TrimRight(content, "\n"))
		if !strings.HasSuffix(content, "\n") {
			builder.WriteString("\n")
		}
		builder.WriteString("\n## ")
		builder.WriteString(section)
		builder.WriteString("\n\n")
		builder.WriteString(strings.TrimSpace(newSectionContent))
		builder.WriteString("\n")
		return builder.String(), nil
	}

	updated := make([]string, 0, len(lines))
	updated = append(updated, lines[:start]...)
	trimmed := strings.TrimSpace(newSectionContent)
	if trimmed != "" {
		updated = append(updated, strings.Split(trimmed, "\n")...)
	}
	updated = append(updated, lines[end:]...)

	return strings.Join(updated, "\n"), nil
}

func findSectionBounds(lines []string, section string) (int, int, bool) {
	target := strings.ToLower(strings.TrimSpace(section))
	startHeader := -1
	startContent := -1
	headerLevel := 0

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "#") {
			continue
		}

		level := headingLevel(line)
		title := strings.ToLower(strings.TrimSpace(strings.TrimLeft(line, "#")))
		if title == target {
			startHeader = i
			startContent = i + 1
			headerLevel = level
			break
		}
	}

	if startHeader == -1 {
		return 0, 0, false
	}

	end := len(lines)
	for i := startContent; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "#") && headingLevel(line) <= headerLevel {
			end = i
			break
		}
	}

	for startContent < end && strings.TrimSpace(lines[startContent]) == "" {
		startContent++
	}

	return startContent, end, true
}

func headingLevel(line string) int {
	count := 0
	for _, ch := range line {
		if ch == '#' {
			count++
			continue
		}
		break
	}
	if count == 0 {
		return 7
	}
	return count
}
