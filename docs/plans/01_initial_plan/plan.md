# Implementation plan for cross-platform AI transcript auditing tool

**TL;DR**: Build a Go-based CLI tool that extracts JSONL transcripts from `~/.claude/projects/`, JSON sessions from `~/.local/share/opencode/storage/`, and JSONL from `~/.codex/sessions/`, then injects metadata into git notes under `refs/notes/ai-transcripts`. Use a thin POSIX shell hook wrapper to call the Go binary. Serve an HTMX-powered viewer with infinite scroll and expandable transcripts.

---

## Architecture overview

The tool consists of four main components: **extractors** for each AI assistant, a **git notes writer**, a **hook dispatcher**, and an **HTMX web viewer**. Go is the recommended language—it produces single static binaries that cross-compile trivially (`GOOS=windows go build`) and requires zero runtime dependencies. The GitHub CLI, lazygit, and Docker all use this approach successfully.

```
┌──────────────────────────────────────────────────────────────────────┐
│                         ai-audit (Go binary)                         │
├──────────────────────────────────────────────────────────────────────┤
│  Commands:                                                           │
│    post-commit     - Extract + inject (called from git hook)         │
│    extract         - Extract transcripts without git integration     │
│    serve           - Start HTMX viewer server                        │
│    notes list      - List all AI-linked commits                      │
│    notes export    - Export notes to JSON                            │
└──────────────────────────────────────────────────────────────────────┘
          │                    │                    │
          ▼                    ▼                    ▼
   ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
   │Claude Code  │     │  OpenCode   │     │   Codex     │
   │  Extractor  │     │  Extractor  │     │  Extractor  │
   └─────────────┘     └─────────────┘     └─────────────┘
```

---

## Transcript storage locations and formats

### Claude Code (Anthropic)

Claude Code stores conversations in `~/.claude/projects/` with directory paths encoded by replacing `/` with `-`.

| Aspect | Details |
|--------|---------|
| **Location** | `~/.claude/projects/{encoded-path}/{session-uuid}.jsonl` |
| **Format** | JSONL (one JSON object per line) |
| **Subagents** | `~/.claude/projects/{path}/{sessionId}/subagents/agent-{id}.jsonl` |
| **Windows** | `%USERPROFILE%\.claude\projects\` |

**Schema per line:**
```json
{
  "type": "user" | "assistant",
  "uuid": "message-uuid",
  "parentUuid": "parent-message-uuid",
  "sessionId": "00893aaf-19fa-41d2-8238-13269b9b3ca0",
  "timestamp": "2025-06-02T18:46:59.937Z",
  "cwd": "/home/user/project",
  "message": {
    "role": "user" | "assistant",
    "content": "text" | [{"type": "text", "text": "..."}, {"type": "tool_use", ...}],
    "usage": {"input_tokens": 500, "output_tokens": 1200, "cache_read_input_tokens": 3000}
  },
  "isSidechain": false
}
```

**Parsing approach:** Read JSONL, filter lines by `"type": "user"` or `"type": "assistant"`, extract `message.content` and `message.usage`. Skip lines where `isSidechain: true` (subagent context).

---

### OpenCode (SST)

OpenCode uses XDG-compliant paths with individual JSON files per entity—**not JSONL, not SQLite**.

| Aspect | Details |
|--------|---------|
| **Location** | `~/.local/share/opencode/storage/` |
| **Sessions** | `storage/session/{project-hash}/ses_{session-id}.json` |
| **Messages** | `storage/message/ses_{session-id}/msg_{message-id}.json` |
| **Parts** | `storage/part/msg_{message-id}/prt_{part-id}.json` |
| **Override** | `OPENCODE_DATA_DIR` environment variable |

**Session schema:**
```json
{
  "id": "ses_54083ec12ffeOtMmb4yMoQbN5R",
  "projectID": "project-hash",
  "directory": "/home/user/myproject",
  "parentID": null,
  "title": "New session - 2025-01-15T10:30:00.000Z",
  "version": "1.0.0",
  "time": {"created": 1736941800000, "updated": 1736942100000}
}
```

**Message part types:** `text`, `file`, `tool-call`, `tool-result`, `thinking`, `subtask`, `compaction`

**Parsing approach:** Scan `storage/session/` directories, then for each session, scan `storage/message/ses_{id}/` and `storage/part/msg_{id}/` to reconstruct full conversations.

---

### OpenAI Codex CLI

Codex stores sessions organized by date in JSONL format.

| Aspect | Details |
|--------|---------|
| **Location** | `~/.codex/sessions/YYYY/MM/DD/rollout-{timestamp}-{id}.jsonl` |
| **History** | `~/.codex/history.jsonl` (TUI prompt history) |
| **Config** | `~/.codex/config.toml` |
| **Override** | `CODEX_HOME` environment variable |

**Schema per line:**
```json
{
  "type": "message",
  "role": "user" | "assistant",
  "content": [{"type": "text", "text": "..."}]
}
```

**Other event types:** `thread.started`, `turn.started`, `turn.completed`, `item.*` (tool calls, file changes), `event_msg` (token counts)

**Parsing approach:** Glob `~/.codex/sessions/*/*/*.jsonl`, filter for `type: "message"`, extract content. Token counts appear in `event_msg` payloads with `payload.type: "token_count"`.

---

## Git notes integration

### Namespace design

Use a dedicated namespace to avoid conflicts with other tooling:

```bash
refs/notes/ai-transcripts          # Main transcript metadata
refs/notes/ai-transcripts/sessions # Full session dumps (if needed)
```

### Storing structured JSON in notes

Store one JSON object per note, keeping metadata compact and full transcripts referenced externally:

```json
{
  "v": 1,
  "ts": "2026-02-02T10:00:00Z",
  "tool": "claude-code",
  "session_id": "00893aaf-19fa-41d2-8238-13269b9b3ca0",
  "model": "claude-sonnet-4-20250514",
  "metrics": {
    "input_tokens": 1500,
    "output_tokens": 3200,
    "duration_ms": 8400
  },
  "files_changed": ["src/main.go", "README.md"],
  "transcript_path": "~/.claude/projects/-home-user-proj/00893aaf.jsonl"
}
```

**Commands:**
```bash
# Add note to commit
git notes --ref=ai-transcripts add -F /tmp/metadata.json HEAD

# View notes in log
git log --show-notes=ai-transcripts

# Configure automatic display
git config --add notes.displayRef 'refs/notes/ai-transcripts'
```

### Remote synchronization

Notes do **not** sync automatically. Configure fetch/push:

```bash
# Add to .git/config
git config --add remote.origin.fetch '+refs/notes/*:refs/notes/*'
git config --add remote.origin.push 'refs/notes/*'

# Or manually
git push origin refs/notes/ai-transcripts
git fetch origin refs/notes/ai-transcripts:refs/notes/ai-transcripts
```

**Merge strategy for concurrent edits:** Use `cat_sort_uniq` which concatenates, sorts, and deduplicates lines—works perfectly if each note entry is a single-line JSON with timestamps:

```bash
git config notes.ai-transcripts.mergeStrategy cat_sort_uniq
```

### Branch/PR-level summaries

Git notes attach to objects (commits), not refs. For branch-level summaries, annotate the **merge commit** or **branch tip commit**:

```go
// After PR merge, attach summary to merge commit
mergeCommit := getMergeCommitSHA(prNumber)
note := BranchSummary{
    PRNumber: prNumber,
    Branch:   "feature-x",
    Sessions: aggregatedSessionIDs,
    TotalTokens: sumTokens(sessions),
}
addGitNote("ai-transcripts", mergeCommit, note)
```

---

## Cross-platform git hook implementation

### Hook file pattern

Create a POSIX-compliant shell wrapper that calls the Go binary:

```bash
#!/usr/bin/env sh
# .git/hooks/post-commit
# Cross-platform: works on Windows (Git Bash), macOS, Linux

# Get script directory for locating binary
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# Find the ai-audit binary (check common locations)
if command -v ai-audit >/dev/null 2>&1; then
    AI_AUDIT="ai-audit"
elif [ -x "$SCRIPT_DIR/../../.ai-audit/bin/ai-audit" ]; then
    AI_AUDIT="$SCRIPT_DIR/../../.ai-audit/bin/ai-audit"
else
    echo "ai-audit not found, skipping transcript injection"
    exit 0
fi

# Get commit info (post-commit has no args)
COMMIT_SHA=$(git rev-parse HEAD)

# Run the tool
exec "$AI_AUDIT" post-commit --commit="$COMMIT_SHA"
```

### Environment variables available in post-commit

| Variable | Description |
|----------|-------------|
| `GIT_DIR` | Path to `.git` directory |
| `GIT_WORK_TREE` | Repository root |
| `GIT_AUTHOR_NAME/EMAIL` | Commit author info |
| `GIT_COMMITTER_NAME/EMAIL` | Committer info |

**Note:** `post-commit` receives no command-line arguments. Use `git rev-parse HEAD` to get the commit SHA.

### Hook installation

Provide an `install` command that creates the hook:

```go
func installHook(repoPath string) error {
    hookPath := filepath.Join(repoPath, ".git", "hooks", "post-commit")
    hookContent := `#!/usr/bin/env sh
COMMIT=$(git rev-parse HEAD)
ai-audit post-commit --commit="$COMMIT"
`
    return os.WriteFile(hookPath, []byte(hookContent), 0755)
}
```

**Alternative:** Use Lefthook for more complex hook management. It's written in Go, works cross-platform, and supports YAML configuration:

```yaml
# lefthook.yml
post-commit:
  commands:
    ai-audit:
      run: ai-audit post-commit --commit={head_sha}
```

---

## Recommended technology stack

### CLI tool: Go with Cobra

**Why Go:**
- Single static binary (no runtime, no dependencies)
- Trivial cross-compilation: `GOOS=windows GOARCH=amd64 go build`
- Excellent process spawning (`os/exec`) for calling git
- Battle-tested: GitHub CLI, Docker CLI, Terraform all use Go
- Cobra + Viper provide industry-standard CLI patterns

**Project structure:**
```
ai-audit/
├── cmd/
│   ├── root.go           # Cobra root command
│   ├── post_commit.go    # post-commit subcommand
│   ├── extract.go        # Manual extraction
│   └── serve.go          # HTMX viewer server
├── internal/
│   ├── extractors/
│   │   ├── claude.go     # Claude Code JSONL parser
│   │   ├── opencode.go   # OpenCode JSON parser
│   │   └── codex.go      # Codex JSONL parser
│   ├── git/
│   │   ├── notes.go      # Git notes operations
│   │   └── hooks.go      # Hook installation
│   └── viewer/
│       ├── server.go     # HTTP server
│       └── templates/    # HTML templates
├── main.go
└── go.mod
```

**Key dependencies:**
```go
import (
    "github.com/spf13/cobra"     // CLI framework
    "github.com/spf13/viper"     // Configuration
    "encoding/json"              // JSON parsing (stdlib)
    "os/exec"                    // Git command execution
    "path/filepath"              // Cross-platform paths
)
```

### Web viewer: Go stdlib + HTMX

No framework needed—Go's `net/http` and `html/template` are sufficient:

```go
func main() {
    http.HandleFunc("/", handleIndex)
    http.HandleFunc("/logs", handleLogs)
    http.HandleFunc("/transcript/", handleTranscript)
    http.ListenAndServe(":8080", nil)
}

func handleLogs(w http.ResponseWriter, r *http.Request) {
    isHtmx := r.Header.Get("HX-Request") == "true"
    logs := getLogsFromGitNotes(r.URL.Query().Get("page"))
    
    if isHtmx {
        tmpl.ExecuteTemplate(w, "log-entries", logs)
    } else {
        tmpl.ExecuteTemplate(w, "full-page", logs)
    }
}
```

---

## HTMX viewer patterns

### Timeline with infinite scroll

```html
<div class="timeline">
  {{ range $i, $entry := .Logs }}
  <div class="log-entry" 
       {{ if eq $i (sub (len $.Logs) 1) }}
       hx-get="/logs?after={{ $entry.ID }}"
       hx-trigger="revealed"
       hx-swap="afterend"
       {{ end }}>
    <span class="timestamp">{{ $entry.Timestamp }}</span>
    <code class="commit">{{ slice $entry.Commit 0 8 }}</code>
    <span class="tool">{{ $entry.Tool }}</span>
    <span class="tokens">{{ $entry.TotalTokens }} tokens</span>
    <button hx-get="/transcript/{{ $entry.SessionID }}"
            hx-target="#details-{{ $entry.ID }}"
            hx-swap="innerHTML">
      View Transcript
    </button>
    <div id="details-{{ $entry.ID }}" class="transcript-details"></div>
  </div>
  {{ end }}
</div>
```

### Expandable transcript details

```html
<details>
  <summary hx-get="/transcript/{{ .SessionID }}"
           hx-trigger="toggle once"
           hx-target="next div">
    Session: {{ slice .SessionID 0 8 }}... ({{ .TokenCount }} tokens)
  </summary>
  <div class="transcript-content">Loading...</div>
</details>
```

### Search/filter with debounce

```html
<input name="q"
       hx-get="/logs/search"
       hx-trigger="keyup changed delay:300ms"
       hx-target="#results"
       placeholder="Search by commit, session, or content..."/>
```

### Polling for new commits

```html
<div id="new-commits"
     hx-get="/logs/latest?since={{ .LatestCommit }}"
     hx-trigger="every 30s"
     hx-swap="afterbegin">
</div>
```

---

## Extraction algorithms

### Claude Code extractor

```go
func ExtractClaudeSessions(repoPath string) ([]Session, error) {
    // 1. Encode repo path: /home/user/project → -home-user-project
    encodedPath := strings.ReplaceAll(repoPath, string(os.PathSeparator), "-")
    
    // 2. Find session files
    claudeDir := filepath.Join(os.Getenv("HOME"), ".claude", "projects", encodedPath)
    files, _ := filepath.Glob(filepath.Join(claudeDir, "*.jsonl"))
    
    var sessions []Session
    for _, f := range files {
        if strings.HasPrefix(filepath.Base(f), "agent-") {
            continue // Skip subagent files
        }
        
        session := parseJSONL(f)
        sessions = append(sessions, session)
    }
    return sessions, nil
}

func parseJSONL(path string) Session {
    file, _ := os.Open(path)
    scanner := bufio.NewScanner(file)
    
    var messages []Message
    var totalInput, totalOutput int
    
    for scanner.Scan() {
        var entry ClaudeEntry
        json.Unmarshal(scanner.Bytes(), &entry)
        
        if entry.IsSidechain {
            continue // Skip subagent context
        }
        
        if entry.Message.Usage != nil {
            totalInput = entry.Message.Usage.InputTokens
            totalOutput = entry.Message.Usage.OutputTokens
        }
        
        messages = append(messages, Message{
            Role:      entry.Message.Role,
            Content:   extractContent(entry.Message.Content),
            Timestamp: entry.Timestamp,
        })
    }
    
    return Session{
        ID:           extractSessionID(path),
        Messages:     messages,
        InputTokens:  totalInput,
        OutputTokens: totalOutput,
    }
}
```

### OpenCode extractor

```go
func ExtractOpenCodeSessions(repoPath string) ([]Session, error) {
    storageDir := filepath.Join(os.Getenv("HOME"), ".local", "share", "opencode", "storage")
    
    // Override if env var set
    if custom := os.Getenv("OPENCODE_DATA_DIR"); custom != "" {
        storageDir = filepath.Join(custom, "storage")
    }
    
    // Scan session directories
    sessionDirs, _ := filepath.Glob(filepath.Join(storageDir, "session", "*", "ses_*.json"))
    
    var sessions []Session
    for _, sessionFile := range sessionDirs {
        var sessionInfo OpenCodeSession
        data, _ := os.ReadFile(sessionFile)
        json.Unmarshal(data, &sessionInfo)
        
        // Check if session directory matches our repo
        if sessionInfo.Directory != repoPath {
            continue
        }
        
        // Load messages for this session
        messages := loadOpenCodeMessages(storageDir, sessionInfo.ID)
        sessions = append(sessions, Session{
            ID:       sessionInfo.ID,
            Messages: messages,
        })
    }
    return sessions, nil
}
```

### Codex extractor

```go
func ExtractCodexSessions(repoPath string) ([]Session, error) {
    codexHome := os.Getenv("CODEX_HOME")
    if codexHome == "" {
        codexHome = filepath.Join(os.Getenv("HOME"), ".codex")
    }
    
    // Glob all session files
    pattern := filepath.Join(codexHome, "sessions", "*", "*", "*", "rollout-*.jsonl")
    files, _ := filepath.Glob(pattern)
    
    var sessions []Session
    for _, f := range files {
        session := parseCodexJSONL(f, repoPath)
        if session != nil {
            sessions = append(sessions, *session)
        }
    }
    return sessions, nil
}

func parseCodexJSONL(path, repoPath string) *Session {
    file, _ := os.Open(path)
    scanner := bufio.NewScanner(file)
    
    var messages []Message
    for scanner.Scan() {
        var entry map[string]interface{}
        json.Unmarshal(scanner.Bytes(), &entry)
        
        if entry["type"] == "message" {
            role, _ := entry["role"].(string)
            content := extractCodexContent(entry["content"])
            messages = append(messages, Message{Role: role, Content: content})
        }
    }
    
    if len(messages) == 0 {
        return nil
    }
    
    return &Session{
        ID:       extractSessionIDFromPath(path),
        Messages: messages,
    }
}
```

---

## Session-to-commit matching

The core challenge: **which AI session produced which commit?** Several strategies:

### Strategy 1: Temporal proximity (simple)

Match sessions where `session.lastMessageTime` is within N minutes before `commit.timestamp`:

```go
func matchSessionToCommit(sessions []Session, commitSHA string) *Session {
    commitTime := getCommitTime(commitSHA)
    
    var best *Session
    var bestDelta time.Duration = 30 * time.Minute // Max window
    
    for _, s := range sessions {
        delta := commitTime.Sub(s.LastMessageTime)
        if delta > 0 && delta < bestDelta {
            best = &s
            bestDelta = delta
        }
    }
    return best
}
```

### Strategy 2: Working directory matching

Sessions include `cwd` (Claude Code) or `directory` (OpenCode). Match sessions whose directory matches the repository:

```go
func filterSessionsByRepo(sessions []Session, repoPath string) []Session {
    var matched []Session
    for _, s := range sessions {
        if s.WorkingDirectory == repoPath || strings.HasPrefix(s.WorkingDirectory, repoPath) {
            matched = append(matched, s)
        }
    }
    return matched
}
```

### Strategy 3: File overlap analysis

Parse the commit diff for changed files, then check if the session mentioned those files in tool calls:

```go
func scoreSessionByFileOverlap(session Session, commitSHA string) float64 {
    commitFiles := getChangedFiles(commitSHA)
    sessionFiles := extractFilesFromToolCalls(session)
    
    overlap := intersect(commitFiles, sessionFiles)
    return float64(len(overlap)) / float64(len(commitFiles))
}
```

**Recommended approach:** Combine all three—filter by directory, score by file overlap, break ties by temporal proximity.

---

## Build and distribution

### Cross-compilation targets

```makefile
# Makefile
BINARY_NAME=ai-audit
VERSION=1.0.0

build-all:
	GOOS=linux GOARCH=amd64 go build -o dist/$(BINARY_NAME)-linux-amd64
	GOOS=linux GOARCH=arm64 go build -o dist/$(BINARY_NAME)-linux-arm64
	GOOS=darwin GOARCH=amd64 go build -o dist/$(BINARY_NAME)-darwin-amd64
	GOOS=darwin GOARCH=arm64 go build -o dist/$(BINARY_NAME)-darwin-arm64
	GOOS=windows GOARCH=amd64 go build -o dist/$(BINARY_NAME)-windows-amd64.exe
```

### Installation options

1. **Direct download:** GitHub releases with pre-built binaries
2. **Homebrew:** Create a tap for macOS/Linux users
3. **Go install:** `go install github.com/yourorg/ai-audit@latest`
4. **npm wrapper:** Publish npm package that downloads the correct binary (like `esbuild` does)

---

## Existing tools to learn from

| Tool | Approach | Lessons |
|------|----------|---------|
| **claude-code-transcripts** (Simon Willison) | Converts JSONL to static HTML | Pagination, gist publishing |
| **cctrace** | Export sessions with file snapshots | Manifest files, session portability |
| **SpecStory** | Wraps Claude Code to capture transcripts | `.specstory/history/` pattern |
| **git-appraise** (Google) | Full code review on git notes | JSON-per-line format, merge strategies |
| **Lefthook** | Cross-platform Git hooks | YAML config, Go binary distribution |

---

## Implementation phases

### Phase 1: Core extractors (Week 1-2)
- [ ] Claude Code JSONL parser
- [ ] OpenCode JSON walker
- [ ] Codex JSONL parser
- [ ] Session-to-commit matching algorithm
- [ ] Git notes writer

### Phase 2: CLI and hooks (Week 3)
- [ ] Cobra CLI scaffolding (`post-commit`, `extract`, `notes`)
- [ ] Hook installation command
- [ ] Cross-platform testing (Windows, macOS, Linux)

### Phase 3: HTMX viewer (Week 4)
- [ ] Go HTTP server with templates
- [ ] Timeline view with infinite scroll
- [ ] Expandable transcript details
- [ ] Search/filter functionality

### Phase 4: Polish and distribution (Week 5)
- [ ] Cross-compilation CI pipeline
- [ ] Homebrew formula
- [ ] npm wrapper package
- [ ] Documentation and examples

---

## Key technical decisions summary

| Decision | Choice | Rationale |
|----------|--------|-----------|
| **Language** | Go | Single binary, trivial cross-compile, no runtime |
| **CLI framework** | Cobra + Viper | Industry standard, excellent UX |
| **Git notes namespace** | `refs/notes/ai-transcripts` | Dedicated, avoids conflicts |
| **Note format** | Single-line JSON | Enables `cat_sort_uniq` merge strategy |
| **Hook pattern** | POSIX shell wrapper → Go binary | Works on Windows Git Bash + Unix |
| **Viewer** | Go stdlib + HTMX | No JS build step, server-rendered |
| **Session matching** | Directory + file overlap + time | Multi-signal accuracy |

This implementation plan provides all the technical details needed for a coding assistant to build the tool end-to-end.
