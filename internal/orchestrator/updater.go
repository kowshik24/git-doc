package orchestrator

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kowshik24/git-doc/internal/config"
	diffanalyzer "github.com/kowshik24/git-doc/internal/diff"
	"github.com/kowshik24/git-doc/internal/doc"
	"github.com/kowshik24/git-doc/internal/gitutil"
	"github.com/kowshik24/git-doc/internal/llm"
	"github.com/kowshik24/git-doc/internal/state"
)

type Dependencies struct {
	Config     *config.Config
	Git        gitutil.Helper
	State      *state.Store
	DocUpdater doc.Updater
	LLM        llm.Client
}

type Updater struct {
	deps Dependencies
}

type Summary struct {
	Processed int
	Success   int
	Failed    int
	Skipped   int
}

func NewUpdater(deps Dependencies) *Updater {
	return &Updater{deps: deps}
}

func (u *Updater) UpdateNewCommits(ctx context.Context, dryRun bool) (Summary, error) {
	resumableCommits, err := u.deps.State.GetResumableCommits()
	if err != nil {
		return Summary{}, err
	}

	last, err := u.deps.State.GetLastProcessedCommit()
	if err != nil {
		return Summary{}, err
	}

	head, err := u.deps.Git.GetCurrentHEAD()
	if err != nil {
		return Summary{}, err
	}

	commits, err := u.deps.Git.GetLastProcessedRange(last, head)
	if err != nil {
		return Summary{}, err
	}

	commitHashes := make([]string, 0, len(commits))
	for _, c := range commits {
		commitHashes = append(commitHashes, c.Hash)
	}

	commitHashes = mergeUnique(resumableCommits, commitHashes)

	return u.UpdateCommitList(ctx, commitHashes, dryRun)
}

func (u *Updater) UpdateRangeCommits(ctx context.Context, fromHash, toHash string, dryRun bool) (Summary, error) {
	toCommit := strings.TrimSpace(toHash)
	if toCommit == "" {
		head, err := u.deps.Git.GetCurrentHEAD()
		if err != nil {
			return Summary{}, err
		}
		toCommit = head
	}

	commits, err := u.deps.Git.GetLastProcessedRange(strings.TrimSpace(fromHash), toCommit)
	if err != nil {
		return Summary{}, err
	}

	commitHashes := make([]string, 0, len(commits))
	for _, commit := range commits {
		commitHashes = append(commitHashes, commit.Hash)
	}

	return u.UpdateCommitList(ctx, commitHashes, dryRun)
}

func (u *Updater) UpdateCommitList(ctx context.Context, commitHashes []string, dryRun bool) (Summary, error) {
	summary := Summary{}
	runID := fmt.Sprintf("run-%d", time.Now().UnixNano())
	_ = u.deps.State.LogRunEvent(runID, "", "info", "orchestrator", "update loop started", map[string]any{"commits": len(commitHashes)})

	for _, hash := range commitHashes {
		summary.Processed++
		if err := u.deps.State.MarkCommitProcessed(hash, "pending", "", "", nil); err != nil {
			summary.Failed++
			_ = u.deps.State.LogRunEvent(runID, hash, "error", "state", "failed to mark pending", map[string]any{"error": err.Error()})
			continue
		}

		status, err := u.processSingleCommit(ctx, runID, hash, dryRun)
		if err != nil {
			summary.Failed++
			_ = u.deps.State.MarkCommitProcessed(hash, "failed", err.Error(), "", nil)
			_ = u.deps.State.LogRunEvent(runID, hash, "error", "orchestrator", "commit processing failed", map[string]any{"error": err.Error()})
			continue
		}

		switch status {
		case "success":
			summary.Success++
		case "skipped":
			summary.Skipped++
		default:
			summary.Failed++
		}
	}

	_ = u.deps.State.LogRunEvent(runID, "", "info", "orchestrator", "update loop finished", map[string]any{
		"processed": summary.Processed,
		"success":   summary.Success,
		"failed":    summary.Failed,
		"skipped":   summary.Skipped,
	})

	return summary, nil
}

func (u *Updater) processSingleCommit(ctx context.Context, runID, hash string, dryRun bool) (string, error) {
	if err := u.deps.State.MarkCommitProcessed(hash, "in_progress", "", "", nil); err != nil {
		return "failed", err
	}

	changedFiles, err := u.deps.Git.GetChangedFiles(hash)
	if err != nil {
		return "failed", err
	}

	if len(changedFiles) == 0 {
		if err := u.deps.State.MarkCommitProcessed(hash, "skipped", "", "", nil); err != nil {
			return "failed", err
		}
		return "skipped", nil
	}

	commitMessage, err := u.deps.Git.GetCommitMessage(hash)
	if err != nil {
		return "failed", err
	}

	diffContent, err := u.deps.Git.GetCommitDiff(hash)
	if err != nil {
		return "failed", err
	}

	targetDocFile, targetSection := u.resolveTarget(changedFiles)
	repoRoot, err := u.deps.Git.GetRepoRoot()
	if err != nil {
		return "failed", err
	}

	docPath := filepath.Join(repoRoot, targetDocFile)
	docRaw, err := os.ReadFile(docPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "failed", fmt.Errorf("target doc file not found: %s", targetDocFile)
		}
		return "failed", err
	}

	if err := u.deps.State.UpsertPlannedUpdate(hash, targetDocFile, targetSection, "inferred", "planned", ""); err != nil {
		_ = u.deps.State.LogRunEvent(runID, hash, "warn", "state", "failed to persist planned update", map[string]any{"error": err.Error()})
	}

	prompt := buildPrompt(commitMessage, diffContent)
	providerName := u.deps.LLM.Name()
	modelName := u.deps.Config.LLM.Model
	promptHash := hashPrompt(prompt)

	newSection, cached, cacheErr := u.deps.State.GetCachedLLMResponse(hash, targetDocFile, targetSection, providerName, modelName, prompt)
	if cacheErr != nil {
		_ = u.deps.State.LogRunEvent(runID, hash, "warn", "state", "failed to read llm cache", map[string]any{"error": cacheErr.Error()})
	}

	if !cached {
		newSection, err = u.deps.LLM.Generate(ctx, prompt)
		if err != nil {
			_ = u.deps.State.UpsertPlannedUpdate(hash, targetDocFile, targetSection, "inferred", "failed", err.Error())
			return "failed", err
		}

		_ = u.deps.State.PutCachedLLMResponse(state.LLMCacheEntry{
			CommitHash: hash,
			DocFile:    targetDocFile,
			SectionID:  targetSection,
			Provider:   providerName,
			Model:      modelName,
			PromptHash: promptHash,
			Response:   newSection,
		})
	} else {
		_ = u.deps.State.LogRunEvent(runID, hash, "info", "llm", "cache hit", map[string]any{"doc_file": targetDocFile, "section": targetSection})
	}

	if err := validateGeneratedSection(newSection); err != nil {
		_ = u.deps.State.UpsertPlannedUpdate(hash, targetDocFile, targetSection, "inferred", "failed", err.Error())
		return "failed", err
	}

	updated, err := u.deps.DocUpdater.ReplaceSection(string(docRaw), targetSection, newSection)
	if err != nil {
		_ = u.deps.State.UpsertPlannedUpdate(hash, targetDocFile, targetSection, "inferred", "failed", err.Error())
		return "failed", err
	}

	lineEnding := doc.DetectLineEnding(string(docRaw))
	updated = doc.NormalizeLineEndings(updated, lineEnding)

	if strings.TrimSpace(updated) == strings.TrimSpace(string(docRaw)) {
		_ = u.deps.State.UpsertPlannedUpdate(hash, targetDocFile, targetSection, "inferred", "unchanged", "no document delta")
		if err := u.deps.State.MarkCommitProcessed(hash, "skipped", "", "", []string{}); err != nil {
			return "failed", err
		}
		return "skipped", nil
	}

	if dryRun {
		_ = u.deps.State.UpsertPlannedUpdate(hash, targetDocFile, targetSection, "inferred", "applied", "dry-run")
		if err := u.deps.State.MarkCommitProcessed(hash, "success", "", "", []string{targetDocFile}); err != nil {
			return "failed", err
		}
		return "success", nil
	}

	if err := doc.AtomicWriteFile(docPath, []byte(updated), 0o644); err != nil {
		_ = u.deps.State.UpsertPlannedUpdate(hash, targetDocFile, targetSection, "inferred", "failed", err.Error())
		return "failed", err
	}

	docCommitHash := ""
	if u.deps.Config.Git.CommitDocUpdates {
		if u.deps.Config.Git.AmendOriginal {
			docCommitHash, err = u.deps.Git.StageAndAmend([]string{targetDocFile})
		} else {
			msg := strings.ReplaceAll(u.deps.Config.Git.DocCommitMessage, "{hash}", hash)
			docCommitHash, err = u.deps.Git.StageAndCommit([]string{targetDocFile}, msg)
		}
		if err != nil {
			return "failed", err
		}
	}

	if err := u.deps.State.MarkCommitProcessed(hash, "success", "", docCommitHash, []string{targetDocFile}); err != nil {
		return "failed", err
	}

	if err := u.deps.State.StoreMapping(hash, targetDocFile, targetSection); err != nil {
		return "failed", err
	}

	_ = u.deps.State.UpsertPlannedUpdate(hash, targetDocFile, targetSection, "inferred", "applied", "")

	return "success", nil
}

func (u *Updater) resolveTarget(changedFiles []string) (string, string) {
	for _, changed := range changedFiles {
		for _, mapping := range u.deps.Config.Mappings {
			if strings.Contains(changed, strings.Trim(mapping.CodePattern, "*")) {
				return mapping.DocFile, mapping.Section
			}
		}
	}

	if len(u.deps.Config.DocFiles) > 0 {
		return u.deps.Config.DocFiles[0], u.deps.Config.Runtime.DefaultSection
	}

	return "README.md", u.deps.Config.Runtime.DefaultSection
}

func buildPrompt(commitMessage, diff string) string {
	diffContext := ""
	parsed, err := diffanalyzer.ParseUnifiedDiff(diff)
	if err == nil && len(parsed.Files) > 0 {
		diffContext = diffanalyzer.BuildSummary(parsed)
		diffContext = diffanalyzer.TruncateText(diffContext, 3000)
	} else {
		diffContext = diffanalyzer.TruncateText(diff, 3000)
	}

	return fmt.Sprintf(
		"Update docs for this commit.\nCommit message: %s\nDiff:\n%s\nOutput updated section content only.",
		commitMessage,
		diffContext,
	)
}

func mergeUnique(first []string, second []string) []string {
	seen := make(map[string]struct{}, len(first)+len(second))
	out := make([]string, 0, len(first)+len(second))

	for _, item := range first {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}

	for _, item := range second {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}

	return out
}

func validateGeneratedSection(content string) error {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return fmt.Errorf("generated section content is empty")
	}
	if len(trimmed) > 25000 {
		return fmt.Errorf("generated section content exceeds max size")
	}
	return nil
}

func hashPrompt(prompt string) string {
	sum := sha256.Sum256([]byte(prompt))
	return fmt.Sprintf("%x", sum)
}
