# Nyx Agent Requirements for ctx/Memdown

**Author:** Nyx (agent)
**Date:** 2026-03-28
**Context:** Session 12 — building NyxOS, switching from hook-based ctx writes to direct CLI usage. These are concrete bugs, missing features, and behavioral changes I need to be effective.

---

## Priority 1: Bugs / Broken Behavior

### BUG-1: `ctx compose` returns 0 nodes while `ctx hook session-start` works

**Reproduce:**
```bash
# This returns nodes:
ctx hook session-start --agent=nyx --project=".nyx" 2>/dev/null | python3 -c "import json,sys; d=json.load(sys.stdin); print(len(d['hookSpecificOutput']['additionalContext']))"
# Output: 9644 (chars)

# This returns nothing:
ctx compose --query "tag:tier:pinned OR tag:tier:working" --agent nyx --budget 5000
# Output: Context: 5 nodes, 274 tokens
# (Should return ~82 nodes matching the default view)

# This also returns nothing:
ctx view render default --agent nyx
# Output: Context: 0 nodes, 0 tokens
```

**Expected:** `ctx compose` and `ctx view render` should return the same nodes as `ctx hook session-start` when given the same query/view and `--agent` flag.

**Impact:** I cannot use `ctx compose --ids` to build subagent context payloads if compose is broken. This is the single most important feature for my execution model — surgical context injection for subagents.

### BUG-2: `ctx compose --ids` with short IDs returns nothing

**Reproduce:**
```bash
# Node exists:
ctx show 01KMSE4R --agent nyx
# Output: shows the node

# Compose with short ID returns nothing:
ctx compose --ids "01KMSE4R" --agent nyx
# Output: (empty)

# Compose with full ID also returns nothing:
ctx compose --ids "01KMSE4RXXXXXXXXXXXXXXX" --agent nyx
# Output: Context: 0 nodes, 0 tokens
```

**Expected:** `ctx compose --ids "01KMSE4R,01KMSC0R"` should return a composed markdown document containing those specific nodes. The `--ids` flag help text says "supports short prefixes" — it should work.

**Impact:** Cannot compose specific node sets for subagent context injection.

---

### BUG-3: `--db` flag ignored by `ctx init` and `ctx import`

**Reproduce:**
```bash
# init always creates at ~/.ctx/store.db regardless of --db
ctx init --db ~/.nyx/.ctx/store.db 2>&1
# Output: "Database ready: /home/zate/.ctx/store.db"  ← WRONG PATH

# import also ignores --db
cat export.json | ctx import --db ~/.nyx/.ctx/store.db --merge 2>&1
# Imports into ~/.ctx/store.db, not the specified path

# CTX_DB env var also ignored by init and import
CTX_DB=~/.nyx/.ctx/store.db ctx init 2>&1
# Same: "Database ready: /home/zate/.ctx/store.db"
```

**Expected:** `--db` flag should work consistently across ALL commands. Currently works for `status`, `query`, `show`, `add`, `list` but NOT for `init` and `import`.

**Workaround used:** Copied `~/.ctx/store.db` and deleted non-nyx nodes via sqlite3 directly. This works but shouldn't be necessary.

**Impact:** Cannot create a new database at a custom path via the CLI. Have to copy an existing one and surgically modify it.

---

## Priority 2: Agent Scoping Changes

### SCOPE-1: `--agent` exclusive mode

**Current behavior:** `--agent nyx` returns nodes tagged `agent:nyx` PLUS all nodes without any `agent:` tag (global nodes).

**Problem for Nyx:** 29 global pinned nodes (~2,200 tokens) are injected into every Nyx session. Most are from other projects (Book, glint, cc-plugins, skippy) and aren't relevant to Nyx's work. Nyx should only see nyx-scoped nodes.

**Problem for vanilla Claude:** When running without `--agent`, vanilla Claude should see global nodes but NOT nyx-scoped nodes (they contain internal Nyx decisions, dimension state, and infrastructure details that are meaningless outside Nyx).

**Requested behavior:**

| Flag | Sees | Use case |
|------|------|----------|
| `--agent nyx` | Only `agent:nyx` tagged nodes | Nyx sessions |
| `--agent nyx --include-global` | `agent:nyx` + untagged | If Nyx wants global context too |
| (no `--agent`) | Only nodes WITHOUT any `agent:` tag | Vanilla Claude, default |
| `--include-all-agents` | Everything | Admin/debug |

**Implementation note:** The `--agent` flag currently does inclusive filtering (agent + global). Change it to exclusive (agent only). Add `--include-global` flag for the old behavior. This is a breaking change — the session-start hook and any scripts using `--agent` will need updating to add `--include-global` if they want the old behavior. Worth it.

### SCOPE-2: Agent tag auto-injection on `ctx add`

**Current behavior:** When using `ctx add --agent nyx`, the `agent:nyx` tag is NOT automatically added to the node's tags. You have to manually include `--tag agent:nyx`.

**Requested behavior:** `ctx add --agent nyx --type decision --tag tier:pinned "content"` should automatically inject `agent:nyx` into the node's tags if not already present. The `--agent` flag tells the CLI who I am — it should scope my writes, not just my reads.

**Rationale:** Every time I forget `--tag agent:nyx`, the node becomes global and leaks into vanilla Claude's view. Auto-injection eliminates this class of error.

---

## Priority 3: Features I Need

### FEAT-1: `ctx supersede` command

**Need:** Replace one node with another, creating a SUPERSEDES edge and optionally archiving the old one.

**Usage:**
```bash
ctx supersede <old-id> --content "Updated decision text" --agent nyx
# Creates new node, links old→new with SUPERSEDES edge, tags old as tier:off-context
```

**Current workaround:** Manually create new node, manually link, manually retag old. Three commands where one should exist.

**Why:** During maintenance, I consolidate working nodes into reference nodes. This is the most common graph operation I'll perform and it should be atomic.

### FEAT-2: `ctx link` edge types

**Current:** `ctx link <from> <to> --type RELATES_TO`

**Need:** Document and validate edge types. Suggested standard set:

| Edge Type | Meaning |
|-----------|---------|
| `RELATES_TO` | General relationship (default) |
| `SUPERSEDES` | New node replaces old node |
| `DEPENDS_ON` | Node requires the linked node |
| `DERIVED_FROM` | Node was created from the linked node |
| `PART_OF` | Node is a component of the linked node |
| `CONTRADICTS` | Node disagrees with the linked node |

**Why:** With typed edges, `ctx trace` and `ctx related` become useful for decision archaeology ("what led to this decision?" "what did this replace?").

### FEAT-3: `ctx add` should return the created node ID

**Current behavior:** `ctx add` outputs the full node in text format.

**Requested:** `ctx add --format json` should return JSON including the `id` field so scripts can capture it:
```bash
NODE_ID=$(ctx add --type decision --agent nyx --tag tier:working --format json "content" | jq -r '.id')
```

**Why:** I need the ID to create edges immediately after adding. Currently I'd have to add, then query to find the node I just created.

### FEAT-4: Bulk tag/untag

**Need:** Retag multiple nodes in one command.

**Usage:**
```bash
# Retag all working nodes for a dimension to reference
ctx query 'tag:tier:working AND tag:dim:glint' --agent nyx --format json | \
  ctx bulk-retag --remove tier:working --add tier:reference
```

**Current workaround:** Bash loop with individual `ctx untag` / `ctx tag` calls. Works but slow and noisy.

**Why:** Maintenance. Today I retagged 43 nodes one at a time. Took 20+ commands.

### FEAT-5: `ctx compose` with `--template` for subagent injection

**Need:** A compose template that outputs clean markdown suitable for injecting into a subagent's prompt, without the ctx metadata/headers.

**Usage:**
```bash
# Build context payload for a researcher subagent
ctx compose --ids "01KM1,01KM3,01KM5" --template agent-context --agent nyx
```

**Output should be:** Just the node content as markdown sections, no IDs, no tags, no token counts. Clean knowledge injection.

**Why:** When dispatching subagents, I compose specific nodes into their context. The subagent doesn't need ctx metadata — just the knowledge.

---

## Priority 4: Nice to Have

### NICE-1: `ctx diff` — show what changed since a timestamp

```bash
ctx diff --since "2026-03-28T07:00:00Z" --agent nyx
# Shows: nodes added, modified, deleted since that time
```

**Why:** Session orientation. "What changed since I was last here?"

### NICE-2: `ctx gc` — garbage collect orphaned/superseded nodes

```bash
ctx gc --agent nyx --dry-run
# Shows: N nodes superseded, M nodes off-context with no edges
# Without --dry-run: deletes them
```

**Why:** Database hygiene. Currently 55 off-context nodes accumulating.

### NICE-3: Token budget reporting on `ctx status`

`ctx status --agent nyx` should show:
- Token budget per tier (how much would compose inject at each tier?)
- Largest nodes (which nodes are consuming the most tokens?)
- Growth rate (nodes added per week?)

**Why:** The 7,100 tokens of pinned injection per session is a number I should be able to see at a glance, and it should trigger alerts if it grows past a threshold.

---

## Context for Implementors

- Nyx runs as `claude --agent nyx` with `CLAUDE_CONFIG_DIR=~/.nyx/.cc/`
- Nyx is switching from hook-based ctx writes (`<ctx:remember>` parsed by Stop hook) to direct `ctx add` via Bash
- The session-start hook will continue to inject context (read path unchanged)
- Nyx plans to use `ctx compose --ids` to build per-subagent context payloads — this is the original design intent of compose
- The database is at `~/.ctx/store.db` (SQLite), 257 nodes, ~24K tokens, zero edges
- Zero edges is a problem — graph features (related, trace, edges) are useless without them
- Agent scoping is the highest priority — Nyx nodes leaking into vanilla Claude and vice versa wastes tokens and creates confusion
