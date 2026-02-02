package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var notesCmd = &cobra.Command{
	Use:   "notes",
	Short: "Manage AI transcript git notes",
	Long:  `Commands for listing, exporting, and managing AI transcript notes stored in git.`,
}

var notesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all commits with AI transcript notes",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Implement notes listing
		fmt.Println("Listing AI-linked commits...")
		return nil
	},
}

var exportFormat string

var notesExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export AI transcript notes to JSON or CSV",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Implement notes export
		fmt.Printf("Exporting notes as %s...\n", exportFormat)
		return nil
	},
}

func init() {
	notesExportCmd.Flags().StringVarP(&exportFormat, "format", "f", "json", "export format (json, csv)")

	notesCmd.AddCommand(notesListCmd)
	notesCmd.AddCommand(notesExportCmd)
	rootCmd.AddCommand(notesCmd)
}
