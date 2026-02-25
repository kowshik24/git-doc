package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/kowshik24/git-doc/internal/config"
	"github.com/kowshik24/git-doc/internal/doc"
	"github.com/kowshik24/git-doc/internal/gitutil"
	"github.com/kowshik24/git-doc/internal/hooks"
	"github.com/kowshik24/git-doc/internal/llm"
	"github.com/kowshik24/git-doc/internal/orchestrator"
	"github.com/kowshik24/git-doc/internal/runlock"
	"github.com/kowshik24/git-doc/internal/state"
)

const version = "0.1.0"

type rootFlags struct {
	configPath string
	dryRun     bool
	verbose    bool
}

func NewRootCmd() *cobra.Command {
	flags := &rootFlags{}
	cmd := &cobra.Command{
		Use:   "git-doc",
		Short: "Automatically update docs based on Git commits",
	}

	cmd.PersistentFlags().StringVar(&flags.configPath, "config", ".git-doc/config.toml", "Path to config file")
	cmd.PersistentFlags().BoolVar(&flags.dryRun, "dry-run", false, "Preview changes without applying or committing")
	cmd.PersistentFlags().BoolVar(&flags.verbose, "verbose", false, "Enable verbose logging")

	cmd.AddCommand(newInitCmd())
	cmd.AddCommand(newConfigCmd(flags))
	cmd.AddCommand(newUpdateCmd(flags))
	cmd.AddCommand(newEnableHookCmd())
	cmd.AddCommand(newDisableHookCmd())
	cmd.AddCommand(newStatusCmd(flags))
	cmd.AddCommand(newRetryCmd(flags))
	cmd.AddCommand(newRevertCmd(flags))
	cmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Show version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(version)
		},
	})

	return cmd
}

func newConfigCmd(flags *rootFlags) *cobra.Command {
	var edit bool
	var showPath bool

	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show or edit git-doc configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, err := gitutil.GetRepoRoot()
			if err != nil {
				return err
			}

			configPath := flags.configPath
			if !filepath.IsAbs(configPath) {
				configPath = filepath.Join(repoRoot, configPath)
			}

			if showPath {
				fmt.Println(configPath)
				return nil
			}

			if edit {
				editor := strings.TrimSpace(os.Getenv("VISUAL"))
				if editor == "" {
					editor = strings.TrimSpace(os.Getenv("EDITOR"))
				}
				if editor == "" {
					return fmt.Errorf("no editor configured; set VISUAL or EDITOR")
				}

				parts := strings.Fields(editor)
				parts = append(parts, configPath)
				editorCmd := exec.Command(parts[0], parts[1:]...)
				editorCmd.Stdin = os.Stdin
				editorCmd.Stdout = os.Stdout
				editorCmd.Stderr = os.Stderr
				return editorCmd.Run()
			}

			b, err := os.ReadFile(configPath)
			if err != nil {
				return err
			}
			fmt.Print(string(b))
			return nil
		},
	}

	cmd.Flags().BoolVar(&edit, "edit", false, "Open configuration file in editor")
	cmd.Flags().BoolVar(&showPath, "path", false, "Print resolved configuration file path")
	return cmd
}

func newEnableHookCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable-hook",
		Short: "Install git-doc hooks (post-commit, post-merge, post-rewrite)",
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, err := gitutil.GetRepoRoot()
			if err != nil {
				return err
			}

			mgr := hooks.NewManager(repoRoot)
			if err := mgr.Enable(); err != nil {
				return err
			}

			fmt.Println("git hooks enabled")
			return nil
		},
	}
}

func newDisableHookCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable-hook",
		Short: "Remove git-doc hooks and restore backups if available",
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, err := gitutil.GetRepoRoot()
			if err != nil {
				return err
			}

			mgr := hooks.NewManager(repoRoot)
			if err := mgr.Disable(); err != nil {
				return err
			}

			fmt.Println("git hooks disabled")
			return nil
		},
	}
}

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize .git-doc config and state directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, err := gitutil.GetRepoRoot()
			if err != nil {
				return err
			}

			gitDocDir := filepath.Join(repoRoot, ".git-doc")
			if err := os.MkdirAll(gitDocDir, 0o700); err != nil {
				return fmt.Errorf("create .git-doc dir: %w", err)
			}

			configPath := filepath.Join(gitDocDir, "config.toml")
			if _, statErr := os.Stat(configPath); errors.Is(statErr, os.ErrNotExist) {
				if err := os.WriteFile(configPath, []byte(config.DefaultToml()), 0o600); err != nil {
					return fmt.Errorf("write config: %w", err)
				}
			}

			fmt.Printf("Initialized git-doc at %s\n", gitDocDir)
			return nil
		},
	}
}

func newUpdateCmd(flags *rootFlags) *cobra.Command {
	var fromHook bool
	var fromHash string
	var toHash string

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Process new commits and update documentation",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := buildApp(flags)
			if err != nil {
				return err
			}

			lock, err := runlock.Acquire(app.RepoRoot)
			if err != nil {
				if fromHook && runlock.IsAlreadyRunningError(err) {
					return nil
				}
				return err
			}
			defer lock.Release()

			var summary orchestrator.Summary
			if strings.TrimSpace(fromHash) != "" || strings.TrimSpace(toHash) != "" {
				summary, err = app.Updater.UpdateRangeCommits(cmd.Context(), fromHash, toHash, flags.dryRun)
			} else {
				summary, err = app.Updater.UpdateNewCommits(cmd.Context(), flags.dryRun)
			}
			if err != nil {
				return err
			}

			fmt.Printf("processed=%d success=%d failed=%d skipped=%d\n", summary.Processed, summary.Success, summary.Failed, summary.Skipped)
			return nil
		},
	}

	cmd.Flags().BoolVar(&fromHook, "from-hook", false, "Internal: run invoked from git hook")
	cmd.Flags().StringVar(&fromHash, "from", "", "Start commit (exclusive) for manual range updates")
	cmd.Flags().StringVar(&toHash, "to", "", "End commit (inclusive, default HEAD) for manual range updates")
	_ = cmd.Flags().MarkHidden("from-hook")
	return cmd
}

func newStatusCmd(flags *rootFlags) *cobra.Command {
	var asJSON bool
	var limit int

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show state of processed commits",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := buildApp(flags)
			if err != nil {
				return err
			}

			rows, err := app.State.ListRecent(limit)
			if err != nil {
				return err
			}

			counts, err := app.State.GetStatusCounts()
			if err != nil {
				return err
			}

			if asJSON {
				type statusRow struct {
					CommitHash  string `json:"commit_hash"`
					Status      string `json:"status"`
					ProcessedAt string `json:"processed_at"`
					Error       string `json:"error,omitempty"`
					DocCommit   string `json:"doc_commit_hash,omitempty"`
				}

				payloadRows := make([]statusRow, 0, len(rows))
				for _, row := range rows {
					entry := statusRow{
						CommitHash:  row.CommitHash,
						Status:      row.Status,
						ProcessedAt: row.ProcessedAt.Format(time.RFC3339),
					}
					if row.Error.Valid {
						entry.Error = row.Error.String
					}
					if row.DocCommit.Valid {
						entry.DocCommit = row.DocCommit.String
					}
					payloadRows = append(payloadRows, entry)
				}

				payload := map[string]any{
					"generated_at": time.Now().UTC().Format(time.RFC3339),
					"counts":       counts,
					"recent":       payloadRows,
				}

				out, err := json.MarshalIndent(payload, "", "  ")
				if err != nil {
					return err
				}
				fmt.Println(string(out))
				return nil
			}

			fmt.Printf("pending=%d in_progress=%d success=%d failed=%d skipped=%d total=%d\n",
				counts.Pending, counts.InProgress, counts.Success, counts.Failed, counts.Skipped, counts.Total)

			for _, row := range rows {
				fmt.Printf("%s %s %s\n", row.CommitHash, row.Status, row.ProcessedAt.Format("2006-01-02 15:04:05"))
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output status as JSON")
	cmd.Flags().IntVar(&limit, "limit", 25, "Maximum number of recent commit rows")
	return cmd
}

func newRetryCmd(flags *rootFlags) *cobra.Command {
	var specificCommit string

	cmd := &cobra.Command{
		Use:   "retry",
		Short: "Retry failed commits",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := buildApp(flags)
			if err != nil {
				return err
			}

			lock, err := runlock.Acquire(app.RepoRoot)
			if err != nil {
				return err
			}
			defer lock.Release()

			var commits []string
			if specificCommit != "" {
				commits = []string{specificCommit}
			} else {
				commits, err = app.State.GetRetryableCommits()
				if err != nil {
					return err
				}
			}

			summary, err := app.Updater.UpdateCommitList(cmd.Context(), commits, flags.dryRun)
			if err != nil {
				return err
			}

			fmt.Printf("retried=%d success=%d failed=%d skipped=%d\n", summary.Processed, summary.Success, summary.Failed, summary.Skipped)
			return nil
		},
	}

	cmd.Flags().StringVar(&specificCommit, "commit", "", "Retry specific commit hash")
	return cmd
}

func newRevertCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "revert <code-commit-hash>",
		Short: "Revert documentation commit linked to a code commit",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := buildApp(flags)
			if err != nil {
				return err
			}

			codeCommit := args[0]
			docCommit, err := app.State.GetDocCommitHash(codeCommit)
			if err != nil {
				return err
			}
			if docCommit == "" {
				return fmt.Errorf("no documentation commit found for code commit %s", codeCommit)
			}

			if flags.dryRun {
				fmt.Printf("dry-run: would revert doc commit %s (for code commit %s)\n", docCommit, codeCommit)
				return nil
			}

			if err := app.Git.RevertCommit(docCommit); err != nil {
				return err
			}

			fmt.Printf("reverted doc commit %s\n", docCommit)
			return nil
		},
	}
}

type appContainer struct {
	Updater  *orchestrator.Updater
	State    *state.Store
	Git      gitutil.Helper
	RepoRoot string
}

func buildApp(flags *rootFlags) (*appContainer, error) {
	repoRoot, err := gitutil.GetRepoRoot()
	if err != nil {
		return nil, err
	}

	configPath := flags.configPath
	if !filepath.IsAbs(configPath) {
		configPath = filepath.Join(repoRoot, configPath)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}

	statePath := cfg.State.DBPath
	if !filepath.IsAbs(statePath) {
		statePath = filepath.Join(repoRoot, statePath)
	}

	store, err := state.New(statePath)
	if err != nil {
		return nil, err
	}

	gitClient := gitutil.NewHelper(repoRoot)
	docUpdater := doc.NewMarkdownUpdater()
	llmClient, err := llm.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	updater := orchestrator.NewUpdater(orchestrator.Dependencies{
		Config:     cfg,
		Git:        gitClient,
		State:      store,
		DocUpdater: docUpdater,
		LLM:        llmClient,
	})

	return &appContainer{Updater: updater, State: store, Git: gitClient, RepoRoot: repoRoot}, nil
}
