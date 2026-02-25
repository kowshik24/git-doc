package state

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestStoreRoundTrip(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("failed to create state store: %v", err)
	}

	if err := store.MarkCommitProcessed("abc", "success", "", "doc123", []string{"README.md"}); err != nil {
		t.Fatalf("mark commit: %v", err)
	}

	last, err := store.GetLastProcessedCommit()
	if err != nil {
		t.Fatalf("get last processed: %v", err)
	}
	if last != "abc" {
		t.Fatalf("expected abc, got %s", last)
	}

	if err := store.MarkCommitProcessed("def", "failed", "boom", "", nil); err != nil {
		t.Fatalf("mark failed commit: %v", err)
	}

	failed, err := store.GetFailedCommits()
	if err != nil {
		t.Fatalf("get failed commits: %v", err)
	}
	if len(failed) != 1 || failed[0] != "def" {
		t.Fatalf("unexpected failed commits: %#v", failed)
	}

	docCommit, err := store.GetDocCommitHash("abc")
	if err != nil {
		t.Fatalf("get doc commit hash: %v", err)
	}
	if docCommit != "doc123" {
		t.Fatalf("expected doc123, got %q", docCommit)
	}
}

func TestGetResumableCommits(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("failed to create state store: %v", err)
	}

	if err := store.MarkCommitProcessed("a1", "pending", "", "", nil); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkCommitProcessed("a2", "in_progress", "", "", nil); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkCommitProcessed("a3", "success", "", "", nil); err != nil {
		t.Fatal(err)
	}

	resumable, err := store.GetResumableCommits()
	if err != nil {
		t.Fatal(err)
	}

	if len(resumable) != 2 {
		t.Fatalf("expected 2 resumable commits, got %d", len(resumable))
	}
}

func TestGetStatusCounts(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("failed to create state store: %v", err)
	}

	_ = store.MarkCommitProcessed("c1", "pending", "", "", nil)
	_ = store.MarkCommitProcessed("c2", "in_progress", "", "", nil)
	_ = store.MarkCommitProcessed("c3", "success", "", "", nil)
	_ = store.MarkCommitProcessed("c4", "failed", "boom", "", nil)
	_ = store.MarkCommitProcessed("c5", "skipped", "", "", nil)

	counts, err := store.GetStatusCounts()
	if err != nil {
		t.Fatal(err)
	}

	if counts.Pending != 1 || counts.InProgress != 1 || counts.Success != 1 || counts.Failed != 1 || counts.Skipped != 1 {
		t.Fatalf("unexpected counts: %+v", counts)
	}
	if counts.Total != 5 {
		t.Fatalf("expected total=5, got %d", counts.Total)
	}
}

func TestGetRetryableCommits(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("failed to create state store: %v", err)
	}

	_ = store.MarkCommitProcessed("r1", "failed", "boom", "", nil)
	_ = store.MarkCommitProcessed("r2", "in_progress", "", "", nil)
	_ = store.MarkCommitProcessed("r3", "pending", "", "", nil)
	_ = store.MarkCommitProcessed("r4", "success", "", "", nil)

	retryable, err := store.GetRetryableCommits()
	if err != nil {
		t.Fatal(err)
	}

	if len(retryable) != 2 {
		t.Fatalf("expected 2 retryable commits, got %d (%v)", len(retryable), retryable)
	}
}

func TestPlannedUpdateCacheAndRunEvents(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("failed to create state store: %v", err)
	}

	if err := store.UpsertPlannedUpdate("p1", "README.md", "Recent Changes", "inferred", "planned", ""); err != nil {
		t.Fatalf("upsert planned update: %v", err)
	}

	prompt := "hello-prompt"
	if err := store.PutCachedLLMResponse(LLMCacheEntry{
		CommitHash: "p1",
		DocFile:    "README.md",
		SectionID:  "Recent Changes",
		Provider:   "mock",
		Model:      "gpt-4o-mini",
		PromptHash: hashPrompt(prompt),
		Response:   "cached-response",
	}); err != nil {
		t.Fatalf("put cache: %v", err)
	}

	resp, hit, err := store.GetCachedLLMResponse("p1", "README.md", "Recent Changes", "mock", "gpt-4o-mini", prompt)
	if err != nil {
		t.Fatalf("get cache: %v", err)
	}
	if !hit || resp != "cached-response" {
		t.Fatalf("unexpected cache result: hit=%v resp=%q", hit, resp)
	}

	if err := store.LogRunEvent("run-1", "p1", "info", "test", "message", map[string]any{"k": "v"}); err != nil {
		t.Fatalf("log run event: %v", err)
	}

	var count int
	err = store.db.QueryRow(`SELECT COUNT(*) FROM run_events WHERE run_id = 'run-1'`).Scan(&count)
	if err != nil && err != sql.ErrNoRows {
		t.Fatalf("query run events: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 run event, got %d", count)
	}
}
