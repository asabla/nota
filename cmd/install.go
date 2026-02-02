package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install git hooks for automatic transcript tracking",
	Long: `Install a post-commit hook that automatically extracts and links
AI transcripts to commits.

The hook is a thin shell wrapper that calls 'nota post-commit'.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Implement hook installation
		fmt.Printf("Installing hooks in: %s\n", getRepoPath())
		return nil
	},
}

func init() {
	rootCmd.AddCommand(installCmd)
}
