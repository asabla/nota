package git

import (
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/emilsoderling/nota/internal/types"
)

// MatchConfig configures the session-to-commit matching algorithm.
type MatchConfig struct {
	// MaxTimeDelta is the maximum time between session end and commit time
	MaxTimeDelta time.Duration

	// MinScore is the minimum match score to consider a session
	MinScore float64

	// TimeWeight is the weight given to temporal proximity (0-1)
	TimeWeight float64

	// FileOverlapWeight is the weight given to file overlap (0-1)
	FileOverlapWeight float64
}

// DefaultMatchConfig returns the default matching configuration.
func DefaultMatchConfig() MatchConfig {
	return MatchConfig{
		MaxTimeDelta:      30 * time.Minute,
		MinScore:          0.1,
		TimeWeight:        0.6,
		FileOverlapWeight: 0.4,
	}
}

// MatchResult represents a session matched to a commit with a confidence score.
type MatchResult struct {
	Session types.Session
	Score   float64
	Reasons []string
}

// MatchSessionsToCommit finds sessions that likely produced the given commit.
func MatchSessionsToCommit(sessions []types.Session, commitInfo *CommitInfo, repoPath string, config MatchConfig) []MatchResult {
	var results []MatchResult

	absRepoPath, _ := filepath.Abs(repoPath)

	for _, session := range sessions {
		// First filter: directory must match
		if !isInRepo(session.WorkingDirectory, absRepoPath) {
			continue
		}

		score, reasons := scoreSession(session, commitInfo, config)
		if score >= config.MinScore {
			results = append(results, MatchResult{
				Session: session,
				Score:   score,
				Reasons: reasons,
			})
		}
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results
}

// scoreSession calculates a match score for a session against a commit.
func scoreSession(session types.Session, commitInfo *CommitInfo, config MatchConfig) (float64, []string) {
	var score float64
	var reasons []string

	// Time-based scoring
	timeScore := calculateTimeScore(session, commitInfo, config.MaxTimeDelta)
	if timeScore > 0 {
		score += timeScore * config.TimeWeight
		reasons = append(reasons, formatTimeReason(session, commitInfo))
	}

	// File overlap scoring
	fileScore := calculateFileOverlapScore(session, commitInfo)
	if fileScore > 0 {
		score += fileScore * config.FileOverlapWeight
		reasons = append(reasons, formatFileReason(session, commitInfo))
	}

	return score, reasons
}

// calculateTimeScore returns a score (0-1) based on temporal proximity.
// Sessions that ended shortly before the commit get higher scores.
func calculateTimeScore(session types.Session, commitInfo *CommitInfo, maxDelta time.Duration) float64 {
	if session.EndTime.IsZero() {
		return 0
	}

	// Time between session end and commit
	delta := commitInfo.Timestamp.Sub(session.EndTime)

	// Session must have ended before the commit (or within a small grace period)
	gracePeriod := 5 * time.Minute
	if delta < -gracePeriod {
		return 0 // Session ended after commit
	}

	// If session ended after commit but within grace period, still consider it
	if delta < 0 {
		delta = 0
	}

	// Beyond max delta, no match
	if delta > maxDelta {
		return 0
	}

	// Linear decay: closer to commit = higher score
	return 1.0 - (float64(delta) / float64(maxDelta))
}

// calculateFileOverlapScore returns a score (0-1) based on file overlap.
func calculateFileOverlapScore(session types.Session, commitInfo *CommitInfo) float64 {
	if len(commitInfo.Files) == 0 {
		return 0
	}

	sessionFiles := extractFilesFromSession(session)
	if len(sessionFiles) == 0 {
		return 0
	}

	// Count overlapping files
	overlap := 0
	for _, commitFile := range commitInfo.Files {
		for _, sessionFile := range sessionFiles {
			if filesMatch(commitFile, sessionFile) {
				overlap++
				break
			}
		}
	}

	if overlap == 0 {
		return 0
	}

	// Score based on what fraction of commit files were touched in session
	return float64(overlap) / float64(len(commitInfo.Files))
}

// extractFilesFromSession extracts file paths mentioned in tool calls.
func extractFilesFromSession(session types.Session) []string {
	fileSet := make(map[string]bool)

	for _, msg := range session.Messages {
		for _, tc := range msg.ToolCalls {
			// Look for file paths in common tool call patterns
			if path := extractFileFromToolCall(tc); path != "" {
				fileSet[path] = true
			}
		}
	}

	// Also check FilesTouched if populated
	for _, f := range session.FilesTouched {
		fileSet[f] = true
	}

	var files []string
	for f := range fileSet {
		files = append(files, f)
	}
	return files
}

// extractFileFromToolCall tries to extract a file path from a tool call.
func extractFileFromToolCall(tc types.ToolCall) string {
	// Common parameter names for file paths
	paramNames := []string{"path", "file_path", "filePath", "file", "filename"}

	for _, name := range paramNames {
		if v, ok := tc.Input[name]; ok {
			if path, ok := v.(string); ok {
				return path
			}
		}
	}

	return ""
}

// filesMatch checks if two file paths refer to the same file.
func filesMatch(a, b string) bool {
	// Normalize paths
	a = filepath.Clean(a)
	b = filepath.Clean(b)

	// Direct match
	if a == b {
		return true
	}

	// Check if one is a suffix of the other (handles relative vs absolute)
	if strings.HasSuffix(a, "/"+b) || strings.HasSuffix(b, "/"+a) {
		return true
	}

	// Check basename match as fallback
	if filepath.Base(a) == filepath.Base(b) {
		return true
	}

	return false
}

// isInRepo checks if a path is within the repository.
func isInRepo(sessionDir, repoPath string) bool {
	// Normalize paths
	sessionDir = filepath.Clean(sessionDir)
	repoPath = filepath.Clean(repoPath)

	// Exact match or subdirectory
	return sessionDir == repoPath || strings.HasPrefix(sessionDir, repoPath+string(filepath.Separator))
}

// formatTimeReason formats a human-readable reason for time-based matching.
func formatTimeReason(session types.Session, commitInfo *CommitInfo) string {
	delta := commitInfo.Timestamp.Sub(session.EndTime)
	if delta < time.Minute {
		return "session ended seconds before commit"
	} else if delta < time.Hour {
		return "session ended " + delta.Round(time.Minute).String() + " before commit"
	}
	return "session ended " + delta.Round(time.Hour).String() + " before commit"
}

// formatFileReason formats a human-readable reason for file-based matching.
func formatFileReason(session types.Session, commitInfo *CommitInfo) string {
	sessionFiles := extractFilesFromSession(session)
	overlap := 0
	for _, commitFile := range commitInfo.Files {
		for _, sessionFile := range sessionFiles {
			if filesMatch(commitFile, sessionFile) {
				overlap++
				break
			}
		}
	}
	return "session touched " + string(rune('0'+overlap)) + "/" + string(rune('0'+len(commitInfo.Files))) + " commit files"
}
