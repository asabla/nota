package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/emilsoderling/nota/internal/extractors"
	"github.com/spf13/cobra"
)

var (
	extractAll    bool
	extractOutput string
	extractTool   string
)

var extractCmd = &cobra.Command{
	Use:   "extract",
	Short: "Extract AI transcripts without git integration",
	Long: `Extract AI coding assistant transcripts from their storage locations.

This command finds and parses transcripts from Claude Code, OpenCode, and Codex
without writing to git notes. Useful for inspection and debugging.`,
	RunE: runExtract,
}

func init() {
	extractCmd.Flags().BoolVarP(&extractAll, "all", "a", false, "extract all sessions, not just for current repo")
	extractCmd.Flags().StringVarP(&extractOutput, "output", "o", "", "output format: json, table (default: table)")
	extractCmd.Flags().StringVarP(&extractTool, "tool", "t", "", "only extract from specific tool: claude-code, opencode, codex")
	rootCmd.AddCommand(extractCmd)
}

func runExtract(cmd *cobra.Command, args []string) error {
	registry := extractors.DefaultRegistry()

	var allSessions []extractors.SessionInfo

	repoPath := getRepoPath()
	absRepoPath, err := filepath.Abs(repoPath)
	if err != nil {
		return fmt.Errorf("failed to resolve repo path: %w", err)
	}

	for _, e := range registry.All() {
		// Filter by tool if specified
		if extractTool != "" && e.Name() != extractTool {
			continue
		}

		var sessions []extractors.SessionInfo
		var err error

		if extractAll {
			rawSessions, err := e.ExtractAll()
			if err != nil {
				if verbose {
					fmt.Fprintf(os.Stderr, "Warning: %s extractor failed: %v\n", e.Name(), err)
				}
				continue
			}
			for _, s := range rawSessions {
				sessions = append(sessions, extractors.SessionInfo{Session: s, Extractor: e.Name()})
			}
		} else {
			rawSessions, err := e.Extract(absRepoPath)
			if err != nil {
				if verbose {
					fmt.Fprintf(os.Stderr, "Warning: %s extractor failed: %v\n", e.Name(), err)
				}
				continue
			}
			for _, s := range rawSessions {
				sessions = append(sessions, extractors.SessionInfo{Session: s, Extractor: e.Name()})
			}
		}

		if err != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "Warning: %s extractor failed: %v\n", e.Name(), err)
			}
			continue
		}

		allSessions = append(allSessions, sessions...)
	}

	// Output results
	if extractOutput == "json" {
		return outputJSON(allSessions)
	}
	return outputTable(allSessions, absRepoPath)
}

func outputJSON(sessions []extractors.SessionInfo) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(sessions)
}

func outputTable(sessions []extractors.SessionInfo, repoPath string) error {
	if len(sessions) == 0 {
		fmt.Println("No sessions found.")
		if !extractAll {
			fmt.Printf("Hint: use --all to search all repositories, or check if transcripts exist for: %s\n", repoPath)
		}
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TOOL\tSESSION ID\tMESSAGES\tTOKENS (IN/OUT)\tSTART TIME\tDIRECTORY")
	fmt.Fprintln(w, "----\t----------\t--------\t---------------\t----------\t---------")

	for _, info := range sessions {
		s := info.Session
		sessionID := s.ID
		if len(sessionID) > 12 {
			sessionID = sessionID[:12] + "..."
		}

		dir := s.WorkingDirectory
		if len(dir) > 40 {
			dir = "..." + dir[len(dir)-37:]
		}

		startTime := ""
		if !s.StartTime.IsZero() {
			startTime = s.StartTime.Format("2006-01-02 15:04")
		}

		fmt.Fprintf(w, "%s\t%s\t%d\t%d/%d\t%s\t%s\n",
			s.Tool,
			sessionID,
			len(s.Messages),
			s.InputTokens,
			s.OutputTokens,
			startTime,
			dir,
		)
	}

	w.Flush()
	fmt.Printf("\nTotal: %d sessions\n", len(sessions))
	return nil
}

// getRepoPath returns the repository path from flag or current directory
func getRepoPath() string {
	if repoPath != "" {
		return repoPath
	}
	// Default to current directory
	return "."
}
