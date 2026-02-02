package cmd

import (
	"fmt"

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
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Implement web server
		fmt.Printf("Starting server on http://localhost:%d\n", servePort)
		return nil
	},
}

func init() {
	serveCmd.Flags().IntVarP(&servePort, "port", "p", 8080, "port to listen on")
	rootCmd.AddCommand(serveCmd)
}
