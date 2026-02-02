package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var commitSHA string

var postCommitCmd = &cobra.Command{
	Use:   "post-commit",
	Short: "Extract and inject transcripts for a commit (called from git hook)",
	Long: `Extract AI transcripts and inject metadata into git notes for a commit.

This command is designed to be called from a post-commit git hook. It:
1. Extracts recent sessions from all supported AI assistants
2. Matches sessions to the commit based on timing and file overlap
3. Writes metadata to git notes under refs/notes/ai-transcripts`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Implement post-commit logic
		fmt.Printf("Processing commit: %s\n", commitSHA)
		fmt.Printf("Repository: %s\n", getRepoPath())
		return nil
	},
}

func init() {
	postCommitCmd.Flags().StringVar(&commitSHA, "commit", "", "commit SHA to process (defaults to HEAD)")
	rootCmd.AddCommand(postCommitCmd)
}
