package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/emilsoderling/nota/internal/extractors"
	"github.com/emilsoderling/nota/internal/git"
	"github.com/emilsoderling/nota/internal/types"
	"github.com/spf13/cobra"
)

var (
	commitSHA string
	dryRun    bool
)

var postCommitCmd = &cobra.Command{
	Use:   "post-commit",
	Short: "Extract and inject transcripts for a commit (called from git hook)",
	Long: `Extract AI transcripts and inject metadata into git notes for a commit.

This command is designed to be called from a post-commit git hook. It:
1. Extracts recent sessions from all supported AI assistants
2. Matches sessions to the commit based on timing and file overlap
3. Writes metadata to git notes under refs/notes/ai-transcripts`,
	RunE: runPostCommit,
}

func init() {
	postCommitCmd.Flags().StringVar(&commitSHA, "commit", "", "commit SHA to process (defaults to HEAD)")
	postCommitCmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be done without writing notes")
	rootCmd.AddCommand(postCommitCmd)
}

func runPostCommit(cmd *cobra.Command, args []string) error {
	// Get repo path
	repoPath := getRepoPath()
	absRepoPath, err := filepath.Abs(repoPath)
	if err != nil {
		return fmt.Errorf("failed to resolve repo path: %w", err)
	}

	// Get repo root
	repoRoot, err := git.GetRepoRoot(absRepoPath)
	if err != nil {
		return fmt.Errorf("failed to get repo root: %w", err)
	}

	// Create notes instance
	notes, err := git.NewNotes(repoRoot)
	if err != nil {
		return fmt.Errorf("failed to initialize git notes: %w", err)
	}

	// Get commit SHA
	sha := commitSHA
	if sha == "" {
		sha, err = notes.GetHeadSHA()
		if err != nil {
			return fmt.Errorf("failed to get HEAD: %w", err)
		}
	}

	// Get commit info
	commitInfo, err := notes.GetCommitInfo(sha)
	if err != nil {
		return fmt.Errorf("failed to get commit info: %w", err)
	}

	if verbose {
		fmt.Printf("Processing commit: %s\n", sha[:8])
		fmt.Printf("  Subject: %s\n", commitInfo.Subject)
		fmt.Printf("  Files: %d\n", len(commitInfo.Files))
	}

	// Extract sessions from all tools
	registry := extractors.DefaultRegistry()
	var allSessions []types.Session

	for _, e := range registry.All() {
		sessions, err := e.Extract(repoRoot)
		if err != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "Warning: %s extractor failed: %v\n", e.Name(), err)
			}
			continue
		}
		allSessions = append(allSessions, sessions...)
	}

	if verbose {
		fmt.Printf("Found %d sessions for this repository\n", len(allSessions))
	}

	// Match sessions to commit
	config := git.DefaultMatchConfig()
	matches := git.MatchSessionsToCommit(allSessions, commitInfo, repoRoot, config)

	if len(matches) == 0 {
		if verbose {
			fmt.Println("No matching sessions found for this commit")
		}
		return nil
	}

	// Take the best match
	best := matches[0]

	if verbose {
		fmt.Printf("Best match: %s (score: %.2f)\n", best.Session.ID, best.Score)
		for _, reason := range best.Reasons {
			fmt.Printf("  - %s\n", reason)
		}
	}

	// Create note metadata
	metadata := types.NoteMetadata{
		Version:   1,
		Timestamp: time.Now(),
		Tool:      best.Session.Tool,
		SessionID: best.Session.ID,
		Model:     best.Session.Model,
		Metrics: types.TokenMetrics{
			InputTokens:  best.Session.InputTokens,
			OutputTokens: best.Session.OutputTokens,
			CacheRead:    best.Session.CacheTokens,
		},
		FilesChanged:   commitInfo.Files,
		TranscriptPath: best.Session.SourcePath,
		MatchScore:     best.Score,
	}

	if dryRun {
		fmt.Printf("Would add note to commit %s:\n", sha[:8])
		fmt.Printf("  Tool: %s\n", metadata.Tool)
		fmt.Printf("  Session: %s\n", metadata.SessionID)
		fmt.Printf("  Tokens: %d in / %d out\n", metadata.Metrics.InputTokens, metadata.Metrics.OutputTokens)
		fmt.Printf("  Match score: %.2f\n", metadata.MatchScore)
		return nil
	}

	// Add the note
	if err := notes.Add(sha, metadata); err != nil {
		return fmt.Errorf("failed to add note: %w", err)
	}

	fmt.Printf("Linked session %s to commit %s (score: %.2f)\n",
		best.Session.ID[:12]+"...", sha[:8], best.Score)

	return nil
}
