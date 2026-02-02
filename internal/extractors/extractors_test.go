package extractors

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestClaudePathEncoding tests Claude's path encoding/decoding.
func TestClaudePathEncoding(t *testing.T) {
	tests := []struct {
		original string
		encoded  string
	}{
		{"/Users/emil/project", "-Users-emil-project"},
		{"/home/user/code/app", "-home-user-code-app"},
		{"/tmp/test", "-tmp-test"},
	}

	for _, tt := range tests {
		t.Run(tt.original, func(t *testing.T) {
			encoded := encodeClaudePath(tt.original)
			if encoded != tt.encoded {
				t.Errorf("encodeClaudePath(%q) = %q, want %q", tt.original, encoded, tt.encoded)
			}

			decoded := decodeClaudePath(tt.encoded)
			if decoded != tt.original {
				t.Errorf("decodeClaudePath(%q) = %q, want %q", tt.encoded, decoded, tt.original)
			}
		})
	}
}

// TestExtractClaudeContent tests content extraction from different formats.
func TestExtractClaudeContent(t *testing.T) {
	tests := []struct {
		name    string
		content interface{}
		want    string
	}{
		{
			name:    "string content",
			content: "Hello, world!",
			want:    "Hello, world!",
		},
		{
			name: "array with text blocks",
			content: []interface{}{
				map[string]interface{}{"type": "text", "text": "First part"},
				map[string]interface{}{"type": "text", "text": "Second part"},
			},
			want: "First part\nSecond part",
		},
		{
			name: "array with mixed blocks",
			content: []interface{}{
				map[string]interface{}{"type": "text", "text": "Some text"},
				map[string]interface{}{"type": "tool_use", "name": "write", "input": map[string]interface{}{}},
			},
			want: "Some text",
		},
		{
			name:    "nil content",
			content: nil,
			want:    "",
		},
		{
			name:    "empty array",
			content: []interface{}{},
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractClaudeContent(tt.content)
			if got != tt.want {
				t.Errorf("extractClaudeContent() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestExtractClaudeToolCalls tests tool call extraction.
func TestExtractClaudeToolCalls(t *testing.T) {
	content := []interface{}{
		map[string]interface{}{"type": "text", "text": "Let me help"},
		map[string]interface{}{
			"type":  "tool_use",
			"name":  "write",
			"input": map[string]interface{}{"path": "test.go", "content": "package main"},
		},
		map[string]interface{}{
			"type":  "tool_use",
			"name":  "bash",
			"input": map[string]interface{}{"command": "go build"},
		},
	}

	calls := extractClaudeToolCalls(content)

	if len(calls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(calls))
	}

	if calls[0].Name != "write" {
		t.Errorf("first tool call name = %q, want %q", calls[0].Name, "write")
	}

	if calls[1].Name != "bash" {
		t.Errorf("second tool call name = %q, want %q", calls[1].Name, "bash")
	}
}

// TestClaudeParseSessionFile tests parsing a Claude JSONL session file.
func TestClaudeParseSessionFile(t *testing.T) {
	// Create a temp directory
	tmpDir, err := os.MkdirTemp("", "nota-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test JSONL file
	sessionFile := filepath.Join(tmpDir, "test-session.jsonl")
	entries := []claudeEntry{
		{
			Type:      "user",
			SessionID: "session-123",
			Timestamp: "2024-01-15T10:00:00Z",
			Cwd:       "/test/repo",
			GitBranch: "main",
			Message:   claudeMessage{Role: "user", Content: "Write a hello world function"},
		},
		{
			Type:      "assistant",
			SessionID: "session-123",
			Timestamp: "2024-01-15T10:01:00Z",
			Message: claudeMessage{
				Role:    "assistant",
				Content: "Here's a hello world function:",
				Usage: &claudeUsage{
					InputTokens:  100,
					OutputTokens: 50,
				},
			},
		},
		{
			Type:      "user",
			SessionID: "session-123",
			Timestamp: "2024-01-15T10:02:00Z",
			Message:   claudeMessage{Role: "user", Content: "Thanks!"},
		},
	}

	f, err := os.Create(sessionFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		data, _ := json.Marshal(entry)
		f.Write(data)
		f.WriteString("\n")
	}
	f.Close()

	// Parse the file
	extractor := NewClaudeExtractor()
	session, err := extractor.parseSessionFile(sessionFile, "/test/repo")
	if err != nil {
		t.Fatalf("parseSessionFile failed: %v", err)
	}

	// Verify session
	if session.ID != "session-123" {
		t.Errorf("session ID = %q, want %q", session.ID, "session-123")
	}

	if session.GitBranch != "main" {
		t.Errorf("git branch = %q, want %q", session.GitBranch, "main")
	}

	if len(session.Messages) != 3 {
		t.Errorf("message count = %d, want 3", len(session.Messages))
	}

	if session.InputTokens != 100 {
		t.Errorf("input tokens = %d, want 100", session.InputTokens)
	}

	if session.OutputTokens != 50 {
		t.Errorf("output tokens = %d, want 50", session.OutputTokens)
	}

	// Check time bounds
	expectedStart, _ := time.Parse(time.RFC3339, "2024-01-15T10:00:00Z")
	expectedEnd, _ := time.Parse(time.RFC3339, "2024-01-15T10:02:00Z")

	if !session.StartTime.Equal(expectedStart) {
		t.Errorf("start time = %v, want %v", session.StartTime, expectedStart)
	}

	if !session.EndTime.Equal(expectedEnd) {
		t.Errorf("end time = %v, want %v", session.EndTime, expectedEnd)
	}
}

// TestClaudeSkipsSidechainMessages tests that sidechain messages are skipped.
func TestClaudeSkipsSidechainMessages(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nota-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	sessionFile := filepath.Join(tmpDir, "test-session.jsonl")
	entries := []claudeEntry{
		{
			Type:      "user",
			SessionID: "session-123",
			Timestamp: "2024-01-15T10:00:00Z",
			Message:   claudeMessage{Role: "user", Content: "Main message"},
		},
		{
			Type:        "assistant",
			SessionID:   "session-123",
			Timestamp:   "2024-01-15T10:01:00Z",
			IsSidechain: true, // Should be skipped
			Message:     claudeMessage{Role: "assistant", Content: "Sidechain response"},
		},
		{
			Type:      "assistant",
			SessionID: "session-123",
			Timestamp: "2024-01-15T10:02:00Z",
			Message:   claudeMessage{Role: "assistant", Content: "Main response"},
		},
	}

	f, err := os.Create(sessionFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		data, _ := json.Marshal(entry)
		f.Write(data)
		f.WriteString("\n")
	}
	f.Close()

	extractor := NewClaudeExtractor()
	session, err := extractor.parseSessionFile(sessionFile, "/test/repo")
	if err != nil {
		t.Fatalf("parseSessionFile failed: %v", err)
	}

	// Should only have 2 messages (sidechain skipped)
	if len(session.Messages) != 2 {
		t.Errorf("message count = %d, want 2 (sidechain should be skipped)", len(session.Messages))
	}
}

// TestDefaultRegistry tests the default registry setup.
func TestDefaultRegistry(t *testing.T) {
	registry := DefaultRegistry()
	extractors := registry.All()

	if len(extractors) != 3 {
		t.Errorf("expected 3 extractors, got %d", len(extractors))
	}

	// Verify names
	names := make(map[string]bool)
	for _, e := range extractors {
		names[e.Name()] = true
	}

	expected := []string{"claude-code", "opencode", "codex"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("missing extractor %q", name)
		}
	}
}

// TestExtractorNames tests that extractors return correct names.
func TestExtractorNames(t *testing.T) {
	tests := []struct {
		extractor Extractor
		want      string
	}{
		{NewClaudeExtractor(), "claude-code"},
		{NewOpenCodeExtractor(), "opencode"},
		{NewCodexExtractor(), "codex"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.extractor.Name(); got != tt.want {
				t.Errorf("Name() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestCodexSessionIDExtraction tests extracting session ID from Codex file path.
func TestCodexSessionIDExtraction(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{
			"/home/user/.codex/sessions/2025/11/03/rollout-2025-11-03T07-33-17-019a486b-840a-7282-8cdd-d103a9c4d0a2.jsonl",
			"2025-11-03T07-33-17-019a486b-840a-7282-8cdd-d103a9c4d0a2",
		},
		{
			"rollout-session-id.jsonl",
			"session-id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := extractCodexSessionID(tt.path)
			if got != tt.want {
				t.Errorf("extractCodexSessionID(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// TestClaudeExtractFromNonExistentDir tests graceful handling of missing directories.
func TestClaudeExtractFromNonExistentDir(t *testing.T) {
	extractor := NewClaudeExtractor()

	// Extract from non-existent repo should return empty, not error
	sessions, err := extractor.Extract("/non/existent/path/that/does/not/exist")
	if err != nil {
		t.Errorf("Extract() returned error for non-existent path: %v", err)
	}

	if len(sessions) != 0 {
		t.Errorf("Expected 0 sessions for non-existent path, got %d", len(sessions))
	}
}

// TestOpenCodeExtractFromNonExistentDir tests graceful handling of missing directories.
func TestOpenCodeExtractFromNonExistentDir(t *testing.T) {
	extractor := NewOpenCodeExtractor()

	// When storage dir doesn't exist, should return empty
	sessions, err := extractor.ExtractAll()
	// This might return sessions if the user has opencode installed
	// So just check it doesn't error
	if err != nil {
		t.Errorf("ExtractAll() returned unexpected error: %v", err)
	}
	_ = sessions // May or may not be empty depending on system
}

// TestCodexExtractFromNonExistentDir tests graceful handling of missing directories.
func TestCodexExtractFromNonExistentDir(t *testing.T) {
	extractor := NewCodexExtractor()

	// When sessions dir doesn't exist, should return empty
	sessions, err := extractor.ExtractAll()
	// This might return sessions if the user has codex installed
	// So just check it doesn't error
	if err != nil {
		t.Errorf("ExtractAll() returned unexpected error: %v", err)
	}
	_ = sessions // May or may not be empty depending on system
}
