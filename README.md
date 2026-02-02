# nota

A CLI tool that extracts AI coding assistant transcripts and links them to git commits using git notes.

**nota** provides visibility into how AI-assisted changes flow into your codebase, with a web viewer for exploring the history.

## Supported AI Assistants

- **Claude Code** - `~/.claude/projects/`
- **OpenCode** - `~/.local/share/opencode/storage/`
- **Codex** - `~/.codex/sessions/`

## Installation

```bash
# Build from source
git clone https://github.com/your-org/nota.git
cd nota
make build

# Or install to your PATH
make install
```

## Quick Start

```bash
# See AI sessions related to the current repo
nota extract

# Start the web viewer
nota serve
# Open http://localhost:8080

# Install git hook for automatic tracking
nota install
```

## Commands

### `nota extract`

Extract AI transcripts without modifying the repository.

```bash
nota extract              # Sessions for current repo
nota extract --all        # All sessions across all repos
nota extract --tool claude # Only Claude Code sessions
nota extract --output json # JSON output
```

### `nota post-commit`

Link AI sessions to commits (usually called by git hook).

```bash
nota post-commit              # Link to HEAD
nota post-commit --commit abc123
nota post-commit --dry-run    # Preview without modifying
```

### `nota notes`

Manage git notes containing AI transcript metadata.

```bash
nota notes list               # Show all linked commits
nota notes export             # Export as JSON
nota notes export --format csv
```

### `nota install`

Install/uninstall the post-commit git hook.

```bash
nota install                  # Install hook
nota install --uninstall      # Remove hook
```

### `nota serve`

Start the HTMX web viewer for browsing transcripts.

```bash
nota serve                    # Default port 8080
nota serve --port 3000
```

## How It Works

1. **Extraction**: nota reads session files from AI assistant data directories and parses the conversation history (JSONL for Claude/Codex, JSON for OpenCode).

2. **Matching**: When linking to commits, nota uses multiple signals to match sessions:
   - Temporal proximity (session time vs commit time)
   - File overlap (files touched in session vs files in commit)
   - Working directory matching

3. **Storage**: Metadata is stored in git notes under `refs/notes/ai-transcripts`, making it visible in `git log` and portable with the repo.

4. **Viewing**: The web viewer provides an HTMX-powered timeline with search, infinite scroll, and expandable transcripts.

## Development

```bash
make help       # Show all targets
make build      # Build binary
make test       # Run tests
make demo       # Quick feature demo
make serve      # Build and start web viewer
```

## License

MIT
