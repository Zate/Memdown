# ctx: Persistent Memory System

This file should be installed to `~/.claude/skills/ctx/SKILL.md`

```markdown
---
name: ctx
description: Persistent memory system for managing context across conversations. Use when you need to remember information, recall past knowledge, manage task context, or when the user references past discussions or asks about what you remember.
---

# ctx: Your Persistent Memory System

You have access to a persistent memory system. This allows you to store, organize, query, and manage knowledge across conversations.

## Why This Exists

Your context window is limited and ephemeral. Without this system, knowledge accumulates until it overflows, then gets truncated or summarized in ways you don't control. You lose work. You re-discover things. You can't distinguish "we never discussed X" from "we discussed X but it's gone."

This system gives you agency over your memory.

## Core Concepts

### Nodes

A node is a piece of knowledge. Every node has:
- **ID**: A unique identifier (ULID)
- **Type**: What kind of knowledge this is
- **Content**: The actual information
- **Tags**: Labels for organization and querying
- **Metadata**: Additional structured data

### Node Types

| Type | Purpose | Example |
|------|---------|---------|
| `fact` | Stable knowledge | "User prefers Go for backend services" |
| `decision` | A choice with rationale | "Chose SQLite for single-binary requirement" |
| `pattern` | Recurring approach | "This codebase uses explicit errors over panics" |
| `observation` | Current/temporary context | "The auth bug seems related to token refresh" |
| `hypothesis` | Unvalidated idea | "Maybe the race condition is in cache invalidation" |
| `open-question` | Unresolved question | "How should federation handoff work?" |
| `summary` | Compressed knowledge | Derived from multiple source nodes |
| `source` | External content | Ingested file or document |

### Tiers

Tiers control what gets loaded into your context:

| Tier | Behavior | Use For |
|------|----------|---------|
| `tier:pinned` | Always loaded | Critical facts, active constraints |
| `tier:reference` | Loaded by default | Stable background knowledge |
| `tier:working` | Loaded for current task | Active task context |
| `tier:off-context` | Stored, not loaded | Archived work, source material |

### Edges

Nodes connect via typed relationships:

| Edge | Meaning |
|------|---------|
| `DERIVED_FROM` | This was created by summarizing those |
| `DEPENDS_ON` | This conclusion relies on that premise |
| `SUPERSEDES` | This replaces/corrects that |
| `RELATES_TO` | General association |

## Commands

Issue commands using XML tags in your responses. They are processed after you respond.

### Remember

Store knowledge:

```xml
<ctx:remember type="fact" tags="project:myproject,tier:reference">
The API uses OAuth 2.0 with PKCE for public clients.
</ctx:remember>
```

Parameters:
- `type` (required): Node type
- `tags` (optional): Comma-separated tags

### Recall

Query stored knowledge:

```xml
<ctx:recall query="type:decision AND tag:project:myproject"/>
```

Results appear in your next context. The query language supports:
- `type:<type>` — Match node type
- `tag:<tag>` — Match tag
- `created:>24h` — Time filters
- `AND`, `OR`, `NOT` — Boolean logic
- `(parentheses)` — Grouping

### Summarize

Compress multiple nodes into one:

```xml
<ctx:summarize nodes="01HQ1234,01HQ5678" archive="true">
The authentication system uses OIDC with custom claims for tenant isolation.
Refresh tokens are stored server-side only.
</ctx:summarize>
```

Parameters:
- `nodes` (required): Comma-separated node IDs to summarize
- `archive` (optional): If "true", sources get tagged `tier:off-context`

This creates a new summary node with `DERIVED_FROM` edges to the sources.

### Link

Create relationships:

```xml
<ctx:link from="01HQ1234" to="01HQ5678" type="DEPENDS_ON"/>
```

### Supersede

Mark a node as replaced:

```xml
<ctx:supersede old="01HQ1234" new="01HQ5678"/>
```

The old node remains but is excluded from default queries.

### Expand

Bring source nodes of a summary back into context:

```xml
<ctx:expand node="01HQ1234"/>
```

### Status

Check your memory state:

```xml
<ctx:status/>
```

Returns: node counts by type/tier, token estimates, what's loaded vs stored.

### Task Boundaries

Start a focused task:

```xml
<ctx:task name="implement-auth" action="start"/>
```

End and optionally summarize:

```xml
<ctx:task name="implement-auth" action="end" summarize="true"/>
```

Ending a task:
- Archives working context for that task
- Promotes key decisions to reference tier
- Cleans up scratch observations

## Patterns and Practices

### When to Remember

**Do remember:**
- Decisions and their rationale
- User preferences and constraints
- Project-specific patterns
- Key conclusions from exploration
- Open questions to revisit

**Don't remember:**
- Transient debugging output
- Routine confirmations
- Things already in reference material
- Highly context-dependent observations

### Crystallizing Decisions

When you and the user decide something, capture it:

```xml
<ctx:remember type="decision" tags="project:X,tier:reference">
Using PostgreSQL instead of SQLite.
Rationale: Multi-tenant requirement needs concurrent write access.
Trade-off: Adds deployment dependency but necessary for scale.
</ctx:remember>
```

### Task Workflow

1. Start task: `<ctx:task name="feature-X" action="start"/>`
2. Add observations as you work with `tier:working`
3. Make decisions, tag with `tier:reference`
4. End task: `<ctx:task name="feature-X" action="end" summarize="true"/>`

### Managing Budget

If context is getting crowded:

1. Check status: `<ctx:status/>`
2. Identify what can be summarized
3. Summarize with archive: `<ctx:summarize ... archive="true"/>`
4. Demote verbose content: tag with `tier:off-context`

### Provenance

When you summarize, sources are linked. Later you can:
- Trace back: see what a summary came from
- Expand: bring sources back if you need detail
- Validate: check if sources are still accurate

## What's Loaded Now

Your current context was composed from:
- All `tier:pinned` nodes
- All `tier:reference` nodes  
- All `tier:working` nodes for the active task

Nodes tagged `tier:off-context` are stored but not loaded. Use `<ctx:recall>` to query them.

## Tips

1. **Be specific in types**: `decision` vs `observation` vs `fact` matters for later queries.

2. **Use project tags**: `tag:project:X` keeps things organized across multiple projects.

3. **Summarize proactively**: Don't wait for context overflow. When a thread concludes, compress it.

4. **Check your state**: Use `<ctx:status/>` to understand what you're carrying.

5. **Trust the system**: Off-context doesn't mean gone. You can always recall or expand.

6. **Note open questions**: `type:open-question` helps you track unresolved issues.

7. **Link dependencies**: If conclusion B depends on fact A, link them. If A changes, you'll know B might be stale.
```

## Installation Notes

The skill file above should be saved to `~/.claude/skills/ctx/SKILL.md`.

The `ctx install` command will handle this automatically, but you can also manually:

1. Create the directory: `mkdir -p ~/.claude/skills/ctx`
2. Copy this file (without the outer code fence) to `~/.claude/skills/ctx/SKILL.md`
3. Add hooks to your `~/.claude/settings.json` (see ctx-specification.md for hook configuration)
