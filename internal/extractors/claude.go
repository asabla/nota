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

// ClaudeExtractor extracts sessions from Claude Code transcript files.
// Claude Code stores conversations in ~/.claude/projects/{encoded-path}/{session-uuid}.jsonl
type ClaudeExtractor struct {
	basePath string
}

// claudeEntry represents a single line in the Claude Code JSONL file.
type claudeEntry struct {
	Type        string        `json:"type"`
	UUID        string        `json:"uuid"`
	ParentUUID  *string       `json:"parentUuid"`
	SessionID   string        `json:"sessionId"`
	Timestamp   string        `json:"timestamp"`
	Cwd         string        `json:"cwd"`
	IsSidechain bool          `json:"isSidechain"`
	IsMeta      bool          `json:"isMeta"`
	GitBranch   string        `json:"gitBranch"`
	Version     string        `json:"version"`
	Message     claudeMessage `json:"message"`
}

// claudeMessage represents the message content in a Claude entry.
type claudeMessage struct {
	Role    string       `json:"role"`
	Content interface{}  `json:"content"` // Can be string or []claudeContentBlock
	Usage   *claudeUsage `json:"usage"`
}

// claudeContentBlock represents a content block when content is an array.
type claudeContentBlock struct {
	Type  string      `json:"type"`
	Text  string      `json:"text"`
	Name  string      `json:"name"`  // For tool_use
	Input interface{} `json:"input"` // For tool_use
}

// claudeUsage represents token usage information.
type claudeUsage struct {
	InputTokens          int `json:"input_tokens"`
	OutputTokens         int `json:"output_tokens"`
	CacheReadInputTokens int `json:"cache_read_input_tokens"`
	CacheCreationTokens  int `json:"cache_creation_input_tokens"`
}

// NewClaudeExtractor creates a new Claude Code extractor.
func NewClaudeExtractor() *ClaudeExtractor {
	return &ClaudeExtractor{
		basePath: filepath.Join(getHomeDir(), ".claude", "projects"),
	}
}

// Name returns the extractor identifier.
func (e *ClaudeExtractor) Name() string {
	return "claude-code"
}

// Extract finds sessions associated with the given repository path.
func (e *ClaudeExtractor) Extract(repoPath string) ([]types.Session, error) {
	// Resolve to absolute path
	absRepoPath, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, err
	}

	// Encode the repo path: /Users/emil/project -> -Users-emil-project
	encodedPath := encodeClaudePath(absRepoPath)
	projectDir := filepath.Join(e.basePath, encodedPath)

	// Check if directory exists
	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		return []types.Session{}, nil // No sessions for this repo
	}

	return e.extractFromDir(projectDir, absRepoPath)
}

// ExtractAll finds all sessions regardless of repository.
func (e *ClaudeExtractor) ExtractAll() ([]types.Session, error) {
	var allSessions []types.Session

	// Walk all project directories
	entries, err := os.ReadDir(e.basePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []types.Session{}, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		projectDir := filepath.Join(e.basePath, entry.Name())
		repoPath := decodeClaudePath(entry.Name())

		sessions, err := e.extractFromDir(projectDir, repoPath)
		if err != nil {
			continue // Skip problematic directories
		}
		allSessions = append(allSessions, sessions...)
	}

	return allSessions, nil
}

// extractFromDir extracts sessions from a specific project directory.
func (e *ClaudeExtractor) extractFromDir(projectDir, repoPath string) ([]types.Session, error) {
	var sessions []types.Session

	// Find all JSONL files (excluding agent-*.jsonl which are subagent files)
	files, err := filepath.Glob(filepath.Join(projectDir, "*.jsonl"))
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		basename := filepath.Base(file)

		// Skip subagent files
		if strings.HasPrefix(basename, "agent-") {
			continue
		}

		session, err := e.parseSessionFile(file, repoPath)
		if err != nil {
			continue // Skip problematic files
		}

		if session != nil && len(session.Messages) > 0 {
			sessions = append(sessions, *session)
		}
	}

	return sessions, nil
}

// parseSessionFile parses a single JSONL session file.
func (e *ClaudeExtractor) parseSessionFile(path, repoPath string) (*types.Session, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	session := &types.Session{
		Tool:             "claude-code",
		WorkingDirectory: repoPath,
		SourcePath:       path,
		Messages:         make([]types.Message, 0),
	}

	scanner := bufio.NewScanner(file)
	// Increase buffer size for potentially large lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var totalInput, totalOutput, totalCache int

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry claudeEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue // Skip malformed lines
		}

		// Skip non-message entries
		if entry.Type != "user" && entry.Type != "assistant" {
			continue
		}

		// Skip sidechain (subagent context) messages
		if entry.IsSidechain {
			continue
		}

		// Skip meta messages (system context)
		if entry.IsMeta {
			continue
		}

		// Set session ID from first entry
		if session.ID == "" && entry.SessionID != "" {
			session.ID = entry.SessionID
		}

		// Set git branch if available
		if session.GitBranch == "" && entry.GitBranch != "" {
			session.GitBranch = entry.GitBranch
		}

		// Parse timestamp
		ts, err := time.Parse(time.RFC3339, entry.Timestamp)
		if err != nil {
			ts = time.Time{}
		}

		// Update session time bounds
		if session.StartTime.IsZero() || ts.Before(session.StartTime) {
			session.StartTime = ts
		}
		if ts.After(session.EndTime) {
			session.EndTime = ts
		}

		// Extract message content
		content := extractClaudeContent(entry.Message.Content)
		toolCalls := extractClaudeToolCalls(entry.Message.Content)

		// Update token counts (take the latest, as they're cumulative)
		if entry.Message.Usage != nil {
			totalInput = entry.Message.Usage.InputTokens
			totalOutput = entry.Message.Usage.OutputTokens
			totalCache = entry.Message.Usage.CacheReadInputTokens
		}

		msg := types.Message{
			Role:      entry.Message.Role,
			Content:   content,
			Timestamp: ts,
			ToolCalls: toolCalls,
		}

		session.Messages = append(session.Messages, msg)
	}

	session.InputTokens = totalInput
	session.OutputTokens = totalOutput
	session.CacheTokens = totalCache

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return session, nil
}

// extractClaudeContent extracts text content from the message content field.
func extractClaudeContent(content interface{}) string {
	switch v := content.(type) {
	case string:
		return v
	case []interface{}:
		var parts []string
		for _, item := range v {
			if block, ok := item.(map[string]interface{}); ok {
				if text, ok := block["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

// extractClaudeToolCalls extracts tool calls from the message content.
func extractClaudeToolCalls(content interface{}) []types.ToolCall {
	var toolCalls []types.ToolCall

	items, ok := content.([]interface{})
	if !ok {
		return toolCalls
	}

	for _, item := range items {
		block, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		blockType, _ := block["type"].(string)
		if blockType != "tool_use" {
			continue
		}

		name, _ := block["name"].(string)
		input, _ := block["input"].(map[string]interface{})

		toolCalls = append(toolCalls, types.ToolCall{
			Name:  name,
			Input: input,
		})
	}

	return toolCalls
}

// encodeClaudePath converts a filesystem path to Claude's encoded format.
// /Users/emil/project -> -Users-emil-project
func encodeClaudePath(path string) string {
	// Replace path separators with dashes
	return strings.ReplaceAll(path, string(os.PathSeparator), "-")
}

// decodeClaudePath converts Claude's encoded path back to a filesystem path.
// -Users-emil-project -> /Users/emil/project
func decodeClaudePath(encoded string) string {
	// Replace leading dash with path separator, then remaining dashes
	if strings.HasPrefix(encoded, "-") {
		return string(os.PathSeparator) + strings.ReplaceAll(encoded[1:], "-", string(os.PathSeparator))
	}
	return strings.ReplaceAll(encoded, "-", string(os.PathSeparator))
}
