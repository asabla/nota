// Package extractors provides interfaces and utilities for extracting
// AI coding assistant transcripts from various sources.
package extractors

import (
	"os"
	"path/filepath"

	"github.com/emilsoderling/nota/internal/types"
)

// Extractor defines the interface for extracting sessions from AI assistant transcripts.
type Extractor interface {
	// Name returns the identifier for this extractor (e.g., "claude-code", "opencode", "codex")
	Name() string

	// Extract finds and parses sessions from the default storage location
	// that are associated with the given repository path.
	Extract(repoPath string) ([]types.Session, error)

	// ExtractAll finds and parses all sessions from the default storage location,
	// regardless of repository association.
	ExtractAll() ([]types.Session, error)
}

// SessionInfo pairs a session with the extractor that found it.
type SessionInfo struct {
	Session   types.Session `json:"session"`
	Extractor string        `json:"extractor"`
}

// Registry holds registered extractors for discovery and iteration.
type Registry struct {
	extractors []Extractor
}

// NewRegistry creates a new extractor registry.
func NewRegistry() *Registry {
	return &Registry{
		extractors: make([]Extractor, 0),
	}
}

// Register adds an extractor to the registry.
func (r *Registry) Register(e Extractor) {
	r.extractors = append(r.extractors, e)
}

// All returns all registered extractors.
func (r *Registry) All() []Extractor {
	return r.extractors
}

// ExtractAll runs all extractors for the given repo path and combines results.
func (r *Registry) ExtractAll(repoPath string) ([]types.Session, error) {
	var allSessions []types.Session

	for _, e := range r.extractors {
		sessions, err := e.Extract(repoPath)
		if err != nil {
			// Log but continue with other extractors
			continue
		}
		allSessions = append(allSessions, sessions...)
	}

	return allSessions, nil
}

// DefaultRegistry returns a registry with all built-in extractors.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(NewClaudeExtractor())
	r.Register(NewOpenCodeExtractor())
	r.Register(NewCodexExtractor())
	return r
}

// getHomeDir returns the user's home directory, with fallback for different platforms.
func getHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		// Fallback for edge cases
		if h := os.Getenv("HOME"); h != "" {
			return h
		}
		if h := os.Getenv("USERPROFILE"); h != "" {
			return h
		}
	}
	return home
}

// expandPath expands ~ to the user's home directory.
func expandPath(path string) string {
	if len(path) > 0 && path[0] == '~' {
		return filepath.Join(getHomeDir(), path[1:])
	}
	return path
}
