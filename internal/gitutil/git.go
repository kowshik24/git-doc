package gitutil

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type CommitInfo struct {
	Hash      string
	Author    string
	Email     string
	Timestamp time.Time
	Subject   string
}

type Helper interface {
	GetRepoRoot() (string, error)
	GetCurrentHEAD() (string, error)
	GetLastProcessedRange(fromHash, toHash string) ([]CommitInfo, error)
	GetCommitDiff(commit string) (string, error)
	GetCommitMessage(commit string) (string, error)
	GetChangedFiles(commit string) ([]string, error)
	StageAndCommit(files []string, message string) (string, error)
	StageAndAmend(files []string) (string, error)
	RevertCommit(commit string) error
}

type CLIHelper struct {
	repoRoot string
}

func NewHelper(repoRoot string) *CLIHelper {
	return &CLIHelper{repoRoot: repoRoot}
}

func GetRepoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to detect git repository root: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func (h *CLIHelper) GetRepoRoot() (string, error) {
	return h.repoRoot, nil
}

func (h *CLIHelper) GetCurrentHEAD() (string, error) {
	out, err := h.run("rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (h *CLIHelper) GetLastProcessedRange(fromHash, toHash string) ([]CommitInfo, error) {
	args := []string{"log", "--pretty=format:%H|%an|%ae|%at|%s", "--reverse"}
	if fromHash != "" {
		args = append(args, fmt.Sprintf("%s..%s", fromHash, toHash))
	} else {
		args = append(args, toHash)
	}

	out, err := h.run(args...)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(out) == "" {
		return nil, nil
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	commits := make([]CommitInfo, 0, len(lines))
	for _, line := range lines {
		parts := strings.SplitN(line, "|", 5)
		if len(parts) != 5 {
			continue
		}

		ts, parseErr := parseUnix(parts[3])
		if parseErr != nil {
			return nil, parseErr
		}

		commits = append(commits, CommitInfo{
			Hash:      parts[0],
			Author:    parts[1],
			Email:     parts[2],
			Timestamp: ts,
			Subject:   parts[4],
		})
	}

	return commits, nil
}

func (h *CLIHelper) GetCommitDiff(commit string) (string, error) {
	return h.run("show", "--unified=3", commit)
}

func (h *CLIHelper) GetCommitMessage(commit string) (string, error) {
	out, err := h.run("log", "-1", "--pretty=%B", commit)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (h *CLIHelper) GetChangedFiles(commit string) ([]string, error) {
	out, err := h.run("diff-tree", "--no-commit-id", "--name-only", "-r", commit)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(out) == "" {
		return nil, nil
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i := range lines {
		lines[i] = filepath.ToSlash(strings.TrimSpace(lines[i]))
	}
	return lines, nil
}

func (h *CLIHelper) StageAndCommit(files []string, message string) (string, error) {
	if len(files) == 0 {
		return "", nil
	}

	args := append([]string{"add"}, files...)
	if _, err := h.run(args...); err != nil {
		return "", err
	}

	if _, err := h.run("commit", "-m", message); err != nil {
		return "", err
	}

	return h.GetCurrentHEAD()
}

func (h *CLIHelper) StageAndAmend(files []string) (string, error) {
	if len(files) == 0 {
		return "", nil
	}

	args := append([]string{"add"}, files...)
	if _, err := h.run(args...); err != nil {
		return "", err
	}

	if _, err := h.run("commit", "--amend", "--no-edit"); err != nil {
		return "", err
	}

	return h.GetCurrentHEAD()
}

func (h *CLIHelper) RevertCommit(commit string) error {
	_, err := h.run("revert", "--no-edit", commit)
	return err
}

func (h *CLIHelper) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = h.repoRoot

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s failed: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return stdout.String(), nil
}

func parseUnix(s string) (time.Time, error) {
	unixInt, err := time.ParseDuration(s + "s")
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid unix timestamp %q: %w", s, err)
	}
	return time.Unix(int64(unixInt.Seconds()), 0), nil
}
