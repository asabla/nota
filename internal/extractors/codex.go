package extractors

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/emilsoderling/nota/internal/types"
)

// CodexExtractor extracts sessions from OpenAI Codex CLI transcript files.
// Codex stores sessions in ~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl
type CodexExtractor struct {
	basePath string
}

// codexEntry represents a single line in the Codex JSONL file.
type codexEntry struct {
	Timestamp string      `json:"timestamp"`
	Type      string      `json:"type"`
	Payload   interface{} `json:"payload"`
}

// codexSessionMeta represents the session_meta payload.
type codexSessionMeta struct {
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Cwd       string `json:"cwd"`
	Model     string `json:"model_provider"`
	Version   string `json:"cli_version"`
	Git       struct {
		CommitHash    string `json:"commit_hash"`
		Branch        string `json:"branch"`
		RepositoryURL string `json:"repository_url"`
	} `json:"git"`
}

// codexResponseItem represents a response_item payload.
type codexResponseItem struct {
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// codexTurnContext represents a turn_context payload.
type codexTurnContext struct {
	Model string `json:"model"`
	Cwd   string `json:"cwd"`
}

// NewCodexExtractor creates a new Codex extractor.
func NewCodexExtractor() *CodexExtractor {
	basePath := filepath.Join(getHomeDir(), ".codex", "sessions")

	// Check for override via environment variable
	if customPath := os.Getenv("CODEX_HOME"); customPath != "" {
		basePath = filepath.Join(customPath, "sessions")
	}

	return &CodexExtractor{
		basePath: basePath,
	}
}

// Name returns the extractor identifier.
func (e *CodexExtractor) Name() string {
	return "codex"
}

// Extract finds sessions associated with the given repository path.
func (e *CodexExtractor) Extract(repoPath string) ([]types.Session, error) {
	// Resolve to absolute path
	absRepoPath, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, err
	}

	allSessions, err := e.ExtractAll()
	if err != nil {
		return nil, err
	}

	// Filter to sessions matching this repo
	var filtered []types.Session
	for _, session := range allSessions {
		if session.WorkingDirectory == absRepoPath ||
			strings.HasPrefix(session.WorkingDirectory, absRepoPath+string(os.PathSeparator)) {
			filtered = append(filtered, session)
		}
	}

	return filtered, nil
}

// ExtractAll finds all sessions regardless of repository.
func (e *CodexExtractor) ExtractAll() ([]types.Session, error) {
	var allSessions []types.Session

	// Check if directory exists
	if _, err := os.Stat(e.basePath); os.IsNotExist(err) {
		return []types.Session{}, nil
	}

	// Find all rollout JSONL files: sessions/YYYY/MM/DD/rollout-*.jsonl
	pattern := filepath.Join(e.basePath, "*", "*", "*", "rollout-*.jsonl")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		session, err := e.parseSessionFile(file)
		if err != nil {
			continue
		}
		if session != nil && len(session.Messages) > 0 {
			allSessions = append(allSessions, *session)
		}
	}

	return allSessions, nil
}

// parseSessionFile parses a single Codex JSONL session file.
func (e *CodexExtractor) parseSessionFile(path string) (*types.Session, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	session := &types.Session{
		Tool:       "codex",
		SourcePath: path,
		Messages:   make([]types.Message, 0),
	}

	scanner := bufio.NewScanner(file)
	// Increase buffer size for potentially large lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry codexEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		// Parse timestamp
		ts, _ := time.Parse(time.RFC3339, entry.Timestamp)

		// Update session time bounds
		if session.StartTime.IsZero() || ts.Before(session.StartTime) {
			session.StartTime = ts
		}
		if ts.After(session.EndTime) {
			session.EndTime = ts
		}

		switch entry.Type {
		case "session_meta":
			e.parseSessionMeta(session, entry.Payload)
		case "response_item":
			if msg := e.parseResponseItem(entry.Payload, ts); msg != nil {
				session.Messages = append(session.Messages, *msg)
			}
		case "turn_context":
			e.parseTurnContext(session, entry.Payload)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Extract session ID from filename if not set
	if session.ID == "" {
		session.ID = extractCodexSessionID(path)
	}

	return session, nil
}

// parseSessionMeta extracts session metadata from the session_meta entry.
func (e *CodexExtractor) parseSessionMeta(session *types.Session, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}

	var meta codexSessionMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return
	}

	if meta.ID != "" {
		session.ID = meta.ID
	}
	if meta.Cwd != "" {
		session.WorkingDirectory = meta.Cwd
	}
	if meta.Git.Branch != "" {
		session.GitBranch = meta.Git.Branch
	}
}

// parseResponseItem extracts a message from a response_item entry.
func (e *CodexExtractor) parseResponseItem(payload interface{}, ts time.Time) *types.Message {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil
	}

	var item codexResponseItem
	if err := json.Unmarshal(data, &item); err != nil {
		return nil
	}

	// Only process message type response items
	if item.Type != "message" {
		return nil
	}

	// Extract text content
	var textParts []string
	for _, block := range item.Content {
		if block.Type == "input_text" || block.Type == "output_text" || block.Type == "text" {
			if block.Text != "" {
				textParts = append(textParts, block.Text)
			}
		}
	}

	if len(textParts) == 0 {
		return nil
	}

	return &types.Message{
		Role:      item.Role,
		Content:   strings.Join(textParts, "\n"),
		Timestamp: ts,
	}
}

// parseTurnContext extracts model info from turn_context entries.
func (e *CodexExtractor) parseTurnContext(session *types.Session, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}

	var ctx codexTurnContext
	if err := json.Unmarshal(data, &ctx); err != nil {
		return
	}

	if ctx.Model != "" && session.Model == "" {
		session.Model = ctx.Model
	}
	if ctx.Cwd != "" && session.WorkingDirectory == "" {
		session.WorkingDirectory = ctx.Cwd
	}
}

// extractCodexSessionID extracts the session ID from a Codex session file path.
// rollout-2025-11-03T07-33-17-019a486b-840a-7282-8cdd-d103a9c4d0a2.jsonl
func extractCodexSessionID(path string) string {
	basename := filepath.Base(path)
	// Remove "rollout-" prefix and ".jsonl" suffix
	basename = strings.TrimPrefix(basename, "rollout-")
	basename = strings.TrimSuffix(basename, ".jsonl")
	return basename
}
