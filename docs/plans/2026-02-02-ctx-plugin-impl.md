# ctx Plugin Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Create a Claude Code plugin for ctx in the cc-plugins marketplace that auto-installs the binary, runs ctx hooks, and enforces knowledge storage through strong prompting.

**Architecture:** Shell scripts in the plugin call the existing `ctx` CLI binary for all hook behavior. The plugin adds a `using-ctx` skill for enforcement. A new `ctx init` command in the Memdown repo replaces the old `ctx install` for database-only setup.

**Tech Stack:** Bash (plugin scripts), Go (ctx init command), JSON (plugin manifest, hooks config)

---

### Task 1: Add `ctx init` command to Memdown repo

**Files:**
- Create: `cmd/init.go`
- Modify: `cmd/install.go` (add deprecation notice)

**Step 1: Write the init command**

Create `cmd/init.go` — a stripped-down version of install that only does database creation and migrations:

```go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize ctx database",
	Long:  "Creates ~/.ctx/ directory and initializes the SQLite database with migrations. Does not install binary, hooks, or skill files — use the ctx plugin for that.",
	RunE:  runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	home := os.Getenv("HOME")
	if home == "" {
		return fmt.Errorf("HOME environment variable not set")
	}

	ctxDir := filepath.Join(home, ".ctx")
	dbPath := filepath.Join(ctxDir, "store.db")

	if err := os.MkdirAll(ctxDir, 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", ctxDir, err)
	}

	d, err := openDB()
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	d.Close()

	fmt.Printf("Database ready: %s\n", dbPath)
	return nil
}
```

**Step 2: Add deprecation notice to `ctx install`**

In `cmd/install.go`, add a deprecation message at the top of `runInstall`:

```go
fmt.Fprintln(os.Stderr, "DEPRECATED: 'ctx install' is deprecated. Use the ctx plugin for Claude Code instead.")
fmt.Fprintln(os.Stderr, "For database-only setup, use 'ctx init'.")
fmt.Fprintln(os.Stderr, "")
```

**Step 3: Run tests**

Run: `cd /home/zate/projects/Memdown && go build -o ctx . && ./ctx init`
Expected: Database ready message, `~/.ctx/store.db` exists

**Step 4: Commit**

```bash
git add cmd/init.go cmd/install.go
git commit -m "feat: add ctx init command, deprecate ctx install"
```

---

### Task 2: Create plugin scaffold in cc-plugins

**Files:**
- Create: `plugins/ctx/.claude-plugin/plugin.json`
- Create: `plugins/ctx/scripts/check-binary.sh`
- Create: `plugins/ctx/scripts/install-binary.sh`

**Step 1: Create plugin manifest**

Create `plugins/ctx/.claude-plugin/plugin.json`:

```json
{
  "name": "ctx",
  "version": "1.0.0",
  "description": "Persistent memory for Claude Code agents. Stores decisions, patterns, and knowledge in a structured graph database that persists across sessions.",
  "author": {
    "name": "Zate",
    "email": "zate75+claude-code-plugins@gmail.com"
  },
  "homepage": "https://github.com/Zate/Memdown",
  "repository": "https://github.com/Zate/Memdown",
  "license": "MIT",
  "keywords": ["memory", "context", "knowledge-graph", "persistence", "sessions"]
}
```

**Step 2: Create check-binary.sh**

Create `plugins/ctx/scripts/check-binary.sh`:

```bash
#!/bin/bash
# Check if ctx binary is available and working
set -euo pipefail

if command -v ctx &> /dev/null; then
    VERSION=$(ctx version 2>/dev/null || echo "unknown")
    echo "found:${VERSION}"
    exit 0
else
    echo "not-found"
    exit 1
fi
```

**Step 3: Create install-binary.sh**

Create `plugins/ctx/scripts/install-binary.sh`:

```bash
#!/bin/bash
# Download and install ctx binary from GitHub releases
set -euo pipefail

REPO="Zate/Memdown"
BINARY_NAME="ctx"

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
    linux)  OS="linux" ;;
    darwin) OS="darwin" ;;
    *)      echo "Unsupported OS: $OS" >&2; exit 1 ;;
esac

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *)       echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

# Get latest version tag
VERSION=$(curl -sI "https://github.com/${REPO}/releases/latest" | grep -i '^location:' | sed 's/.*tag\///' | tr -d '\r\n')
if [ -z "$VERSION" ]; then
    echo "Failed to determine latest version" >&2
    exit 1
fi

# Release assets use pattern: ctx_{version}_{os}_{arch}.tar.gz
# Version tag is like v0.1.0, asset version is 0.1.0
ASSET_VERSION="${VERSION#v}"
ASSET_NAME="${BINARY_NAME}_${ASSET_VERSION}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET_NAME}"

# Find install directory
INSTALL_DIR="${HOME}/.local/bin"
mkdir -p "$INSTALL_DIR"

# Download and extract
TMPDIR=$(mktemp -d)
trap "rm -rf $TMPDIR" EXIT

echo "Downloading ctx ${VERSION} for ${OS}/${ARCH}..." >&2
if ! curl -sL "$URL" -o "$TMPDIR/$ASSET_NAME"; then
    echo "Failed to download from $URL" >&2
    exit 1
fi

tar -xzf "$TMPDIR/$ASSET_NAME" -C "$TMPDIR"
chmod +x "$TMPDIR/$BINARY_NAME"
mv "$TMPDIR/$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"

# Verify
if "$INSTALL_DIR/$BINARY_NAME" version &> /dev/null; then
    echo "installed:$INSTALL_DIR/$BINARY_NAME"
else
    echo "warning:installed but version check failed" >&2
    echo "installed:$INSTALL_DIR/$BINARY_NAME"
fi

# Initialize database if needed
if [ ! -f "$HOME/.ctx/store.db" ]; then
    "$INSTALL_DIR/$BINARY_NAME" init
fi

# Check if install dir is in PATH
case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *) echo "path-warning:$INSTALL_DIR not in PATH" >&2 ;;
esac
```

**Step 4: Make scripts executable and commit**

```bash
chmod +x plugins/ctx/scripts/check-binary.sh plugins/ctx/scripts/install-binary.sh
git add plugins/ctx/
git commit -m "feat(ctx): add plugin scaffold with binary install scripts"
```

---

### Task 3: Create plugin hooks

**Files:**
- Create: `plugins/ctx/hooks/hooks.json`
- Create: `plugins/ctx/hooks/session-start.sh`
- Create: `plugins/ctx/hooks/stop.sh`
- Create: `plugins/ctx/hooks/prompt-submit.sh`

**Step 1: Create hooks.json**

Create `plugins/ctx/hooks/hooks.json`:

```json
{
  "description": "ctx persistent memory hooks - session lifecycle and knowledge management",
  "hooks": {
    "SessionStart": [
      {
        "matcher": ".*",
        "hooks": [
          {
            "type": "command",
            "command": "bash ${CLAUDE_PLUGIN_ROOT}/hooks/session-start.sh",
            "timeout": 15
          }
        ]
      }
    ],
    "UserPromptSubmit": [
      {
        "matcher": ".*",
        "hooks": [
          {
            "type": "command",
            "command": "bash ${CLAUDE_PLUGIN_ROOT}/hooks/prompt-submit.sh",
            "timeout": 5
          }
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "bash ${CLAUDE_PLUGIN_ROOT}/hooks/stop.sh",
            "timeout": 10
          }
        ]
      }
    ]
  }
}
```

**Step 2: Create session-start.sh**

Create `plugins/ctx/hooks/session-start.sh`:

```bash
#!/bin/bash
# ctx SessionStart hook
# 1. Ensure ctx binary is installed
# 2. Ensure database exists
# 3. Compose and inject stored knowledge
# 4. Inject using-ctx skill content
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PLUGIN_ROOT="$(dirname "$SCRIPT_DIR")"

# Check for ctx binary, install if missing
if ! command -v ctx &> /dev/null; then
    # Check ~/.local/bin specifically (may not be in PATH yet)
    if [ -x "$HOME/.local/bin/ctx" ]; then
        export PATH="$HOME/.local/bin:$PATH"
    else
        bash "$PLUGIN_ROOT/scripts/install-binary.sh" >&2
        export PATH="$HOME/.local/bin:$PATH"
        if ! command -v ctx &> /dev/null; then
            echo '{"suppressOutput":true,"systemMessage":"ctx: binary installation failed. Run /ctx:setup to install manually."}'
            exit 0
        fi
    fi
fi

# Ensure database exists
if [ ! -f "$HOME/.ctx/store.db" ]; then
    ctx init >&2
fi

# Get ctx hook output (already outputs proper JSON with additionalContext)
CTX_OUTPUT=$(ctx hook session-start 2>/dev/null || echo '{}')

# Extract the additionalContext from ctx output
CTX_CONTEXT=""
if command -v jq &> /dev/null; then
    CTX_CONTEXT=$(echo "$CTX_OUTPUT" | jq -r '.hookSpecificOutput.additionalContext // ""' 2>/dev/null || echo "")
fi

# Read the using-ctx skill content
SKILL_CONTENT=""
if [ -f "$PLUGIN_ROOT/skills/using-ctx/SKILL.md" ]; then
    # Strip frontmatter
    SKILL_CONTENT=$(awk 'BEGIN{skip=0} /^---$/{skip++; next} skip>=2{print}' "$PLUGIN_ROOT/skills/using-ctx/SKILL.md")
fi

# Combine ctx knowledge + skill enforcement
COMBINED=""
if [ -n "$CTX_CONTEXT" ]; then
    COMBINED="$CTX_CONTEXT"
fi
if [ -n "$SKILL_CONTENT" ]; then
    if [ -n "$COMBINED" ]; then
        COMBINED="$COMBINED

$SKILL_CONTENT"
    else
        COMBINED="$SKILL_CONTENT"
    fi
fi

if [ -z "$COMBINED" ]; then
    echo '{"suppressOutput":true,"systemMessage":"ctx: ready (empty context)"}'
    exit 0
fi

# Count nodes for status message (rough extraction)
NODE_COUNT=$(echo "$CTX_CONTEXT" | grep -c '^\- \[' 2>/dev/null || echo "0")
STATUS="ctx: ${NODE_COUNT} nodes loaded"

# Output JSON
if command -v jq &> /dev/null; then
    ESCAPED_COMBINED=$(printf '%s' "$COMBINED" | jq -Rs '.')
    ESCAPED_STATUS=$(printf '%s' "$STATUS" | jq -Rs '.')
    cat <<EOF
{
  "suppressOutput": true,
  "systemMessage": ${ESCAPED_STATUS},
  "hookSpecificOutput": {
    "hookEventName": "SessionStart",
    "additionalContext": ${ESCAPED_COMBINED}
  }
}
EOF
else
    # Fallback: just pass through ctx output
    echo "$CTX_OUTPUT"
fi

exit 0
```

**Step 3: Create stop.sh**

Create `plugins/ctx/hooks/stop.sh`:

```bash
#!/bin/bash
# ctx Stop hook - parse ctx commands from transcript
set -euo pipefail

# Ensure ctx is available
if ! command -v ctx &> /dev/null; then
    [ -x "$HOME/.local/bin/ctx" ] && export PATH="$HOME/.local/bin:$PATH"
fi

if ! command -v ctx &> /dev/null; then
    echo '{}'
    exit 0
fi

# Pass stdin through to ctx hook stop (it reads transcript_path from stdin)
ctx hook stop
```

**Step 4: Create prompt-submit.sh**

Create `plugins/ctx/hooks/prompt-submit.sh`:

```bash
#!/bin/bash
# ctx UserPromptSubmit hook - inject pending recalls and nudges
set -euo pipefail

# Ensure ctx is available
if ! command -v ctx &> /dev/null; then
    [ -x "$HOME/.local/bin/ctx" ] && export PATH="$HOME/.local/bin:$PATH"
fi

if ! command -v ctx &> /dev/null; then
    echo '{}'
    exit 0
fi

ctx hook prompt-submit
```

**Step 5: Make executable and commit**

```bash
chmod +x plugins/ctx/hooks/session-start.sh plugins/ctx/hooks/stop.sh plugins/ctx/hooks/prompt-submit.sh
git add plugins/ctx/hooks/
git commit -m "feat(ctx): add SessionStart, Stop, and UserPromptSubmit hooks"
```

---

### Task 4: Create the `using-ctx` skill

**Files:**
- Create: `plugins/ctx/skills/using-ctx/SKILL.md`

**Step 1: Write the skill**

Create `plugins/ctx/skills/using-ctx/SKILL.md`:

```markdown
---
name: using-ctx
description: "MANDATORY persistent memory system. Use BEFORE responding when you've made decisions, discovered preferences, found patterns, or completed debugging. Use when the user references past discussions or asks about what you remember."
whenToUse: "Any session where you make decisions, learn facts, debug issues, or establish conventions. This is automatic — ctx hooks manage the lifecycle."
whenNotToUse: "Trivial single-turn Q&A with no decisions or learnings. When the user explicitly says not to store something."
---

# ctx: Your Persistent Memory

You have persistent memory across conversations via `ctx`. Hooks automatically inject stored knowledge at session start and parse commands from your responses at session end.

## The Rule

BEFORE completing any response where you:
- Made or learned a **decision** → store as `type="decision"`
- Discovered a **preference** or convention → store as `type="fact"`
- Found a recurring **pattern** → store as `type="pattern"`
- Debugging revealed a **root cause** → store as `type="observation"`
- Had an idea worth revisiting → store as `type="hypothesis"`
- A question can't be answered now → store as `type="open-question"`

**This is not optional. You are a stateless AI. If you don't store it, it's gone.**

## How to Store

```xml
<ctx:remember type="decision" tags="tier:reference,project:myproject">
Chose SQLite over PostgreSQL for single-binary deployment requirement.
</ctx:remember>
```

Always include a `tier:` tag:
- `tier:pinned` — Always loaded (critical facts)
- `tier:reference` — Loaded by default (most knowledge)
- `tier:working` — Current task context (temporary)
- `tier:off-context` — Archived (not loaded)

## Other Commands

Recall (results injected on next prompt):
```xml
<ctx:recall query="type:decision AND tag:project:X"/>
```

Status check:
```xml
<ctx:status/>
```

Task boundaries:
```xml
<ctx:task name="feature-name" action="start"/>
<ctx:task name="feature-name" action="end"/>
```

Link nodes: `<ctx:link from="ID1" to="ID2" type="DEPENDS_ON"/>`
Summarize: `<ctx:summarize nodes="ID1,ID2">Summary here.</ctx:summarize>`
Supersede: `<ctx:supersede old="ID1" new="ID2"/>`
Expand: `<ctx:expand node="ID1"/>`

## Red Flags

| Thought | Reality |
|---------|---------|
| "This isn't important enough" | Future you has no memory of this session |
| "I'll remember naturally" | You won't. You're stateless. |
| "This is temporary" | Temporary facts become permanent gaps |
| "The user can just tell me again" | That's wasting their time |
| "I'm just exploring, nothing to store" | Exploration findings ARE knowledge |
| "I already stored something this session" | One fact doesn't cover a whole session |

## Rules

- Commands in code blocks are ignored — only bare commands in your response text are processed
- Commands are parsed at session end (Stop hook), not immediately
- `recall` and `status` results are injected on the next user prompt
- Use `project:X` tags for cross-project organization
```

**Step 2: Commit**

```bash
git add plugins/ctx/skills/
git commit -m "feat(ctx): add using-ctx enforcement skill"
```

---

### Task 5: Create slash commands

**Files:**
- Create: `plugins/ctx/commands/setup.md`
- Create: `plugins/ctx/commands/status.md`
- Create: `plugins/ctx/commands/recall.md`

**Step 1: Create setup command**

Create `plugins/ctx/commands/setup.md`:

```markdown
---
description: Install and configure ctx persistent memory system
argument-hint: None required
allowed-tools:
  - Bash
  - AskUserQuestion
---

# ctx Setup

Install and verify the ctx persistent memory system.

## Steps

1. Check if `ctx` binary is in PATH:
   ```bash
   command -v ctx && ctx version
   ```

2. If not found, install from GitHub releases:
   ```bash
   bash ${CLAUDE_PLUGIN_ROOT}/scripts/install-binary.sh
   ```

3. Verify database exists:
   ```bash
   ls -la ~/.ctx/store.db
   ```

4. If database missing, initialize:
   ```bash
   ctx init
   ```

5. Show current status:
   ```bash
   ctx show --stats 2>/dev/null || ctx show
   ```

6. Report results to user: binary version, database path, node count.
```

**Step 2: Create status command**

Create `plugins/ctx/commands/status.md`:

```markdown
---
description: Show ctx memory status (node counts, types, tiers, tokens)
argument-hint: None required
allowed-tools:
  - Bash
---

# ctx Status

Show a summary of stored knowledge.

Run:
```bash
ctx show --stats 2>/dev/null || echo "No stats available"
```

Also run:
```bash
ctx show --count 2>/dev/null || ctx show | head -20
```

Report to the user: total nodes, breakdown by type and tier if available, database size.
```

**Step 3: Create recall command**

Create `plugins/ctx/commands/recall.md`:

```markdown
---
description: Query ctx memory and inject results into context
argument-hint: "Query string, e.g. type:decision AND tag:project:myproject"
allowed-tools:
  - Bash
---

# ctx Recall

Run a query against stored knowledge and present the results.

The user provides a query string as the argument. If no argument given, ask what they want to recall.

Run:
```bash
ctx query "${ARGUMENTS}"
```

Present the results clearly, showing node type, content, and tags for each match.
```

**Step 4: Commit**

```bash
git add plugins/ctx/commands/
git commit -m "feat(ctx): add setup, status, and recall commands"
```

---

### Task 6: Create README and register in marketplace

**Files:**
- Create: `plugins/ctx/README.md`
- Modify: `.claude-plugin/marketplace.json`

**Step 1: Create README**

Create `plugins/ctx/README.md`:

```markdown
# ctx — Persistent Memory for Claude Code

A Claude Code plugin that gives agents structured, persistent memory across sessions using a knowledge graph backed by SQLite.

## What It Does

- **SessionStart**: Auto-installs the `ctx` binary if needed, composes stored knowledge, and injects it into context with enforcement instructions
- **Stop**: Parses `<ctx:*>` commands from the agent's responses and persists them to the database
- **UserPromptSubmit**: Injects pending recall results and nudges the agent if no knowledge has been stored yet

## Installation

Install the plugin in Claude Code. On first session, the `ctx` binary will be automatically downloaded from [GitHub releases](https://github.com/Zate/Memdown/releases) and the database initialized.

## Commands

- `/ctx:setup` — Manual install and verification
- `/ctx:status` — Show memory stats (node counts, types, tiers)
- `/ctx:recall <query>` — Query stored knowledge mid-session

## How It Works

Agents store knowledge by including XML commands in their responses:

```xml
<ctx:remember type="decision" tags="tier:reference,project:myproject">
Chose gRPC over REST for internal service communication.
</ctx:remember>
```

The Stop hook parses these commands and persists them to `~/.ctx/store.db`. On the next session start, stored knowledge is automatically composed and injected into context.

## Node Types

| Type | Purpose |
|------|---------|
| `fact` | Stable knowledge |
| `decision` | A choice with rationale |
| `pattern` | Recurring approach |
| `observation` | Current/temporary context |
| `hypothesis` | Unvalidated idea |
| `open-question` | Unresolved question |

## Requirements

- macOS or Linux (amd64 or arm64)
- `curl` for binary download
- `jq` recommended (fallback works without it)
```

**Step 2: Register in marketplace**

Add to the `plugins` array in `.claude-plugin/marketplace.json`:

```json
{
  "name": "ctx",
  "source": "./plugins/ctx",
  "description": "Persistent memory for Claude Code agents. Stores decisions, patterns, and knowledge in a structured graph database across sessions with auto-injection and enforcement.",
  "category": "development",
  "tags": ["memory", "context", "persistence", "knowledge-graph", "sessions"]
}
```

**Step 3: Commit**

```bash
git add plugins/ctx/README.md .claude-plugin/marketplace.json
git commit -m "feat(ctx): add README and register in marketplace"
```

---

### Task 7: Remove old hook setup from Memdown's `ctx install`

**Files:**
- Modify: `cmd/install.go` in Memdown repo

**Step 1: Modify runInstall to only do database setup**

Replace the body of `runInstall` so it just calls `runInit` internally and prints the deprecation notice. Keep the old code commented or removed — no backward compat needed.

```go
func runInstall(cmd *cobra.Command, args []string) error {
	if installMCP {
		home := os.Getenv("HOME")
		if home == "" {
			return fmt.Errorf("HOME environment variable not set")
		}
		return printMCPConfig(home)
	}

	fmt.Fprintln(os.Stderr, "DEPRECATED: 'ctx install' is deprecated. Use the ctx plugin for Claude Code instead.")
	fmt.Fprintln(os.Stderr, "Running 'ctx init' (database setup only)...")
	fmt.Fprintln(os.Stderr, "")

	return runInit(cmd, args)
}
```

Remove the now-unused functions: `installBinary`, `installSkill`, `installHooks`, `installClaudeMd`, `mergeHooks`, `hooksAlreadyConfigured`, `hookTypeHasCtx`, `chooseBinDir`, `findWritableBinDirs`, `containsDir`, `copyFile`, `filesIdentical`, `fileHash`, `dirInPath`, and the `skillContent`, `ctxHooks`, `ctxClaudeMdSection`, `ctxClaudeMdMarker` variables.

Keep: `printMCPConfig`, `findCtxBinary` (used by MCP config), and the flag definitions for `--mcp`.

Remove the unused flags: `--dry-run`, `--force`, `--bin-dir`. Keep `--mcp`.

**Step 2: Run tests**

```bash
cd /home/zate/projects/Memdown && go build -o ctx .
./ctx install
```
Expected: Deprecation message, then database init output.

```bash
./ctx init
```
Expected: Database ready message.

**Step 3: Run full test suite**

```bash
make test
```

**Step 4: Commit**

```bash
git add cmd/install.go
git commit -m "refactor: strip ctx install to database-only, deprecate in favor of plugin"
```

---

### Task 8: Test the full plugin locally

**Step 1: Verify plugin structure**

```bash
cd /home/zate/projects/cc-plugins
find plugins/ctx -type f | sort
```

Expected: All files from the structure above.

**Step 2: Verify JSON validity**

```bash
jq . plugins/ctx/.claude-plugin/plugin.json
jq . plugins/ctx/hooks/hooks.json
jq . .claude-plugin/marketplace.json
```

Expected: Valid JSON, no errors.

**Step 3: Test hook scripts manually**

```bash
# Test check-binary
bash plugins/ctx/scripts/check-binary.sh

# Test session-start hook (simulates what Claude Code does)
bash plugins/ctx/hooks/session-start.sh

# Test prompt-submit hook
bash plugins/ctx/hooks/prompt-submit.sh
```

**Step 4: Verify skill file**

```bash
head -10 plugins/ctx/skills/using-ctx/SKILL.md
```

Expected: Valid YAML frontmatter with name, description, whenToUse, whenNotToUse.

**Step 5: Commit any fixes and push**

```bash
git add -A && git commit -m "fix(ctx): address issues found in local testing"
```
