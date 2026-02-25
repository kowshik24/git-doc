package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kowshik24/git-doc/internal/config"
	"github.com/kowshik24/git-doc/internal/doc"
	"github.com/kowshik24/git-doc/internal/gitutil"
	"github.com/kowshik24/git-doc/internal/llm"
	"github.com/kowshik24/git-doc/internal/state"
)

type fakeGitHelper struct {
	repoRoot    string
	head        string
	commitRange []gitutil.CommitInfo
	changed     map[string][]string
	messages    map[string]string
	diffs       map[string]string
	stageCalled int
	amendCalled int
	rangeFrom   string
	rangeTo     string
	seenDiffFor []string
}

func (f *fakeGitHelper) GetRepoRoot() (string, error) {
	return f.repoRoot, nil
}

func (f *fakeGitHelper) GetCurrentHEAD() (string, error) {
	return f.head, nil
}

func (f *fakeGitHelper) GetLastProcessedRange(fromHash, toHash string) ([]gitutil.CommitInfo, error) {
	f.rangeFrom = fromHash
	f.rangeTo = toHash
	return f.commitRange, nil
}

func (f *fakeGitHelper) GetCommitDiff(commit string) (string, error) {
	f.seenDiffFor = append(f.seenDiffFor, commit)
	return f.diffs[commit], nil
}

func (f *fakeGitHelper) GetCommitMessage(commit string) (string, error) {
	return f.messages[commit], nil
}

func (f *fakeGitHelper) GetChangedFiles(commit string) ([]string, error) {
	return f.changed[commit], nil
}

func (f *fakeGitHelper) StageAndCommit(files []string, message string) (string, error) {
	f.stageCalled++
	return "", nil
}

func (f *fakeGitHelper) StageAndAmend(files []string) (string, error) {
	f.amendCalled++
	return "amended-hash", nil
}

func (f *fakeGitHelper) RevertCommit(commit string) error {
	return nil
}

func newTestRepoAndState(t *testing.T) (string, *state.Store) {
	t.Helper()

	repoRoot := t.TempDir()
	dbPath := filepath.Join(repoRoot, ".git-doc", "state.db")
	store, err := state.New(dbPath)
	if err != nil {
		t.Fatalf("create state: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repoRoot, "README.md"), []byte("# Title\n\n## Recent Changes\nold\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	return repoRoot, store
}

func newTestUpdaterWithFakeGit(store *state.Store, fakeGit *fakeGitHelper) *Updater {
	cfg := config.Default()
	cfg.Git.CommitDocUpdates = false
	cfg.DocFiles = []string{"README.md"}

	return NewUpdater(Dependencies{
		Config:     cfg,
		Git:        fakeGit,
		State:      store,
		DocUpdater: doc.NewMarkdownUpdater(),
		LLM:        llm.NewMockClient(),
	})
}

func sampleRangeCommit(hash string) []gitutil.CommitInfo {
	return []gitutil.CommitInfo{{
		Hash:      hash,
		Author:    "bot",
		Email:     "bot@example.com",
		Timestamp: time.Now(),
		Subject:   "range commit",
	}}
}
