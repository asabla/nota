// Package extractors provides interfaces and utilities for extracting
// AI coding assistant transcripts from various sources.
package extractors

import "github.com/emilsoderling/nota/internal/types"

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
