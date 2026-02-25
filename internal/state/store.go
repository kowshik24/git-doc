package state

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type ProcessedCommitRow struct {
	CommitHash  string
	ProcessedAt time.Time
	Status      string
	Error       sql.NullString
	DocCommit   sql.NullString
}

type StatusCounts struct {
	Pending    int `json:"pending"`
	InProgress int `json:"in_progress"`
	Success    int `json:"success"`
	Failed     int `json:"failed"`
	Skipped    int `json:"skipped"`
	Total      int `json:"total"`
}

type LLMCacheEntry struct {
	CommitHash string
	DocFile    string
	SectionID  string
	Provider   string
	Model      string
	PromptHash string
	Response   string
}

func New(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS processed_commits (
			commit_hash TEXT PRIMARY KEY,
			processed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			status TEXT CHECK(status IN ('pending', 'in_progress', 'success', 'failed', 'skipped')),
			error TEXT,
			doc_commit_hash TEXT,
			doc_files_changed TEXT,
			metadata TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS mappings (
			id INTEGER PRIMARY KEY,
			code_commit_hash TEXT,
			doc_file TEXT,
			section TEXT,
			FOREIGN KEY(code_commit_hash) REFERENCES processed_commits(commit_hash)
		);`,
		`CREATE TABLE IF NOT EXISTS planned_updates (
			id INTEGER PRIMARY KEY,
			commit_hash TEXT NOT NULL,
			doc_file TEXT NOT NULL,
			section_id TEXT NOT NULL,
			strategy TEXT NOT NULL,
			status TEXT NOT NULL,
			reason TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(commit_hash, doc_file, section_id)
		);`,
		`CREATE TABLE IF NOT EXISTS llm_cache (
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
		);`,
		`CREATE TABLE IF NOT EXISTS run_events (
			id INTEGER PRIMARY KEY,
			run_id TEXT NOT NULL,
			commit_hash TEXT,
			level TEXT NOT NULL,
			component TEXT NOT NULL,
			message TEXT NOT NULL,
			metadata TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
	}

	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	if err := s.ensureProcessedCommitSchema(); err != nil {
		return err
	}

	return nil
}

func (s *Store) ensureProcessedCommitSchema() error {
	row := s.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='processed_commits'`)
	var tableSQL string
	if err := row.Scan(&tableSQL); err != nil {
		return err
	}

	if strings.Contains(tableSQL, "'pending'") && strings.Contains(tableSQL, "'in_progress'") {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmts := []string{
		`CREATE TABLE processed_commits_new (
			commit_hash TEXT PRIMARY KEY,
			processed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			status TEXT CHECK(status IN ('pending', 'in_progress', 'success', 'failed', 'skipped')),
			error TEXT,
			doc_commit_hash TEXT,
			doc_files_changed TEXT,
			metadata TEXT
		);`,
		`INSERT INTO processed_commits_new (commit_hash, processed_at, status, error, doc_commit_hash, doc_files_changed, metadata)
		 SELECT commit_hash, processed_at, status, error, doc_commit_hash, doc_files_changed, metadata
		 FROM processed_commits;`,
		`DROP TABLE processed_commits;`,
		`ALTER TABLE processed_commits_new RENAME TO processed_commits;`,
	}

	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) GetLastProcessedCommit() (string, error) {
	row := s.db.QueryRow(`SELECT commit_hash FROM processed_commits WHERE status='success' ORDER BY processed_at DESC LIMIT 1`)
	var hash string
	if err := row.Scan(&hash); err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	return hash, nil
}

func (s *Store) MarkCommitProcessed(commitHash, status, errText, docCommit string, filesChanged []string) error {
	filesJSON := "[]"
	if filesChanged != nil {
		b, err := json.Marshal(filesChanged)
		if err != nil {
			return err
		}
		filesJSON = string(b)
	}

	_, err := s.db.Exec(`
	INSERT INTO processed_commits (commit_hash, status, error, doc_commit_hash, doc_files_changed)
	VALUES (?, ?, ?, ?, ?)
	ON CONFLICT(commit_hash) DO UPDATE SET
		processed_at = CURRENT_TIMESTAMP,
		status = excluded.status,
		error = excluded.error,
		doc_commit_hash = excluded.doc_commit_hash,
		doc_files_changed = excluded.doc_files_changed
	`, commitHash, status, nullIfEmpty(errText), nullIfEmpty(docCommit), filesJSON)
	if err != nil {
		return fmt.Errorf("mark commit processed: %w", err)
	}

	return nil
}

func (s *Store) GetFailedCommits() ([]string, error) {
	rows, err := s.db.Query(`SELECT commit_hash FROM processed_commits WHERE status='failed' ORDER BY processed_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var hash string
		if scanErr := rows.Scan(&hash); scanErr != nil {
			return nil, scanErr
		}
		out = append(out, hash)
	}
	return out, rows.Err()
}

func (s *Store) GetRetryableCommits() ([]string, error) {
	rows, err := s.db.Query(`
		SELECT commit_hash
		FROM processed_commits
		WHERE status IN ('failed', 'in_progress')
		ORDER BY processed_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var hash string
		if scanErr := rows.Scan(&hash); scanErr != nil {
			return nil, scanErr
		}
		out = append(out, hash)
	}

	return out, rows.Err()
}

func (s *Store) GetResumableCommits() ([]string, error) {
	rows, err := s.db.Query(`
		SELECT commit_hash
		FROM processed_commits
		WHERE status IN ('pending', 'in_progress')
		ORDER BY processed_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var hash string
		if scanErr := rows.Scan(&hash); scanErr != nil {
			return nil, scanErr
		}
		out = append(out, hash)
	}

	return out, rows.Err()
}

func (s *Store) StoreMapping(commitHash, docFile, section string) error {
	_, err := s.db.Exec(`INSERT INTO mappings (code_commit_hash, doc_file, section) VALUES (?, ?, ?)`, commitHash, docFile, section)
	return err
}

func (s *Store) GetDocCommitHash(codeCommitHash string) (string, error) {
	row := s.db.QueryRow(`SELECT COALESCE(doc_commit_hash, '') FROM processed_commits WHERE commit_hash = ? LIMIT 1`, codeCommitHash)
	var hash string
	if err := row.Scan(&hash); err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	return hash, nil
}

func (s *Store) ListRecent(limit int) ([]ProcessedCommitRow, error) {
	if limit <= 0 {
		limit = 25
	}

	rows, err := s.db.Query(`
		SELECT commit_hash, processed_at, status, COALESCE(error, ''), COALESCE(doc_commit_hash, '')
		FROM processed_commits
		ORDER BY processed_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ProcessedCommitRow, 0, limit)
	for rows.Next() {
		var row ProcessedCommitRow
		var errStr string
		var docCommit string
		if scanErr := rows.Scan(&row.CommitHash, &row.ProcessedAt, &row.Status, &errStr, &docCommit); scanErr != nil {
			return nil, scanErr
		}
		if errStr != "" {
			row.Error = sql.NullString{String: errStr, Valid: true}
		}
		if docCommit != "" {
			row.DocCommit = sql.NullString{String: docCommit, Valid: true}
		}
		out = append(out, row)
	}

	return out, rows.Err()
}

func (s *Store) GetStatusCounts() (StatusCounts, error) {
	rows, err := s.db.Query(`
		SELECT status, COUNT(*)
		FROM processed_commits
		GROUP BY status
	`)
	if err != nil {
		return StatusCounts{}, err
	}
	defer rows.Close()

	counts := StatusCounts{}
	for rows.Next() {
		var status string
		var count int
		if scanErr := rows.Scan(&status, &count); scanErr != nil {
			return StatusCounts{}, scanErr
		}

		switch status {
		case "pending":
			counts.Pending = count
		case "in_progress":
			counts.InProgress = count
		case "success":
			counts.Success = count
		case "failed":
			counts.Failed = count
		case "skipped":
			counts.Skipped = count
		}
	}

	counts.Total = counts.Pending + counts.InProgress + counts.Success + counts.Failed + counts.Skipped
	return counts, rows.Err()
}

func (s *Store) UpsertPlannedUpdate(commitHash, docFile, sectionID, strategy, status, reason string) error {
	_, err := s.db.Exec(`
	INSERT INTO planned_updates (commit_hash, doc_file, section_id, strategy, status, reason)
	VALUES (?, ?, ?, ?, ?, ?)
	ON CONFLICT(commit_hash, doc_file, section_id) DO UPDATE SET
		strategy = excluded.strategy,
		status = excluded.status,
		reason = excluded.reason,
		updated_at = CURRENT_TIMESTAMP
	`, commitHash, docFile, sectionID, strategy, status, nullIfEmpty(reason))
	return err
}

func (s *Store) GetCachedLLMResponse(commitHash, docFile, sectionID, provider, model, prompt string) (string, bool, error) {
	promptHash := hashPrompt(prompt)
	row := s.db.QueryRow(`
		SELECT response_text
		FROM llm_cache
		WHERE commit_hash = ? AND doc_file = ? AND section_id = ? AND provider = ? AND model = ? AND prompt_hash = ?
		LIMIT 1
	`, commitHash, docFile, sectionID, provider, model, promptHash)

	var response string
	if err := row.Scan(&response); err != nil {
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		return "", false, err
	}

	return response, true, nil
}

func (s *Store) PutCachedLLMResponse(entry LLMCacheEntry) error {
	if entry.PromptHash == "" {
		return fmt.Errorf("prompt hash is required for llm cache entry")
	}

	_, err := s.db.Exec(`
	INSERT INTO llm_cache (commit_hash, doc_file, section_id, provider, model, prompt_hash, response_text)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(commit_hash, doc_file, section_id, provider, model, prompt_hash) DO UPDATE SET
		response_text = excluded.response_text
	`, entry.CommitHash, entry.DocFile, entry.SectionID, entry.Provider, entry.Model, entry.PromptHash, entry.Response)
	return err
}

func (s *Store) LogRunEvent(runID, commitHash, level, component, message string, metadata map[string]any) error {
	metadataJSON := ""
	if metadata != nil {
		b, err := json.Marshal(metadata)
		if err != nil {
			return err
		}
		metadataJSON = string(b)
	}

	_, err := s.db.Exec(`
	INSERT INTO run_events (run_id, commit_hash, level, component, message, metadata)
	VALUES (?, ?, ?, ?, ?, ?)
	`, runID, nullIfEmpty(commitHash), level, component, message, nullIfEmpty(metadataJSON))
	return err
}

func hashPrompt(prompt string) string {
	sum := sha256.Sum256([]byte(prompt))
	return fmt.Sprintf("%x", sum)
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
