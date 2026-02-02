package git

import (
	"fmt"
	"os"
	"path/filepath"
)

const hookScript = `#!/usr/bin/env sh
# nota post-commit hook
# This hook is called after a commit is created.
# It extracts AI transcripts and links them to the commit.

# Get script directory for locating binary
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# Find the nota binary (check common locations)
if command -v nota >/dev/null 2>&1; then
    NOTA="nota"
elif [ -x "$SCRIPT_DIR/../../.nota/bin/nota" ]; then
    NOTA="$SCRIPT_DIR/../../.nota/bin/nota"
elif [ -x "/usr/local/bin/nota" ]; then
    NOTA="/usr/local/bin/nota"
else
    # Silently skip if nota not found
    exit 0
fi

# Get commit SHA
COMMIT_SHA=$(git rev-parse HEAD)

# Run nota post-commit (errors are logged but don't block)
"$NOTA" post-commit --commit="$COMMIT_SHA" 2>/dev/null || true
`

// InstallHook installs the post-commit hook in the repository.
func InstallHook(repoPath string) error {
	hookDir := filepath.Join(repoPath, ".git", "hooks")
	hookPath := filepath.Join(hookDir, "post-commit")

	// Check if .git/hooks directory exists
	if _, err := os.Stat(hookDir); os.IsNotExist(err) {
		return fmt.Errorf("not a git repository or hooks directory missing: %s", hookDir)
	}

	// Check if hook already exists
	if _, err := os.Stat(hookPath); err == nil {
		// Read existing hook to check if it's ours
		content, err := os.ReadFile(hookPath)
		if err != nil {
			return fmt.Errorf("failed to read existing hook: %w", err)
		}

		// If it's our hook, just update it
		if containsNotaHook(string(content)) {
			return os.WriteFile(hookPath, []byte(hookScript), 0755)
		}

		// Existing hook from something else - don't overwrite
		return fmt.Errorf("post-commit hook already exists (not from nota). Please manually integrate nota into your existing hook")
	}

	// Write the hook
	if err := os.WriteFile(hookPath, []byte(hookScript), 0755); err != nil {
		return fmt.Errorf("failed to write hook: %w", err)
	}

	return nil
}

// UninstallHook removes the post-commit hook if it was installed by nota.
func UninstallHook(repoPath string) error {
	hookPath := filepath.Join(repoPath, ".git", "hooks", "post-commit")

	// Check if hook exists
	content, err := os.ReadFile(hookPath)
	if os.IsNotExist(err) {
		return nil // Nothing to remove
	}
	if err != nil {
		return fmt.Errorf("failed to read hook: %w", err)
	}

	// Only remove if it's our hook
	if !containsNotaHook(string(content)) {
		return fmt.Errorf("post-commit hook exists but was not installed by nota")
	}

	if err := os.Remove(hookPath); err != nil {
		return fmt.Errorf("failed to remove hook: %w", err)
	}

	return nil
}

// IsHookInstalled checks if the nota hook is installed.
func IsHookInstalled(repoPath string) bool {
	hookPath := filepath.Join(repoPath, ".git", "hooks", "post-commit")

	content, err := os.ReadFile(hookPath)
	if err != nil {
		return false
	}

	return containsNotaHook(string(content))
}

// containsNotaHook checks if hook content is from nota.
func containsNotaHook(content string) bool {
	return len(content) > 0 &&
		(containsString(content, "nota post-commit hook") ||
			containsString(content, "nota post-commit"))
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr) >= 0
}

func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
