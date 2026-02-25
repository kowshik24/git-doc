package orchestrator

import (
	"context"
	"testing"
)

func TestUpdateNewCommits_ReprocessesPendingAndInProgress(t *testing.T) {
	repoRoot, store := newTestRepoAndState(t)

	if err := store.MarkCommitProcessed("c-pending", "pending", "", "", nil); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkCommitProcessed("c-progress", "in_progress", "", "", nil); err != nil {
		t.Fatal(err)
	}

	fakeGit := &fakeGitHelper{
		repoRoot:    repoRoot,
		head:        "head-hash",
		commitRange: nil,
		changed: map[string][]string{
			"c-pending":  {"src/a.go"},
			"c-progress": {"src/b.go"},
		},
		messages: map[string]string{
			"c-pending":  "feat: pending",
			"c-progress": "fix: progress",
		},
		diffs: map[string]string{
			"c-pending":  "diff --git a/src/a.go b/src/a.go\n+new",
			"c-progress": "diff --git a/src/b.go b/src/b.go\n+new",
		},
	}

	updater := newTestUpdaterWithFakeGit(store, fakeGit)

	summary, err := updater.UpdateNewCommits(context.Background(), false)
	if err != nil {
		t.Fatalf("update new commits failed: %v", err)
	}

	if summary.Processed != 2 || summary.Success != 2 || summary.Failed != 0 {
		t.Fatalf("unexpected summary: %+v", summary)
	}

	rows, err := store.ListRecent(10)
	if err != nil {
		t.Fatal(err)
	}

	statusByCommit := map[string]string{}
	for _, row := range rows {
		statusByCommit[row.CommitHash] = row.Status
	}

	if statusByCommit["c-pending"] != "success" {
		t.Fatalf("expected c-pending to be success, got %q", statusByCommit["c-pending"])
	}
	if statusByCommit["c-progress"] != "success" {
		t.Fatalf("expected c-progress to be success, got %q", statusByCommit["c-progress"])
	}
}

func TestUpdateNewCommits_DedupsResumableAndRangeCommit(t *testing.T) {
	repoRoot, store := newTestRepoAndState(t)

	if err := store.MarkCommitProcessed("dup-commit", "pending", "", "", nil); err != nil {
		t.Fatal(err)
	}

	fakeGit := &fakeGitHelper{
		repoRoot:    repoRoot,
		head:        "head-hash",
		commitRange: sampleRangeCommit("dup-commit"),
		changed: map[string][]string{
			"dup-commit": {"src/a.go"},
		},
		messages: map[string]string{
			"dup-commit": "feat: dedup",
		},
		diffs: map[string]string{
			"dup-commit": "diff --git a/src/a.go b/src/a.go\n+new",
		},
	}

	updater := newTestUpdaterWithFakeGit(store, fakeGit)

	summary, err := updater.UpdateNewCommits(context.Background(), false)
	if err != nil {
		t.Fatalf("update new commits failed: %v", err)
	}

	if summary.Processed != 1 || summary.Success != 1 {
		t.Fatalf("expected single processing after dedup, got summary=%+v", summary)
	}

	if len(fakeGit.seenDiffFor) != 1 {
		t.Fatalf("expected commit diff to be requested once, got %d", len(fakeGit.seenDiffFor))
	}

	if fakeGit.seenDiffFor[0] != "dup-commit" {
		t.Fatalf("unexpected commit processed: %v", fakeGit.seenDiffFor)
	}
}

func TestUpdateRangeCommits_UsesProvidedBounds(t *testing.T) {
	repoRoot, store := newTestRepoAndState(t)

	fakeGit := &fakeGitHelper{
		repoRoot:    repoRoot,
		head:        "head-hash",
		commitRange: sampleRangeCommit("range-commit"),
		changed: map[string][]string{
			"range-commit": {"src/r.go"},
		},
		messages: map[string]string{
			"range-commit": "feat: range update",
		},
		diffs: map[string]string{
			"range-commit": "diff --git a/src/r.go b/src/r.go\n+new",
		},
	}

	updater := newTestUpdaterWithFakeGit(store, fakeGit)

	summary, err := updater.UpdateRangeCommits(context.Background(), "from-1", "to-1", false)
	if err != nil {
		t.Fatalf("update range commits failed: %v", err)
	}

	if summary.Processed != 1 || summary.Success != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}

	if fakeGit.rangeFrom != "from-1" || fakeGit.rangeTo != "to-1" {
		t.Fatalf("expected range from/to to be propagated, got from=%q to=%q", fakeGit.rangeFrom, fakeGit.rangeTo)
	}
}

func TestUpdateCommitList_UsesAmendOriginalWhenConfigured(t *testing.T) {
	repoRoot, store := newTestRepoAndState(t)

	fakeGit := &fakeGitHelper{
		repoRoot: repoRoot,
		changed: map[string][]string{
			"amend-commit": {"src/a.go"},
		},
		messages: map[string]string{
			"amend-commit": "feat: amend",
		},
		diffs: map[string]string{
			"amend-commit": "diff --git a/src/a.go b/src/a.go\n+new",
		},
	}

	updater := newTestUpdaterWithFakeGit(store, fakeGit)
	updater.deps.Config.Git.CommitDocUpdates = true
	updater.deps.Config.Git.AmendOriginal = true

	summary, err := updater.UpdateCommitList(context.Background(), []string{"amend-commit"}, false)
	if err != nil {
		t.Fatalf("update commit list failed: %v", err)
	}

	if summary.Success != 1 {
		t.Fatalf("expected successful amend flow, summary=%+v", summary)
	}
	if fakeGit.amendCalled != 1 {
		t.Fatalf("expected amend path to be used once, got %d", fakeGit.amendCalled)
	}
	if fakeGit.stageCalled != 0 {
		t.Fatalf("expected stage-and-commit path not to be used, got %d", fakeGit.stageCalled)
	}
}
