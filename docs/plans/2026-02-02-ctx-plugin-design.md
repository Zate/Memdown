# ctx Claude Code Plugin Design

**Date:** 2026-02-02
**Status:** Approved
**Goal:** Make agents reliably use ctx by packaging it as a Claude Code plugin with enforcement mechanisms.

## Problem

ctx works as a standalone tool — hooks parse commands, store knowledge, inject on session start. But agents don't reliably participate in the memory loop. They make decisions, debug issues, and learn preferences without emitting `<ctx:remember>` commands. The memory loop only works if the agent participates.

## Decisions

- **ctx becomes a full Claude Code plugin** in the cc-plugins marketplace. Keeps ownership clean, doesn't depend on third-party plugins.
- **Auto-install binary from GitHub releases** on first SessionStart. Release infrastructure already exists.
- **Strong prompting enforcement** (not hard blocking). Modeled on superpowers' `using-superpowers` pattern with mandatory language, anti-rationalization tables, and red flags. Stop hook warns but doesn't block.
- **Plugin is the only install path.** `ctx install` becomes `ctx init` (database + migrations only). No more writing hooks to `settings.json`, no skill file installation, no CLAUDE.md injection from the Go binary.

## Plugin Structure

```
plugins/ctx/
├── .claude-plugin/
│   └── plugin.json
├── commands/
│   ├── setup.md              # Manual install/verify
│   ├── status.md             # Show memory stats
│   └── recall.md             # Ad-hoc recall query
├── hooks/
│   ├── hooks.json            # SessionStart, Stop, UserPromptSubmit
│   ├── session-start.sh      # Auto-install check + ctx hook session-start
│   ├── stop.sh               # ctx hook stop (parse commands from transcript)
│   └── prompt-submit.sh      # Inject pending recalls + nudge reminders
├── skills/
│   └── using-ctx/
│       └── SKILL.md          # Mandatory usage skill (superpowers pattern)
├── scripts/
│   ├── install-binary.sh     # Download ctx from GitHub releases
│   └── check-binary.sh       # Verify ctx is available
└── README.md
```

## Hook Behavior

### SessionStart (`session-start.sh`)

1. Check if `ctx` binary exists in PATH
2. If missing, run `install-binary.sh` (download from GitHub releases, detect OS/arch, place in `~/.local/bin/`)
3. If database doesn't exist, run `ctx init`
4. Run `ctx hook session-start` to compose and inject stored knowledge
5. Inject the `using-ctx` skill content via `additionalContext`
6. Output summary line: `ctx: 12 nodes loaded, 4800 tokens`

### Stop (`stop.sh`)

1. Run `ctx hook stop` to parse `<ctx:*>` commands from the transcript
2. If commands were found, output summary: `ctx: stored 3 nodes, 1 link`
3. If no commands were found, output warning in `systemMessage`: "No knowledge stored this session."
4. Never blocks — just warns

### UserPromptSubmit (`prompt-submit.sh`)

1. Run `ctx hook prompt-submit` to inject any pending recalls/expands
2. If nothing pending, inject a brief contextual nudge when the session has been long but nothing stored yet

## The `using-ctx` Skill

Injected at SessionStart via `additionalContext`. Short, punchy — rules and anti-rationalization, not a manual.

```markdown
# The Rule

You have persistent memory via ctx. BEFORE completing any response where you:
- Made or learned a decision -> store it
- Discovered a preference or convention -> store it
- Found a recurring pattern -> store it
- Debugging revealed a root cause -> store it
- Had an idea worth revisiting -> store it

This is not optional. You are a stateless AI. If you don't store it, it's gone.

# How to Store

<ctx:remember type="decision|fact|pattern|observation" tags="tier:reference,project:X">
Content here.
</ctx:remember>

# Red Flags

| Thought | Reality |
|---------|---------|
| "This isn't important enough" | Future you has no memory of this session |
| "I'll remember naturally" | You won't. You're stateless. |
| "This is temporary" | Temporary facts become permanent gaps |
| "The user can just tell me again" | That's wasting their time |
| "I'm just exploring, nothing to store" | Exploration findings ARE knowledge |
| "I already stored something this session" | One fact doesn't cover a whole session |

# When NOT to Use
- Trivial single-turn Q&A with no decisions or learnings
- User explicitly says not to store something
```

## Commands

### `/ctx:setup`
Manual install and verification. Checks binary, installs if missing, runs `ctx init` if database missing, shows status.

### `/ctx:status`
Quick memory overview: total nodes, nodes by type/tier, total tokens, database path.

### `/ctx:recall`
Ad-hoc query mid-session. Takes a query argument (e.g., `/ctx:recall type:decision AND tag:project:memdown`), runs it, injects results.

No `/ctx:remember` command — storing happens naturally via `<ctx:remember>` in responses. The skill enforces this.

## Binary Auto-Install (`scripts/install-binary.sh`)

1. Detect OS (`uname -s`) and arch (`uname -m`) -> map to release artifact names
2. Find writable PATH directory, prefer `~/.local/bin/`
3. Download from `https://github.com/Zate/Memdown/releases/latest/download/ctx-{os}-{arch}`
4. `chmod +x`, verify with `ctx version`
5. Run `ctx init` to create database

## Changes to Memdown Repo

- Strip `ctx install` to `ctx init` — database creation and migrations only
- Keep `ctx install` as alias for `ctx init` with deprecation notice
- No other Go code changes needed — plugin shells out to existing CLI

## Changes to cc-plugins Repo

- New `plugins/ctx/` directory with structure above
- Register in `.claude-plugin/marketplace.json` under `development` category
