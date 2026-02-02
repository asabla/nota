package cmd

import (
	"fmt"

	"github.com/emilsoderling/nota/internal/viewer"
	"github.com/spf13/cobra"
)

var servePort int

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the HTMX web viewer",
	Long: `Start a local web server that displays AI transcript history.

The viewer provides:
- Timeline view of AI-assisted commits
- Expandable transcript details
- Search and filter functionality
- Real-time updates via HTMX`,
	RunE: runServe,
}

func init() {
	serveCmd.Flags().IntVarP(&servePort, "port", "p", 8080, "port to listen on")
	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	repoPath := getRepoPath()

	server, err := viewer.NewServer(repoPath)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	return server.Start(servePort)
}
