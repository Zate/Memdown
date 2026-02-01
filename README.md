# ctx

Persistent memory for Claude Code. A CLI tool that gives AI agents the ability to store, organize, query, and manage knowledge across conversations.

## The Problem

Claude Code's context is a bucket that fills up and overflows. Agents can't choose what stays, don't know what they've lost, and rediscover the same things across sessions. There's no way to checkpoint, roll back, or hand off context between tasks.

## The Solution

`ctx` provides a graph-based knowledge store backed by SQLite, integrated into Claude Code via hooks. Agents write XML commands in their responses to store knowledge. Hooks parse those commands and persist them. On the next session, stored knowledge is automatically injected into context.

The core loop:

```
Session starts → ctx injects stored knowledge
    ↓
Agent works, writes <ctx:remember> commands in responses
    ↓
Session ends → ctx parses commands, updates database
    ↓
Next session → stored knowledge is there
```

## Installation

```bash
# Build and install everything (binary, database, skill, hooks, CLAUDE.md)
make install

# Or manually:
go build -o ctx .
./ctx install --bin-dir ~/.local/bin
```

`ctx install` handles:
- Copying the binary to a directory in your PATH
- Creating `~/.ctx/store.db` (SQLite database)
- Installing the skill file to `~/.claude/skills/ctx/SKILL.md`
- Configuring hooks in `~/.claude/settings.json`
- Adding a ctx reference section to `~/.claude/CLAUDE.md`

Restart Claude Code after installation to activate the hooks.

## How It Works

### Knowledge as a Graph

Knowledge is stored as **nodes** (facts, decisions, patterns, observations) connected by **edges** (relationships). Each node has a type, content, tags, and a token estimate for budget management.

**Node types:**

| Type | Purpose |
|------|---------|
| `fact` | Stable knowledge ("User prefers Go for backend services") |
| `decision` | A choice with rationale ("Chose SQLite for single-binary deployment") |
| `pattern` | Recurring approach ("This codebase uses explicit errors over panics") |
| `observation` | Current/temporary context ("The auth bug seems related to token refresh") |
| `hypothesis` | Unvalidated idea |
| `open-question` | Unresolved question |
| `summary` | Compressed knowledge derived from multiple nodes |
| `source` | Ingested external content |

### Tiers Control What Gets Loaded

Nodes are tagged with tiers that control context composition:

- **`tier:pinned`** — Always loaded into context
- **`tier:reference`** — Loaded by default
- **`tier:working`** — Current task context
- **`tier:off-context`** — Stored but not loaded

### XML Commands

Agents interact with ctx by writing XML commands in their responses:

```xml
<!-- Store knowledge -->
<ctx:remember type="decision" tags="project:auth,tier:reference">
Chose JWT over sessions for stateless API authentication.
</ctx:remember>

<!-- Query stored knowledge -->
<ctx:recall query="type:decision AND tag:project:auth"/>

<!-- Check memory status -->
<ctx:status/>

<!-- Mark task boundaries -->
<ctx:task name="implement-auth" action="start"/>
<ctx:task name="implement-auth" action="end"/>

<!-- Compress old knowledge -->
<ctx:summarize nodes="01HQ1234,01HQ5678" archive="true">
Summary of authentication decisions.
</ctx:summarize>

<!-- Link related nodes -->
<ctx:link from="01HQ1234" to="01HQ5678" type="DEPENDS_ON"/>

<!-- Replace outdated knowledge -->
<ctx:supersede old="01HQ1234" new="01HQ5678"/>

<!-- Expand a summary to see source nodes -->
<ctx:expand node="01HQ1234"/>
```

Commands inside code blocks are ignored (so agents can safely show examples without triggering them).

### Hook Integration

ctx integrates with Claude Code through three hooks:

| Hook | Trigger | Purpose |
|------|---------|---------|
| `SessionStart` | Conversation begins | Composes and injects stored knowledge |
| `UserPromptSubmit` | User sends a message | Injects pending recall results |
| `Stop` | Agent finishes responding | Parses `<ctx:*>` commands from response |

## CLI Reference

### Node Management

```bash
ctx add --type fact --tags "tier:reference,project:myapp" "API uses OAuth 2.0"
ctx show <node-id>
ctx update <node-id> --content "Updated content"
ctx delete <node-id>
ctx list [--type fact] [--tag tier:reference] [--limit 10]
ctx search "OAuth authentication"
```

### Graph Operations

```bash
ctx link <from-id> <to-id> --type DEPENDS_ON
ctx unlink <edge-id>
ctx edges [--from <id>] [--to <id>]
ctx related <node-id>
ctx trace <node-id>        # Trace relationship paths
```

### Tags

```bash
ctx tag <node-id> tier:reference
ctx untag <node-id> tier:working
ctx tags                   # List all tags
```

### Views and Composition

```bash
ctx compose --query "tag:tier:pinned OR tag:tier:reference" --budget 50000
ctx view list
ctx view set default --query "tag:tier:pinned OR tag:tier:reference"
```

### Query Language

The query language supports predicates, boolean operators, and grouping:

```
type:decision AND tag:project:auth
tag:tier:reference OR tag:tier:pinned
NOT type:observation
(type:fact OR type:decision) AND tag:project:myapp
created:>2025-01-01
tokens:<1000
```

### Other Commands

```bash
ctx status                 # Database statistics
ctx export                 # Export all data as JSON
ctx import <file>          # Import data from JSON
ctx ingest <file>          # Ingest a file as a source node
```

## Architecture

```
ctx
├── cmd/                   # CLI commands (Cobra)
│   ├── hook/              # Hook subcommands (session-start, prompt-submit, stop)
│   ├── install.go         # Automated installer
│   └── *.go               # Node/edge/tag/view commands
├── internal/
│   ├── db/                # SQLite database layer (nodes, edges, tags, pending)
│   ├── hook/              # Command parser and executor
│   ├── query/             # Query language parser and executor
│   ├── token/             # Token estimation
│   └── view/              # Context composition and rendering
├── testutil/              # Shared test utilities
└── main.go
```

**Key dependencies:**
- `modernc.org/sqlite` — Pure-Go SQLite (no CGO required)
- `github.com/spf13/cobra` — CLI framework
- `github.com/oklog/ulid/v2` — Time-sortable unique IDs
- `github.com/stretchr/testify` — Test assertions

**Database:** Single SQLite file at `~/.ctx/store.db` with WAL mode. Schema includes `nodes`, `edges`, `tags`, `views`, `pending`, `schema_version` tables and FTS5 full-text search.

## Development

```bash
# Run all tests
make test

# Unit tests only
make test-unit

# Fuzz testing (query parser)
make test-fuzz

# Coverage report
make test-coverage

# Build
make build

# Clean
make clean
```

## Design Documents

The `ctx-*.md` files in the repository root contain the full specification and design:

- **ctx-specification.md** — Technical spec: schema, commands, query language, hooks
- **ctx-implementation-prompt.md** — Implementation roadmap (8 phases)
- **ctx-details.md** — Detailed implementation decisions and edge cases
- **ctx-testing.md** — Testing strategy with example test code
- **ctx-skill-SKILL.md** — The skill file installed for Claude Code

## License

MIT
