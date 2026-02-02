package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/emilsoderling/nota/internal/git"
	"github.com/spf13/cobra"
)

var uninstall bool

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install git hooks for automatic transcript tracking",
	Long: `Install a post-commit hook that automatically extracts and links
AI transcripts to commits.

The hook is a thin shell wrapper that calls 'nota post-commit'.`,
	RunE: runInstall,
}

func init() {
	installCmd.Flags().BoolVar(&uninstall, "uninstall", false, "remove the hook instead of installing")
	rootCmd.AddCommand(installCmd)
}

func runInstall(cmd *cobra.Command, args []string) error {
	repoPath := getRepoPath()
	absRepoPath, err := filepath.Abs(repoPath)
	if err != nil {
		return fmt.Errorf("failed to resolve repo path: %w", err)
	}

	// Get repo root
	repoRoot, err := git.GetRepoRoot(absRepoPath)
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}

	if uninstall {
		if err := git.UninstallHook(repoRoot); err != nil {
			return fmt.Errorf("failed to uninstall hook: %w", err)
		}
		fmt.Println("Hook uninstalled successfully")
		return nil
	}

	// Check if already installed
	if git.IsHookInstalled(repoRoot) {
		fmt.Println("Hook is already installed")
		return nil
	}

	if err := git.InstallHook(repoRoot); err != nil {
		return fmt.Errorf("failed to install hook: %w", err)
	}

	fmt.Println("Hook installed successfully")
	fmt.Println("AI transcripts will now be automatically linked to commits")
	return nil
}
