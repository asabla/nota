package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var extractCmd = &cobra.Command{
	Use:   "extract",
	Short: "Extract AI transcripts without git integration",
	Long: `Extract AI coding assistant transcripts from their storage locations.

This command finds and parses transcripts from Claude Code, OpenCode, and Codex
without writing to git notes. Useful for inspection and debugging.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Implement extraction logic
		fmt.Println("Extracting transcripts...")
		fmt.Printf("Repository: %s\n", getRepoPath())
		return nil
	},
}

func init() {
	rootCmd.AddCommand(extractCmd)
}

// getRepoPath returns the repository path from flag or current directory
func getRepoPath() string {
	if repoPath != "" {
		return repoPath
	}
	// Default to current directory
	return "."
}
