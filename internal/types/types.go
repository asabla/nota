// Package types defines the core data structures used throughout nota.
package types

import "time"

// Session represents an AI coding assistant session extracted from transcript files.
type Session struct {
	// ID is the unique identifier for this session (format varies by tool)
	ID string `json:"id"`

	// Tool identifies the AI assistant that created this session
	// Valid values: "claude-code", "opencode", "codex"
	Tool string `json:"tool"`

	// WorkingDirectory is the repository/project path where the session occurred
	WorkingDirectory string `json:"working_directory"`

	// Messages contains the conversation history
	Messages []Message `json:"messages"`

	// Token usage metrics
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	CacheTokens  int `json:"cache_tokens,omitempty"`

	// Timing information
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`

	// SourcePath is the original transcript file path
	SourcePath string `json:"source_path"`

	// Model is the AI model used (if available)
	Model string `json:"model,omitempty"`

	// GitBranch is the branch name at the time of the session (if available)
	GitBranch string `json:"git_branch,omitempty"`

	// FilesTouched contains files that were read/written during the session
	FilesTouched []string `json:"files_touched,omitempty"`
}

// Message represents a single message in a conversation.
type Message struct {
	// Role is either "user" or "assistant"
	Role string `json:"role"`

	// Content is the text content of the message
	Content string `json:"content"`

	// Timestamp when this message was sent
	Timestamp time.Time `json:"timestamp"`

	// ToolCalls contains any tool invocations in this message (for assistant messages)
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// ToolCall represents a tool invocation by the AI assistant.
type ToolCall struct {
	// Name of the tool (e.g., "read_file", "write_file", "bash")
	Name string `json:"name"`

	// Input contains the tool's input parameters
	Input map[string]interface{} `json:"input,omitempty"`

	// Output contains the tool's result (if captured)
	Output string `json:"output,omitempty"`
}

// TokenMetrics contains token usage information.
type TokenMetrics struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	CacheRead    int `json:"cache_read,omitempty"`
	CacheWrite   int `json:"cache_write,omitempty"`
	DurationMs   int `json:"duration_ms,omitempty"`
}

// NoteMetadata is the JSON structure stored in git notes.
// It provides a compact summary of an AI session linked to a commit.
type NoteMetadata struct {
	// Version for schema evolution
	Version int `json:"v"`

	// Timestamp when this note was created
	Timestamp time.Time `json:"ts"`

	// Tool that generated this session
	Tool string `json:"tool"`

	// SessionID links back to the original session
	SessionID string `json:"session_id"`

	// Model used (if known)
	Model string `json:"model,omitempty"`

	// Metrics summarizes token usage
	Metrics TokenMetrics `json:"metrics"`

	// FilesChanged lists files modified in the linked commit
	FilesChanged []string `json:"files_changed,omitempty"`

	// TranscriptPath points to the original transcript file
	TranscriptPath string `json:"transcript_path"`

	// MatchScore indicates confidence in session-to-commit matching (0-1)
	MatchScore float64 `json:"match_score,omitempty"`
}

// ScoredSession pairs a session with its match score for commit matching.
type ScoredSession struct {
	Session Session
	Score   float64
}
