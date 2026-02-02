// Package viewer provides an HTMX-powered web interface for browsing
// AI transcript history linked to git commits.
package viewer

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/emilsoderling/nota/internal/extractors"
	"github.com/emilsoderling/nota/internal/git"
	"github.com/emilsoderling/nota/internal/types"
)

//go:embed templates/*.html
var templateFS embed.FS

// Server is the HTMX web viewer server.
type Server struct {
	repoPath  string
	notes     *git.Notes
	templates *template.Template
	registry  *extractors.Registry
}

// LogEntry represents a single entry in the timeline view.
type LogEntry struct {
	CommitSHA    string
	CommitShort  string
	Subject      string
	CommitTime   time.Time
	Tool         string
	SessionID    string
	SessionShort string
	Model        string
	InputTokens  int
	OutputTokens int
	CacheTokens  int
	MatchScore   float64
	FilesChanged []string
	HasNote      bool
}

// TranscriptView represents the expanded transcript view.
type TranscriptView struct {
	Session  types.Session
	Messages []MessageView
}

// MessageView represents a single message in the transcript.
type MessageView struct {
	Role        string
	Content     string
	Timestamp   string
	ToolCalls   []ToolCallView
	IsUser      bool
	ContentHTML template.HTML
}

// ToolCallView represents a tool call in the transcript.
type ToolCallView struct {
	Name   string
	Input  string
	Output string
}

// NewServer creates a new viewer server.
func NewServer(repoPath string) (*Server, error) {
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, err
	}

	repoRoot, err := git.GetRepoRoot(absPath)
	if err != nil {
		return nil, fmt.Errorf("not a git repository: %w", err)
	}

	notes, err := git.NewNotes(repoRoot)
	if err != nil {
		return nil, err
	}

	// Parse templates
	funcMap := template.FuncMap{
		"truncate": truncate,
		"timeAgo":  timeAgo,
		"formatTime": func(t time.Time) string {
			return t.Format("2006-01-02 15:04:05")
		},
		"json": func(v interface{}) string {
			b, _ := json.MarshalIndent(v, "", "  ")
			return string(b)
		},
		"add": func(a, b int) int {
			return a + b
		},
		"sub": func(a, b int) int {
			return a - b
		},
		"mul": func(a, b float64) float64 {
			return a * b
		},
		"toolColor": toolColor,
	}

	tmpl, err := template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %w", err)
	}

	return &Server{
		repoPath:  repoRoot,
		notes:     notes,
		templates: tmpl,
		registry:  extractors.DefaultRegistry(),
	}, nil
}

// Start starts the HTTP server.
func (s *Server) Start(port int) error {
	mux := http.NewServeMux()

	// Routes
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/logs", s.handleLogs)
	mux.HandleFunc("/transcript/", s.handleTranscript)
	mux.HandleFunc("/search", s.handleSearch)
	mux.HandleFunc("/stats", s.handleStats)

	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("Starting nota viewer at http://localhost%s\n", addr)
	return http.ListenAndServe(addr, mux)
}

// handleIndex serves the main page.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	entries, err := s.getLogEntries("", 20)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	stats := s.getStats()

	data := map[string]interface{}{
		"Entries":  entries,
		"Stats":    stats,
		"RepoPath": s.repoPath,
	}

	s.render(w, "index.html", data)
}

// handleLogs serves paginated log entries (for infinite scroll).
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	after := r.URL.Query().Get("after")
	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	entries, err := s.getLogEntries(after, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// If HTMX request, return partial
	if r.Header.Get("HX-Request") == "true" {
		s.render(w, "log-entries.html", map[string]interface{}{
			"Entries": entries,
		})
		return
	}

	// Otherwise return full page
	s.handleIndex(w, r)
}

// handleTranscript serves the expanded transcript view.
func (s *Server) handleTranscript(w http.ResponseWriter, r *http.Request) {
	// Extract session ID from path: /transcript/{sessionID}
	sessionID := strings.TrimPrefix(r.URL.Path, "/transcript/")
	if sessionID == "" {
		http.Error(w, "session ID required", http.StatusBadRequest)
		return
	}

	// Find the session
	session, err := s.findSession(sessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	view := s.buildTranscriptView(session)

	s.render(w, "transcript.html", view)
}

// handleSearch handles search queries.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		s.handleLogs(w, r)
		return
	}

	entries, err := s.searchEntries(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.render(w, "log-entries.html", map[string]interface{}{
		"Entries": entries,
		"Query":   query,
	})
}

// handleStats returns statistics.
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats := s.getStats()
	s.render(w, "stats.html", stats)
}

// getLogEntries retrieves log entries with pagination.
func (s *Server) getLogEntries(afterCommit string, limit int) ([]LogEntry, error) {
	// Get all notes
	notesList, err := s.notes.List()
	if err != nil {
		return nil, err
	}

	// Get recent commits with notes
	var entries []LogEntry

	for commitSHA, note := range notesList {
		info, err := s.notes.GetCommitInfo(commitSHA)
		if err != nil {
			continue
		}

		entry := LogEntry{
			CommitSHA:    commitSHA,
			CommitShort:  truncate(commitSHA, 8),
			Subject:      info.Subject,
			CommitTime:   info.Timestamp,
			Tool:         note.Tool,
			SessionID:    note.SessionID,
			SessionShort: truncate(note.SessionID, 12),
			Model:        note.Model,
			InputTokens:  note.Metrics.InputTokens,
			OutputTokens: note.Metrics.OutputTokens,
			CacheTokens:  note.Metrics.CacheRead,
			MatchScore:   note.MatchScore,
			FilesChanged: note.FilesChanged,
			HasNote:      true,
		}

		entries = append(entries, entry)
	}

	// Sort by commit time descending
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].CommitTime.After(entries[j].CommitTime)
	})

	// Apply pagination
	startIdx := 0
	if afterCommit != "" {
		for i, e := range entries {
			if e.CommitSHA == afterCommit {
				startIdx = i + 1
				break
			}
		}
	}

	endIdx := startIdx + limit
	if endIdx > len(entries) {
		endIdx = len(entries)
	}

	if startIdx >= len(entries) {
		return []LogEntry{}, nil
	}

	return entries[startIdx:endIdx], nil
}

// searchEntries searches for entries matching the query.
func (s *Server) searchEntries(query string) ([]LogEntry, error) {
	entries, err := s.getLogEntries("", 1000)
	if err != nil {
		return nil, err
	}

	query = strings.ToLower(query)
	var matched []LogEntry

	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Subject), query) ||
			strings.Contains(strings.ToLower(e.Tool), query) ||
			strings.Contains(strings.ToLower(e.SessionID), query) ||
			strings.Contains(strings.ToLower(e.CommitSHA), query) {
			matched = append(matched, e)
		}
	}

	return matched, nil
}

// findSession finds a session by ID.
func (s *Server) findSession(sessionID string) (*types.Session, error) {
	for _, e := range s.registry.All() {
		sessions, err := e.ExtractAll()
		if err != nil {
			continue
		}
		for _, session := range sessions {
			if session.ID == sessionID {
				return &session, nil
			}
		}
	}
	return nil, fmt.Errorf("session not found: %s", sessionID)
}

// buildTranscriptView builds the view model for a transcript.
func (s *Server) buildTranscriptView(session *types.Session) TranscriptView {
	view := TranscriptView{
		Session:  *session,
		Messages: make([]MessageView, 0, len(session.Messages)),
	}

	for _, msg := range session.Messages {
		mv := MessageView{
			Role:        msg.Role,
			Content:     msg.Content,
			Timestamp:   msg.Timestamp.Format("15:04:05"),
			IsUser:      msg.Role == "user",
			ContentHTML: template.HTML(formatContent(msg.Content)),
		}

		for _, tc := range msg.ToolCalls {
			inputJSON, _ := json.MarshalIndent(tc.Input, "", "  ")
			mv.ToolCalls = append(mv.ToolCalls, ToolCallView{
				Name:   tc.Name,
				Input:  string(inputJSON),
				Output: tc.Output,
			})
		}

		view.Messages = append(view.Messages, mv)
	}

	return view
}

// getStats returns aggregate statistics.
func (s *Server) getStats() map[string]interface{} {
	notesList, _ := s.notes.List()

	var totalInput, totalOutput int
	toolCounts := make(map[string]int)

	for _, note := range notesList {
		totalInput += note.Metrics.InputTokens
		totalOutput += note.Metrics.OutputTokens
		toolCounts[note.Tool]++
	}

	return map[string]interface{}{
		"TotalCommits": len(notesList),
		"TotalInput":   totalInput,
		"TotalOutput":  totalOutput,
		"TotalTokens":  totalInput + totalOutput,
		"ToolCounts":   toolCounts,
	}
}

// render renders a template.
func (s *Server) render(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
		fmt.Fprintf(os.Stderr, "template error: %v\n", err)
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// Helper functions

func truncate(s string, length int) string {
	if len(s) <= length {
		return s
	}
	return s[:length]
}

func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	case d < 7*24*time.Hour:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "yesterday"
		}
		return fmt.Sprintf("%d days ago", days)
	default:
		return t.Format("Jan 2, 2006")
	}
}

func toolColor(tool string) string {
	switch tool {
	case "claude-code":
		return "#D97706" // Amber/orange
	case "opencode":
		return "#059669" // Emerald/green
	case "codex":
		return "#7C3AED" // Violet/purple
	default:
		return "#6B7280" // Gray
	}
}

func formatContent(content string) string {
	// Basic markdown-like formatting
	// Escape HTML first
	content = template.HTMLEscapeString(content)

	// Convert newlines to <br>
	content = strings.ReplaceAll(content, "\n", "<br>")

	// Code blocks (simplified)
	// This is a very basic implementation

	return content
}
