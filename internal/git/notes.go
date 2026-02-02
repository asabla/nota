// Package git provides utilities for interacting with git repositories,
// including git notes operations for storing AI transcript metadata.
package git

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/emilsoderling/nota/internal/types"
)

const (
	// NotesRef is the git notes reference used for AI transcripts.
	NotesRef = "refs/notes/ai-transcripts"
)

// Notes provides operations for managing AI transcript notes in a git repository.
type Notes struct {
	repoPath string
}

// NewNotes creates a new Notes instance for the given repository path.
func NewNotes(repoPath string) (*Notes, error) {
	// Verify it's a git repository
	gitDir := filepath.Join(repoPath, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("not a git repository: %s", repoPath)
	}

	return &Notes{repoPath: repoPath}, nil
}

// Add adds a note to the specified commit.
func (n *Notes) Add(commitSHA string, metadata types.NoteMetadata) error {
	// Serialize metadata to JSON (single line for cat_sort_uniq merge strategy)
	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to serialize metadata: %w", err)
	}

	// Write to temp file
	tmpFile, err := os.CreateTemp("", "nota-note-*.json")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	tmpFile.Close()

	// Add the note using git notes
	cmd := exec.Command("git", "notes", "--ref="+NotesRef, "add", "-f", "-F", tmpFile.Name(), commitSHA)
	cmd.Dir = n.repoPath

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git notes add failed: %w: %s", err, stderr.String())
	}

	return nil
}

// Get retrieves the note for a specific commit.
func (n *Notes) Get(commitSHA string) (*types.NoteMetadata, error) {
	cmd := exec.Command("git", "notes", "--ref="+NotesRef, "show", commitSHA)
	cmd.Dir = n.repoPath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Note doesn't exist
		if strings.Contains(stderr.String(), "No note found") {
			return nil, nil
		}
		return nil, fmt.Errorf("git notes show failed: %w: %s", err, stderr.String())
	}

	var metadata types.NoteMetadata
	if err := json.Unmarshal(stdout.Bytes(), &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse note: %w", err)
	}

	return &metadata, nil
}

// List returns all commits that have AI transcript notes.
func (n *Notes) List() (map[string]types.NoteMetadata, error) {
	// Get list of notes
	cmd := exec.Command("git", "notes", "--ref="+NotesRef, "list")
	cmd.Dir = n.repoPath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// No notes exist yet
		if strings.Contains(stderr.String(), "Ref ") && strings.Contains(stderr.String(), "does not exist") {
			return make(map[string]types.NoteMetadata), nil
		}
		// Empty notes ref is also valid
		if stdout.Len() == 0 {
			return make(map[string]types.NoteMetadata), nil
		}
		return nil, fmt.Errorf("git notes list failed: %w: %s", err, stderr.String())
	}

	result := make(map[string]types.NoteMetadata)

	// Parse output: each line is "note-blob-sha commit-sha"
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}

		commitSHA := parts[1]

		// Get the note content
		metadata, err := n.Get(commitSHA)
		if err != nil || metadata == nil {
			continue
		}

		result[commitSHA] = *metadata
	}

	return result, nil
}

// Remove removes the note from a commit.
func (n *Notes) Remove(commitSHA string) error {
	cmd := exec.Command("git", "notes", "--ref="+NotesRef, "remove", commitSHA)
	cmd.Dir = n.repoPath

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git notes remove failed: %w: %s", err, stderr.String())
	}

	return nil
}

// GetCommitInfo retrieves information about a commit.
func (n *Notes) GetCommitInfo(commitSHA string) (*CommitInfo, error) {
	// Get commit timestamp and changed files
	cmd := exec.Command("git", "show", "--format=%cI%n%s", "--name-only", commitSHA)
	cmd.Dir = n.repoPath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git show failed: %w: %s", err, stderr.String())
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) < 2 {
		return nil, fmt.Errorf("unexpected git show output")
	}

	timestamp, err := time.Parse(time.RFC3339, lines[0])
	if err != nil {
		return nil, fmt.Errorf("failed to parse commit timestamp: %w", err)
	}

	info := &CommitInfo{
		SHA:       commitSHA,
		Timestamp: timestamp,
		Subject:   lines[1],
		Files:     make([]string, 0),
	}

	// Rest of lines are file names (skip empty lines)
	for i := 2; i < len(lines); i++ {
		if lines[i] != "" {
			info.Files = append(info.Files, lines[i])
		}
	}

	return info, nil
}

// GetHeadSHA returns the SHA of HEAD.
func (n *Notes) GetHeadSHA() (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = n.repoPath

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git rev-parse HEAD failed: %w", err)
	}

	return strings.TrimSpace(stdout.String()), nil
}

// GetRepoRoot returns the root directory of the repository.
func GetRepoRoot(path string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = path

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("not a git repository: %w", err)
	}

	return strings.TrimSpace(stdout.String()), nil
}

// CommitInfo contains information about a git commit.
type CommitInfo struct {
	SHA       string
	Timestamp time.Time
	Subject   string
	Files     []string
}
