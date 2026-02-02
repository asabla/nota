package cmd

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"text/tabwriter"

	"github.com/emilsoderling/nota/internal/git"
	"github.com/emilsoderling/nota/internal/types"
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
	RunE:  runNotesList,
}

var exportFormat string

var notesExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export AI transcript notes to JSON or CSV",
	RunE:  runNotesExport,
}

func init() {
	notesExportCmd.Flags().StringVarP(&exportFormat, "format", "f", "json", "export format (json, csv)")

	notesCmd.AddCommand(notesListCmd)
	notesCmd.AddCommand(notesExportCmd)
	rootCmd.AddCommand(notesCmd)
}

func runNotesList(cmd *cobra.Command, args []string) error {
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

	notes, err := git.NewNotes(repoRoot)
	if err != nil {
		return fmt.Errorf("failed to initialize git notes: %w", err)
	}

	notesList, err := notes.List()
	if err != nil {
		return fmt.Errorf("failed to list notes: %w", err)
	}

	if len(notesList) == 0 {
		fmt.Println("No AI transcript notes found.")
		fmt.Println("Hint: Use 'nota post-commit' to link sessions to commits, or 'nota install' for automatic tracking.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "COMMIT\tTOOL\tSESSION\tTOKENS (IN/OUT)\tSCORE\tTIMESTAMP")
	fmt.Fprintln(w, "------\t----\t-------\t---------------\t-----\t---------")

	for commitSHA, meta := range notesList {
		shortCommit := commitSHA
		if len(shortCommit) > 8 {
			shortCommit = shortCommit[:8]
		}

		shortSession := meta.SessionID
		if len(shortSession) > 12 {
			shortSession = shortSession[:12] + "..."
		}

		ts := ""
		if !meta.Timestamp.IsZero() {
			ts = meta.Timestamp.Format("2006-01-02 15:04")
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%d/%d\t%.2f\t%s\n",
			shortCommit,
			meta.Tool,
			shortSession,
			meta.Metrics.InputTokens,
			meta.Metrics.OutputTokens,
			meta.MatchScore,
			ts,
		)
	}

	w.Flush()
	fmt.Printf("\nTotal: %d notes\n", len(notesList))
	return nil
}

func runNotesExport(cmd *cobra.Command, args []string) error {
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

	notes, err := git.NewNotes(repoRoot)
	if err != nil {
		return fmt.Errorf("failed to initialize git notes: %w", err)
	}

	notesList, err := notes.List()
	if err != nil {
		return fmt.Errorf("failed to list notes: %w", err)
	}

	switch exportFormat {
	case "json":
		return exportNotesJSON(notesList)
	case "csv":
		return exportNotesCSV(notesList)
	default:
		return fmt.Errorf("unknown format: %s (use json or csv)", exportFormat)
	}
}

func exportNotesJSON(notesList map[string]types.NoteMetadata) error {
	// Convert to a more export-friendly format with commit SHA included
	type exportEntry struct {
		Commit string             `json:"commit"`
		Note   types.NoteMetadata `json:"note"`
	}

	var entries []exportEntry
	for commitSHA, note := range notesList {
		entries = append(entries, exportEntry{
			Commit: commitSHA,
			Note:   note,
		})
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(entries)
}

func exportNotesCSV(notesList map[string]types.NoteMetadata) error {
	w := csv.NewWriter(os.Stdout)
	defer w.Flush()

	// Write header
	if err := w.Write([]string{"commit", "tool", "session_id", "model", "input_tokens", "output_tokens", "match_score", "timestamp", "transcript_path"}); err != nil {
		return err
	}

	for commitSHA, note := range notesList {
		row := []string{
			commitSHA,
			note.Tool,
			note.SessionID,
			note.Model,
			strconv.Itoa(note.Metrics.InputTokens),
			strconv.Itoa(note.Metrics.OutputTokens),
			strconv.FormatFloat(note.MatchScore, 'f', 2, 64),
			note.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
			note.TranscriptPath,
		}

		if err := w.Write(row); err != nil {
			return err
		}
	}

	return nil
}
