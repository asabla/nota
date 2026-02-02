package extractors

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/emilsoderling/nota/internal/types"
)

// OpenCodeExtractor extracts sessions from OpenCode transcript files.
// OpenCode stores sessions in ~/.local/share/opencode/storage/ with separate
// JSON files for sessions, messages, and parts.
type OpenCodeExtractor struct {
	basePath string
}

// openCodeSession represents an OpenCode session JSON file.
type openCodeSession struct {
	ID        string `json:"id"`
	Version   string `json:"version"`
	ProjectID string `json:"projectID"`
	Directory string `json:"directory"`
	Title     string `json:"title"`
	Time      struct {
		Created int64 `json:"created"`
		Updated int64 `json:"updated"`
	} `json:"time"`
	Summary struct {
		Additions int `json:"additions"`
		Deletions int `json:"deletions"`
		Files     int `json:"files"`
	} `json:"summary"`
}

// openCodeMessage represents an OpenCode message JSON file.
type openCodeMessage struct {
	ID         string `json:"id"`
	SessionID  string `json:"sessionID"`
	Role       string `json:"role"`
	ParentID   string `json:"parentID"`
	ModelID    string `json:"modelID"`
	ProviderID string `json:"providerID"`
	Time       struct {
		Created   int64 `json:"created"`
		Completed int64 `json:"completed"`
	} `json:"time"`
	Path struct {
		Cwd  string `json:"cwd"`
		Root string `json:"root"`
	} `json:"path"`
	Tokens struct {
		Input     int `json:"input"`
		Output    int `json:"output"`
		Reasoning int `json:"reasoning"`
		Cache     struct {
			Read  int `json:"read"`
			Write int `json:"write"`
		} `json:"cache"`
	} `json:"tokens"`
	Finish string `json:"finish"`
}

// openCodePart represents an OpenCode message part JSON file.
type openCodePart struct {
	ID        string `json:"id"`
	MessageID string `json:"messageID"`
	Type      string `json:"type"` // text, tool-call, tool-result, file, thinking, etc.
	Time      struct {
		Created int64 `json:"created"`
	} `json:"time"`
	// For text parts
	Text string `json:"text"`
	// For tool-call parts
	Tool struct {
		Name  string                 `json:"name"`
		Input map[string]interface{} `json:"input"`
	} `json:"tool"`
	// For tool-result parts
	Result string `json:"result"`
}

// NewOpenCodeExtractor creates a new OpenCode extractor.
func NewOpenCodeExtractor() *OpenCodeExtractor {
	basePath := filepath.Join(getHomeDir(), ".local", "share", "opencode", "storage")

	// Check for override via environment variable
	if customPath := os.Getenv("OPENCODE_DATA_DIR"); customPath != "" {
		basePath = filepath.Join(customPath, "storage")
	}

	return &OpenCodeExtractor{
		basePath: basePath,
	}
}

// Name returns the extractor identifier.
func (e *OpenCodeExtractor) Name() string {
	return "opencode"
}

// Extract finds sessions associated with the given repository path.
func (e *OpenCodeExtractor) Extract(repoPath string) ([]types.Session, error) {
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
func (e *OpenCodeExtractor) ExtractAll() ([]types.Session, error) {
	var allSessions []types.Session

	sessionDir := filepath.Join(e.basePath, "session")

	// Check if directory exists
	if _, err := os.Stat(sessionDir); os.IsNotExist(err) {
		return []types.Session{}, nil
	}

	// Walk all project hash directories
	projectDirs, err := os.ReadDir(sessionDir)
	if err != nil {
		return nil, err
	}

	for _, projectDir := range projectDirs {
		if !projectDir.IsDir() {
			continue
		}

		projectPath := filepath.Join(sessionDir, projectDir.Name())

		// Find all session files in this project
		sessionFiles, err := filepath.Glob(filepath.Join(projectPath, "ses_*.json"))
		if err != nil {
			continue
		}

		for _, sessionFile := range sessionFiles {
			session, err := e.parseSession(sessionFile)
			if err != nil {
				continue
			}
			if session != nil {
				allSessions = append(allSessions, *session)
			}
		}
	}

	return allSessions, nil
}

// parseSession parses a single session file and loads its messages.
func (e *OpenCodeExtractor) parseSession(sessionFile string) (*types.Session, error) {
	data, err := os.ReadFile(sessionFile)
	if err != nil {
		return nil, err
	}

	var ocSession openCodeSession
	if err := json.Unmarshal(data, &ocSession); err != nil {
		return nil, err
	}

	session := &types.Session{
		ID:               ocSession.ID,
		Tool:             "opencode",
		WorkingDirectory: ocSession.Directory,
		SourcePath:       sessionFile,
		StartTime:        time.UnixMilli(ocSession.Time.Created),
		EndTime:          time.UnixMilli(ocSession.Time.Updated),
		Messages:         make([]types.Message, 0),
	}

	// Load messages for this session
	messages, err := e.loadMessages(ocSession.ID)
	if err != nil {
		// Continue with empty messages if we can't load them
		return session, nil
	}

	// Sort messages by creation time
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].Timestamp.Before(messages[j].Timestamp)
	})

	session.Messages = messages

	// Calculate token totals from messages
	var totalInput, totalOutput, totalCache int
	var model string

	// Re-load messages to get token info (this is a bit inefficient, could optimize)
	msgDir := filepath.Join(e.basePath, "message", ocSession.ID)
	if msgFiles, err := filepath.Glob(filepath.Join(msgDir, "msg_*.json")); err == nil {
		for _, msgFile := range msgFiles {
			if msgData, err := os.ReadFile(msgFile); err == nil {
				var ocMsg openCodeMessage
				if json.Unmarshal(msgData, &ocMsg) == nil {
					totalInput += ocMsg.Tokens.Input
					totalOutput += ocMsg.Tokens.Output
					totalCache += ocMsg.Tokens.Cache.Read
					if model == "" && ocMsg.ModelID != "" {
						model = ocMsg.ModelID
					}
				}
			}
		}
	}

	session.InputTokens = totalInput
	session.OutputTokens = totalOutput
	session.CacheTokens = totalCache
	session.Model = model

	return session, nil
}

// loadMessages loads all messages for a session.
func (e *OpenCodeExtractor) loadMessages(sessionID string) ([]types.Message, error) {
	var messages []types.Message

	msgDir := filepath.Join(e.basePath, "message", sessionID)

	// Check if directory exists
	if _, err := os.Stat(msgDir); os.IsNotExist(err) {
		return messages, nil
	}

	msgFiles, err := filepath.Glob(filepath.Join(msgDir, "msg_*.json"))
	if err != nil {
		return nil, err
	}

	for _, msgFile := range msgFiles {
		data, err := os.ReadFile(msgFile)
		if err != nil {
			continue
		}

		var ocMsg openCodeMessage
		if err := json.Unmarshal(data, &ocMsg); err != nil {
			continue
		}

		// Load parts for this message
		content, toolCalls := e.loadParts(ocMsg.ID)

		msg := types.Message{
			Role:      ocMsg.Role,
			Content:   content,
			Timestamp: time.UnixMilli(ocMsg.Time.Created),
			ToolCalls: toolCalls,
		}

		messages = append(messages, msg)
	}

	return messages, nil
}

// loadParts loads all parts for a message and returns combined content and tool calls.
func (e *OpenCodeExtractor) loadParts(messageID string) (string, []types.ToolCall) {
	var textParts []string
	var toolCalls []types.ToolCall

	partDir := filepath.Join(e.basePath, "part", messageID)

	// Check if directory exists
	if _, err := os.Stat(partDir); os.IsNotExist(err) {
		return "", nil
	}

	partFiles, err := filepath.Glob(filepath.Join(partDir, "prt_*.json"))
	if err != nil {
		return "", nil
	}

	// Sort part files to maintain order
	sort.Strings(partFiles)

	for _, partFile := range partFiles {
		data, err := os.ReadFile(partFile)
		if err != nil {
			continue
		}

		var part openCodePart
		if err := json.Unmarshal(data, &part); err != nil {
			continue
		}

		switch part.Type {
		case "text":
			if part.Text != "" {
				textParts = append(textParts, part.Text)
			}
		case "tool-call":
			if part.Tool.Name != "" {
				toolCalls = append(toolCalls, types.ToolCall{
					Name:  part.Tool.Name,
					Input: part.Tool.Input,
				})
			}
		case "tool-result":
			// Could track tool results if needed
		}
	}

	return strings.Join(textParts, "\n"), toolCalls
}
