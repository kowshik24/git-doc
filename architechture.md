# Full Project Architecture: `git-doc`

## 1. Overview & Goals

**git-doc** is a CLI tool that automatically updates project documentation based on Git commits. It leverages user-provided LLM API keys to generate or modify documentation sections corresponding to code changes. The tool integrates deeply with Git, tracking processed commits, handling failures, and enabling reverts. It works seamlessly on Windows, Linux, and macOS.

**Core Goals:**

- Detect new commits since last run and process them in order.
- Intelligently link code changes to specific documentation sections (via mapping, special comments, or LLM inference).
- Call LLM (OpenAI, Anthropic, Gemini, Groq, etc.) to generate updated documentation content.
- Apply updates to documentation files and commit them separately (or amend original commits, configurable).
- Maintain a state database to track processed commits, failures, and associated doc commits.
- Provide commands to enable/disable Git hooks for automatic updates.
- Support revert of documentation changes tied to specific commits.
- Be robust across platforms, handle errors gracefully, and respect `.gitignore`.

## 2. System Requirements

- **Operating Systems:** Windows (10+), Linux (kernel 3.10+), macOS (10.15+).
- **Dependencies:** Git (any recent version) must be installed and available in PATH.
- **LLM Providers:** Valid API keys for chosen provider(s). Internet connection required for cloud LLMs; local models (Ollama) can work offline.
- **Disk Space:** Minimal (< 50 MB for binary, plus SQLite state).
- **Memory:** Adequate for processing diffs and LLM responses (typically < 500 MB).

## 3. High-Level Architecture

The tool is built in **Go** (1.21+) and structured as a set of modular components:

```
┌─────────────────┐
│   CLI (cobra)   │
└────────┬────────┘
         │
         ▼
┌─────────────────────────────────────┐
│          Core Orchestrator           │
│  - Processes commands                │
│  - Coordinates component interactions│
└─┬─────┬──────┬─────────┬─────────────┘
  │     │      │         │
  ▼     ▼      ▼         ▼
┌──────────┐┌──────────┐┌──────────┐┌────────────┐
│ Config   ││ Git      ││ State    ││ Doc        │
│ Manager  ││ Helper   ││ Manager  ││ Parser     │
└──────────┘└──────────┘└──────────┘└────────────┘
                            │              │
                            ▼              ▼
                    ┌──────────────┐┌──────────────┐
                    │   SQLite     ││ Markdown AST │
                    │   (state.db) ││   (goldmark) │
                    └──────────────┘└──────────────┘

┌──────────┐┌──────────┐┌──────────┐
│ Diff     ││ LLM      ││ Hook     │
│ Analyzer ││ Client   ││ Manager  │
└──────────┘└──────────┘└──────────┘
                  │
                  ▼
        ┌─────────────────┐
        │ Multi-Provider  │
        │   (OpenAI,      │
        │ Anthropic, ...) │
        └─────────────────┘
```

## 4. Detailed Component Design

### 4.1 CLI Layer (Cobra)

**Commands:**

- `git-doc init` – Initializes `.git-doc/` directory with default config and mapping file. Detects repository root.
- `git-doc update [--from <commit>] [--to <commit>]` – Manually processes commits in the given range (default: since last processed to HEAD). If no state, processes all commits.
- `git-doc enable-hook` – Installs Git hooks (post-commit, post-merge, post-rewrite) to run `update` automatically.
- `git-doc disable-hook` – Removes installed hooks.
- `git-doc status` – Shows pending, failed, and processed commits with summary.
- `git-doc retry [--commit <hash>]` – Retries failed commits (if no hash, retry all failed).
- `git-doc revert <commit-hash>` – Reverts documentation changes associated with the given code commit.
- `git-doc config` – Displays or edits configuration (launches editor).
- `git-doc version` – Shows version.

**Flags:**

- `--dry-run` – Preview changes without applying or committing.
- `--verbose` – Detailed logs.
- `--config` – Path to config file (default `.git-doc/config.toml`).

### 4.2 Configuration Manager

**Location:** `.git-doc/config.toml` (in repository root).

**Schema:**

```toml
# LLM settings
[llm]
provider = "openai"          # openai, anthropic, google, groq, ollama
api_key = "${GITDOC_OPENAI_KEY}"  # env var or plain text
model = "gpt-4"              # provider-specific model
timeout = 60                  # seconds
max_retries = 3

# Documentation files to manage (glob patterns)
doc_files = [
    "README.md",
    "docs/**/*.md",
    "*.rst"
]

# Optional: manual mapping between code files and doc sections
[[mappings]]
code_pattern = "src/api/**/*.py"
doc_file = "README.md"
section = "API Reference"

[[mappings]]
code_pattern = "src/models/user.go"
doc_file = "docs/models.md"
section = "User Model"

# Git commit behavior
[git]
commit_doc_updates = true     # create separate doc commits
amend_original = false        # if true, amend original commit (dangerous if pushed)
doc_commit_message = "docs: auto-update for {hash}"

# State
[state]
db_path = ".git-doc/state.db"  # SQLite database
```

**Implementation:**

- Use `BurntSushi/toml` for parsing.
- Expand environment variables in strings (e.g., `${VAR}`).
- Validate provider and required fields.
- Provide defaults.

### 4.3 Git Helper

**Purpose:** Execute Git commands and parse outputs. Use shell execution for simplicity and reliability (assumes `git` in PATH). All functions return structured data or errors.

**Key Functions:**

- `GetRepoRoot() (string, error)` – Run `git rev-parse --show-toplevel`.
- `GetCommitsRange(from, to string) ([]CommitInfo, error)` – Run `git log --pretty=format:"%H|%an|%ae|%at|%s" --reverse <from>..<to>`.
- `GetCommitDiff(commit string) (string, error)` – Run `git show --unified=3 <commit>`.
- `GetCommitMessage(commit string) (string, error)` – Run `git log -1 --pretty=%B <commit>`.
- `GetChangedFiles(commit string) ([]string, error)` – Run `git diff-tree --no-commit-id --name-only -r <commit>`.
- `StageAndCommit(files []string, message string) error` – `git add <files>` then `git commit -m <message>`.
- `RevertCommit(commit string) error` – `git revert --no-edit <commit>`.
- `GetCurrentHEAD() (string, error)` – `git rev-parse HEAD`.

**Error Handling:** Capture stderr, return descriptive errors.

### 4.4 State Manager (SQLite)

**Purpose:** Track processed commits, their status, and associated doc commits.

**Database Schema:**

```sql
CREATE TABLE processed_commits (
    commit_hash TEXT PRIMARY KEY,
    processed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    status TEXT CHECK(status IN ('success', 'failed', 'skipped')),
    error TEXT,
    doc_commit_hash TEXT,          -- hash of the doc update commit (if any)
    doc_files_changed TEXT,         -- JSON array of files updated
    metadata TEXT                   -- JSON for extra info (e.g., tokens used)
);

CREATE TABLE mappings (
    id INTEGER PRIMARY KEY,
    code_commit_hash TEXT,
    doc_file TEXT,
    section TEXT,
    FOREIGN KEY(code_commit_hash) REFERENCES processed_commits(commit_hash)
);
```

**Functions:**

- `GetLastProcessedCommit() (string, error)` – Returns the most recent successful commit hash.
- `MarkCommitProcessed(commit, status, error, docCommit, filesChanged)`
- `GetFailedCommits() ([]string, error)`
- `GetCommitMapping(commit string) ([]Mapping, error)`
- `StoreMapping(commit, docFile, section)`

We'll use `modernc.org/sqlite` (pure Go, no CGO) for cross-platform compatibility.

### 4.5 Diff Analyzer

**Purpose:** Parse unified diff output to extract changed hunks and file paths.

**Input:** Raw diff string from `git show`.

**Output:** Structured diff:

```go
type Diff struct {
    Files []FileDiff
}
type FileDiff struct {
    Path    string
    Hunks   []Hunk
}
type Hunk struct {
    OldStart, OldLines int
    NewStart, NewLines int
    Lines              []string // each line with +/-/ context
}
```

**Implementation:** Use `sourcegraph/go-diff` or a simple parser. We'll also compute a summary (e.g., "added function X, modified Y") for LLM prompts.

**Token Budgeting:** If total diff exceeds a threshold (e.g., 4000 tokens), we either:

- Summarize (only show file names and change types).
- Process only files that map to documentation (via mapping).
- Ask LLM to summarize before generating doc updates (chaining calls).

### 4.6 Documentation Parser & Updater

**Purpose:** Parse documentation files (Markdown initially) to identify sections, and apply updates.

**Markdown Support:** Use `github.com/yuin/goldmark` with AST traversal to find headings and their content.

**Section Identification:**

- Headings (level 1–6) define sections.
- Content under a heading until next heading of same or higher level.
- Special comments: `<!-- git-doc: section=api -->` can mark a block even without heading.

**Updating:**

- Given a section name (heading text or comment ID), replace its content.
- For mapping-based updates, we have explicit target.
- For LLM-inferred, we may need to replace a whole section or append to "Recent Changes".

**Merging Strategy:**

- Preserve everything outside the updated section.
- Write changes to a temporary file, then atomically rename.

**Supported Formats:** Initially Markdown (`.md`). Later: `.rst`, `.txt` via plugins.

### 4.7 LLM Client (Multi-Provider)

**Interface:**

```go
type LLMClient interface {
    Generate(prompt string) (string, error)
    Name() string
}
```

**Provider Implementations:**

- `OpenAIClient`: Uses `gpt-3.5-turbo` or `gpt-4` with chat completions.
- `AnthropicClient`: Claude models via Messages API.
- `GoogleClient`: Gemini via `generateContent`.
- `GroqClient`: Mixtral, Llama via Groq API.
- `OllamaClient`: Local models via Ollama REST API.

**Prompt Engineering:**

```
You are an assistant that updates documentation based on code changes.
Given:
- Commit message: {message}
- Code diff (unified): {diff}
- Current documentation section: {section_content}

Instructions:
- Update the section to reflect the code changes.
- Only output the updated section content, no extra text or explanations.
- Maintain the same tone and style as the original section.
- If the changes are not relevant to this section, output the original content unchanged.
```

**Token Management:** Use `tiktoken`-like library (or approximate) to count tokens before sending. If prompt too long, truncate diff (e.g., keep only added lines) or split across multiple calls.

**Error Handling:** Retry with exponential backoff on rate limits. If provider fails, mark commit as failed with error.

### 4.8 Hook Manager

**Purpose:** Install/remove Git hooks in `.git/hooks/`.

**Hooks to install:**

- `post-commit`
- `post-merge`
- `post-rewrite` (for rebase, amend)

**Hook Script (bash):**

```bash
#!/bin/sh
git-doc update > /dev/null 2>&1 &
```

The ampersand runs in background to not block Git. On Windows, Git Bash will execute this.

**Implementation:**

- Check if `.git/hooks/` exists.
- Write script with appropriate shebang.
- Make executable (`os.Chmod`).

**Disable:** Remove the hook files (or rename).

### 4.9 Revert & History

**Revert command workflow:**

1. User runs `git-doc revert <code-commit-hash>`.
2. Query state for doc commits associated with that code commit.
3. For each doc commit, run `git revert <doc-commit-hash>` (or `git checkout` previous version? Use revert to keep history).
4. Update state to reflect revert (optional).

**Alternative:** If doc updates were committed separately, user can also manually revert using Git. Our command is a convenience.

**History Tracking:**

- The state database stores doc_commit_hash, so we can link.
- For revert, we also log a revert event in state (optional).

## 5. Data Flow Diagrams

### 5.1 `git-doc update` Flow

```
Start
 │
 ▼
Load config
 │
 ▼
Get last processed commit from state (or none)
 │
 ▼
Get list of new commits from Git (since last to HEAD)
 │
 ▼
For each commit in order:
 │   ├─ Get diff and commit message
 │   ├─ Determine target doc files/sections (mapping/comments/LLM)
 │   ├─ For each target:
 │   │   ├─ Read current doc file
 │   │   ├─ Extract section content
 │   │   ├─ Build prompt and call LLM
 │   │   ├─ Parse LLM response (updated section)
 │   │   ├─ Merge into doc file (replace section)
 │   │   └─ Write updated file
 │   ├─ Stage updated doc files
 │   ├─ Commit (separate commit) with message
 │   └─ Mark commit as processed in state, store doc commit hash
 │
 └─ After loop: done
```

### 5.2 Hook Trigger Flow

```
Git commit
   │
   ▼
post-commit hook (installed by git-doc)
   │
   ▼
git-doc update (background)
   │
   ▼
(same as above)
```

## 6. Error Handling & Edge Cases

| Edge Case                                               | Handling Strategy                                                                                                                                                                                 |
| ------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **No Git repository**                             | All commands (except `init`) should detect and error with helpful message. `init` can initialize if in a repo, else error.                                                                    |
| **Git not in PATH**                               | Error with clear instruction to install Git.                                                                                                                                                      |
| **No API key configured**                         | Error during `update`; prompt user to run `git-doc config`.                                                                                                                                   |
| **LLM provider unavailable/rate limited**         | Retry with backoff (configurable). After max retries, mark commit as failed and continue.                                                                                                         |
| **Diff too large for LLM context**                | Summarize diff (only file names and types) or process files individually.                                                                                                                         |
| **LLM returns invalid/malformed content**         | Log warning; if parsing fails, keep original section and mark commit as failed? We'll mark as failed but preserve state.                                                                          |
| **Documentation file not found or deleted**       | Skip with warning; maybe create if missing? Not by default.                                                                                                                                       |
| **Merge conflicts during doc commit**             | Since doc commits are separate, conflicts unlikely. If they occur (e.g., concurrent edits), we abort and mark commit as failed.                                                                   |
| **Rebase / amend**                                | `post-rewrite` hook triggers. We must handle that commits may be rewritten. We'll store original commit hash in state, and on rewrite, we can reprocess the new commit and mark old as skipped. |
| **Stash / pop**                                   | No hook; user can run `git-doc update` manually after pop.                                                                                                                                      |
| **Commit with no code changes (e.g., docs only)** | Diff may be empty; we can skip or still update docs if mapping matches? Probably skip.                                                                                                            |
| **Multiple doc files changed**                    | Process each file independently, but commit all changes in one doc commit (or per file? We'll do one commit for all changes from a code commit).                                                  |
| **Concurrent runs**                               | Use file locking on state database to prevent corruption. SQLite handles concurrent writes via locks.                                                                                             |
| **User interrupts (Ctrl+C)**                      | Gracefully abort current commit processing; state not updated for that commit. Next run will reprocess from last successful.                                                                      |
| **Windows path issues**                           | Use `filepath.Join` and ensure Git commands use forward slashes (Git accepts both).                                                                                                             |

## 7. Cross-Platform Considerations

- **Binary distribution:** Provide pre-compiled binaries for Windows (amd64), Linux (amd64, arm64), macOS (amd64, arm64). Use Go's build constraints where needed.
- **File permissions:** Ensure config and state files are created with `0600` (user read/write only) for API key safety.
- **Git hooks:** Write shell scripts with `#!/bin/sh`. On Windows, Git Bash will execute them. Ensure the script calls `git-doc` (which must be in PATH). We can also fallback to a batch file, but not needed.
- **Line endings:** When reading/writing doc files, preserve original line endings (detect by scanning first few lines). Go's default is to read as is; writing will use system default unless we preserve.
- **Environment variables:** Use `os.ExpandEnv` for config strings.
- **SQLite:** Use `modernc.org/sqlite` (pure Go) to avoid CGO and cross-compilation issues.

## 8. Security & Privacy

- **API keys:** Store in config file with restrictive permissions. Recommend using environment variables to avoid accidental commit.
- **Data sent to LLM:** Clearly document that code diffs are sent to third-party APIs. For sensitive projects, suggest using local models (Ollama).
- **Git hooks:** The tool modifies `.git/hooks/`; ensure we don't overwrite existing hooks without backup (or ask user). We'll store backups.
- **State database:** Contains commit hashes and metadata only, no secrets.

## 9. Testing Strategy

**Unit Tests:**

- Config parsing, environment expansion.
- Git helper functions (with mock exec).
- Diff analyzer (with sample diffs).
- Markdown parser and section replacement.
- LLM client mock for testing prompts.

**Integration Tests:**

- Create temporary Git repositories, make commits, run `git-doc update`, verify doc files and commits.
- Test hooks by simulating commits and checking background process.
- Test error cases (missing API key, LLM failure).

**Cross-Platform Testing:**

- Run tests on Windows (Git Bash), Linux, macOS in CI (GitHub Actions).

## 10. Future Extensions

- Support for more doc formats (reStructuredText, AsciiDoc).
- Plugin system for custom doc parsers/updaters.
- Web dashboard for visualizing doc status.
- Integration with CI/CD to enforce doc updates before merge.
- Support for multiple languages in LLM prompts.

## 11. Architecture Enhancements (v2 Delta)

This section adds production-grade improvements while preserving the current design and command surface.

### 11.1 Deterministic Two-Phase Processing

Split `git-doc update` into two explicit phases internally:

1. **Plan phase (deterministic):**
     - Resolve commit range.
     - Parse diffs.
     - Resolve candidate doc targets (mapping/comments/inference).
     - Produce a `planned_updates` record per target.
2. **Apply phase (LLM-assisted):**
     - Execute each plan item.
     - Validate section boundaries.
     - Write files atomically.
     - Commit and mark success/failure.

Benefits:

- Better crash recovery and replay.
- Easier observability (`planned`, `applied`, `failed`).
- Lower risk of partial updates.

### 11.2 Idempotency and Resume Safety

- If generated content equals existing section content, mark target as `unchanged`.
- If all targets are unchanged, mark commit as `skipped` and do not create a doc commit.
- Add `in_progress` state for commit processing:
    - Start: `pending`/`in_progress`.
    - End: `success`, `failed`, or `skipped`.
- On restart, resume `in_progress` safely using plan records.

### 11.3 Prompt/Response Caching

- Add cache table keyed by:
    - `commit_hash`
    - `doc_file`
    - `section_id`
    - `prompt_hash`
- Reuse cached responses during retry for deterministic reruns and lower API cost.
- Add TTL/version field so cache can be invalidated on prompt-template upgrades.

### 11.4 Output Validation Guardrails

Before writing section updates, enforce:

- Section heading or comment marker still exists.
- Replacement stays within section bounds.
- No deletion of unrelated sibling sections.
- Max output size limits.

If validation fails, mark target failed with explicit reason and keep file unchanged.

### 11.5 Multi-Provider Reliability

- Introduce provider policy:
    - Primary provider + ordered fallbacks.
    - Per-provider timeout and retry budget.
    - Circuit breaker (open on repeated failures, half-open after cooldown).
- Log provider used per target for observability and cost analysis.

### 11.6 Concurrency Control

- Keep SQLite locking but add process-level lock file: `.git-doc/run.lock`.
- If lock exists and active PID is alive, new run exits with clear message.
- Hooks should no-op quickly when a run is already active.

## 12. Expanded Data Model

Additive schema extensions:

```sql
CREATE TABLE IF NOT EXISTS planned_updates (
        id INTEGER PRIMARY KEY,
        commit_hash TEXT NOT NULL,
        doc_file TEXT NOT NULL,
        section_id TEXT NOT NULL,
        strategy TEXT NOT NULL,          -- mapping | comment | inferred
        status TEXT NOT NULL,            -- planned | applied | failed | unchanged
        reason TEXT,
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
        updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS llm_cache (
        id INTEGER PRIMARY KEY,
        commit_hash TEXT NOT NULL,
        doc_file TEXT NOT NULL,
        section_id TEXT NOT NULL,
        provider TEXT NOT NULL,
        model TEXT NOT NULL,
        prompt_hash TEXT NOT NULL,
        response_text TEXT NOT NULL,
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
        UNIQUE(commit_hash, doc_file, section_id, provider, model, prompt_hash)
);

CREATE TABLE IF NOT EXISTS run_events (
        id INTEGER PRIMARY KEY,
        run_id TEXT NOT NULL,
        commit_hash TEXT,
        level TEXT NOT NULL,             -- info | warn | error
        component TEXT NOT NULL,
        message TEXT NOT NULL,
        metadata TEXT,
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

Notes:

- Keep existing tables unchanged for backward compatibility.
- Use migrations with monotonic versions in `.git-doc/migrations/`.

## 13. End-to-End Testing Blueprint (Detailed)

### 13.1 Unit Test Packages

- `internal/config`: defaults, env expansion, validation errors.
- `internal/git`: argument building, parsing git outputs, stderr propagation.
- `internal/diff`: hunk parsing, large diff summarization, path filtering.
- `internal/doc`: heading/comment section extraction, replacement boundaries, line-ending preservation.
- `internal/llm`: provider adapter contract tests with fake HTTP server.
- `internal/state`: migrations, CRUD, transaction rollback behavior.
- `internal/orchestrator`: commit workflow state transitions with mocked dependencies.

### 13.2 Integration Tests

Use temp repos (`t.TempDir`) to validate realistic flows:

- `init` creates `.git-doc/` with secure permissions.
- `update` processes ordered commits and generates doc commits.
- `retry` reprocesses only failed commits.
- `revert` reverts associated doc commits correctly.
- hook scripts trigger update and respect lock file.
- large-diff commit follows summarization path.

### 13.3 Contract and Golden Tests

- Golden files for markdown section update input/output.
- Golden fixtures for diff parsing and prompt assembly.
- Provider contract tests validating request/response mapping.

### 13.4 Quality Gates

- `go test ./...`
- `go test -race ./...` (Linux at minimum)
- Coverage gate (start 65%, target 80% over time)
- `go vet ./...` and `staticcheck ./...`
- Optional `govulncheck ./...` in CI security job

## 14. CI/CD Pipeline Design

### 14.1 CI Workflows (GitHub Actions)

1. **PR Fast Checks (`ci.yml`)**
     - Trigger: pull requests + pushes to main.
     - Jobs:
         - lint (`gofmt -l`, `go vet`, `staticcheck`)
         - unit tests
         - minimal integration smoke test
2. **Cross-Platform Matrix (`test-matrix.yml`)**
     - Trigger: PR label or push to main.
     - Matrix:
         - `ubuntu-latest`, `macos-latest`, `windows-latest`
         - Go `1.21.x`, optional `1.22.x`
     - Jobs:
         - integration suite with temporary repos.
3. **Nightly Resilience (`nightly.yml`)**
     - Trigger: schedule.
     - Jobs:
         - large repo simulation
         - repeated retry/failover scenarios
         - hook race-condition checks
4. **Security Scan (`security.yml`)**
     - Trigger: PR + nightly.
     - Jobs:
         - `govulncheck`
         - `gosec`
         - dependency update checks (Dependabot or Renovate)

### 14.2 Release Workflow (`release.yml`)

- Trigger: tags (`v*.*.*`).
- Steps:
    - run full tests
    - build multi-platform artifacts
    - generate checksums
    - sign artifacts (Cosign or GPG)
    - generate changelog from conventional commits
    - publish GitHub Release

## 15. Packaging & Distribution Plan

### 15.1 Build and Release Tooling

- Use **GoReleaser** for reproducible cross-platform binaries.
- Initial targets:
    - Linux: `amd64`, `arm64`
    - macOS: `amd64`, `arm64`
    - Windows: `amd64` (add `arm64` later)

### 15.2 Channels

- GitHub Releases (primary).
- Homebrew tap (macOS/Linux convenience).
- Scoop manifest (Windows convenience).
- Optional package outputs later: `.deb`, `.rpm`.

### 15.3 Artifact Set Per Release

- `git-doc_<version>_<os>_<arch>.tar.gz` (or `.zip` for Windows)
- `checksums.txt`
- signature files (`.sig`)
- SBOM (CycloneDX or SPDX)

### 15.4 Versioning

- SemVer (`MAJOR.MINOR.PATCH`).
- Conventional commits for changelog automation.
- Pre-releases (`-rc.1`) for stabilization before major launches.

## 16. Operational Observability

- Add structured logs (`json` optional) with run ID.
- Add `git-doc status --json` for tooling integration.
- Track metrics in state metadata:
    - tokens in/out
    - provider/model
    - latency per target
    - cache hit ratio

## 17. Recommended Phased Execution Plan

### Phase 1 (MVP Core)

- `init`, `update`, config loading, OpenAI client, markdown section replacement.
- processed state tracking, doc commit creation, `--dry-run`.

### Phase 2 (Reliability)

- two-phase planning/apply, lock file, retries, failed commit recovery, cache.
- `status`, `retry`, robust validation guardrails.

### Phase 3 (Git Integration)

- hook enable/disable with backup/restore.
- `revert` command with state-linked doc commit revert.

### Phase 4 (Scale & Distribution)

- multi-provider support and fallback.
- cross-platform matrix CI.
- signed releases + Homebrew/Scoop distribution.

### Phase 5 (Hardening)

- security workflows, SBOM/provenance, nightly resilience suite.
- stricter coverage thresholds and performance regression tests.

## 18. Release Playbook (Operational)

This section defines the practical release process for maintainers so each version is reproducible, signed, and easy to verify.

### 18.1 Tag Format and Triggers

- Use SemVer tags as the release trigger:
    - Stable: `vX.Y.Z` (example: `v1.2.0`)
    - Pre-release: `vX.Y.Z-rc.N` (example: `v1.2.0-rc.1`)
- Pushing a matching tag triggers `release.yml`.
- `ci.yml` and `security.yml` should already be green on `main` before tagging.

### 18.2 Required GitHub Permissions

For release automation to work reliably, the workflow token must have:

- `contents: write` (create GitHub Releases, upload artifacts)
- `packages: write` (future package publishing compatibility)
- `id-token: write` (keyless signing with Cosign)

Recommended repository settings:

- Allow GitHub Actions to create and approve pull requests only where required.
- Keep branch protections on `main` (required status checks + linear history or squash policy, as desired).

### 18.3 First Release Checklist

1. Validate repository health locally:
    - run tests and ensure formatting is clean.
2. Confirm workflows exist and are valid:
    - `ci.yml`, `security.yml`, `nightly.yml`, `release.yml`.
3. Confirm release config exists and is current:
    - `.goreleaser.yml` includes builds, checksums, SBOMs, and signing.
4. Ensure version/tag target is final:
    - changelog notes ready, no pending breaking changes.
5. Create and push the release tag:
    - `git tag v0.1.0`
    - `git push origin v0.1.0`
6. Verify GitHub Release output:
    - multi-platform binaries uploaded
    - `checksums.txt` uploaded
    - signature and certificate artifacts present
    - SBOM artifact present
7. Perform quick smoke verification:
    - download one artifact per major OS family and run `git-doc version`.

### 18.4 Post-Release Follow-ups

- Announce release notes and notable migration steps (if any).
- Monitor first 24 hours for install/runtime regressions.
- If a critical issue appears, ship `vX.Y.(Z+1)` patch instead of replacing artifacts.

## 19. Rollback & Incident Playbook

This section defines the response process when a release is broken, artifacts are invalid, or an urgent security/runtime fix is needed.

### 19.1 Failed Release Run Handling

If `release.yml` fails for a tag:

1. Identify failure stage (build, checksum/sign, publish).
2. Fix on `main` using a normal PR + green `ci.yml` and `security.yml`.
3. Do not overwrite or mutate existing release artifacts.
4. Create a new patch tag for the corrected release (example: move from `v1.4.0` attempt to `v1.4.1`).

### 19.2 Broken Artifact or Signature Response

If uploaded binaries/checksums/signatures are invalid or unverifiable:

1. Mark the affected GitHub Release as pre-release or add a prominent warning note.
2. Investigate root cause (cross-compile mismatch, signing identity issue, checksum generation error).
3. Regenerate and republish via a new version tag; avoid replacing files in-place.
4. Publish a short incident note with impact, affected versions, and upgrade target.

### 19.3 Security or Critical Runtime Incident

For critical vulnerabilities or severe runtime breakage:

1. Open an incident branch from latest stable tag.
2. Apply minimal fix scope only (no refactors, no feature work).
3. Run full tests + security checks before release tag.
4. Release emergency patch as `vX.Y.(Z+1)`.
5. Backport to active maintenance lines if multiple supported minors exist.

### 19.4 Emergency Patch Checklist

1. Reproduce issue and document exact scope.
2. Add/adjust regression test that fails before fix and passes after fix.
3. Merge fix with highest review priority.
4. Tag and publish patch release.
5. Validate install + `git-doc version` smoke test on at least one Linux and one macOS runner.
6. Communicate timeline, mitigation, and required user action.

### 19.5 Rollback Communication Template (Short)

- What happened: concise failure statement.
- Impact: who is affected and how.
- Action: version to avoid and version to upgrade to.
- Verification: command(s) users run to confirm healthy install.
- Follow-up: ETA for detailed postmortem.

### 19.6 Owner + SLA Matrix

| Incident Activity | Primary Owner | Backup Owner | Target SLA |
|---|---|---|---|
| Acknowledge production release incident | Release Maintainer | On-call Engineer | ≤ 15 minutes |
| Triage severity and blast radius | On-call Engineer | Release Maintainer | ≤ 30 minutes |
| Publish initial user advisory | Release Maintainer | Product/Repo Admin | ≤ 60 minutes |
| Prepare and validate emergency patch | On-call Engineer | Component Maintainer | ≤ 4 hours |
| Cut and publish patched release tag | Release Maintainer | Repo Admin | ≤ 6 hours |
| Post-release verification and sign-off | QA/Reviewer | On-call Engineer | ≤ 8 hours |
| Publish postmortem summary | Release Maintainer | Product/Repo Admin | ≤ 48 hours |

Notes:

- For low-severity issues, SLAs can be relaxed by explicit maintainer agreement.
- For security incidents, follow the fastest path and prioritize patch publication over broad refactoring.

### 19.7 Severity Definitions (SEV-1/2/3)

- **SEV-1 (Critical):** release is unusable for most users, severe security risk, or major data-loss/corruption risk.
    - Response mode: immediate incident command, emergency patch path.
    - Target: acknowledge ≤ 15 min, patch publication target ≤ 6 hours.
- **SEV-2 (High):** major feature regression or install/runtime failure affecting a significant subset of users.
    - Response mode: accelerated hotfix flow.
    - Target: acknowledge ≤ 30 min, patch publication target ≤ 24 hours.
- **SEV-3 (Moderate):** limited-impact bug with acceptable workaround and no critical security impact.
    - Response mode: standard patch planning.
    - Target: acknowledge ≤ 1 business day, patch in next scheduled patch release.

Escalation rule:

- If impact expands during investigation, severity must be upgraded immediately and SLA targets reset to the higher tier.

---

## Conclusion

This architecture provides a solid foundation for building `git-doc`. It is modular, robust, and cross-platform, addressing the core goals of automated documentation updates with LLM assistance and deep Git integration. Start by implementing the core `update` flow with a single LLM provider (OpenAI), then gradually add hooks, state management, and multi-provider support. Good luck!
