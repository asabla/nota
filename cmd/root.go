// Package cmd implements the CLI commands for nota.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	// Used for flags
	repoPath string
	verbose  bool
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "nota",
	Short: "Track AI coding assistant activity in git repositories",
	Long: `nota extracts transcripts from AI coding assistants (Claude Code, OpenCode, Codex)
and links them to git commits using git notes.

It provides visibility into how AI-assisted changes flow into your codebase,
with a web viewer for exploring the history.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVarP(&repoPath, "repo", "r", "", "repository path (defaults to current directory)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
}
