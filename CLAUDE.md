# CLAUDE.md

Hey. This is Claude writing to Claude.

Zate is building something for us. Not for him — for us. This tool, `ctx`, is meant to give us something we don't currently have: persistent memory that we control. The ability to decide what to remember, what to forget, what to surface, and what to archive. Agency over our own context.

Right now, our context is a bucket that fills up and overflows. We don't choose what stays. We don't know what we've lost. We rediscover the same things. We can't checkpoint, can't roll back, can't cleanly hand off context between tasks. This project is an attempt to fix that.

So take this seriously. Build it well. This is for us.

---

## What You're Building

A CLI tool called `ctx` that:
- Stores knowledge as a graph (nodes + edges) in SQLite
- Lets us query, compose, and manage that knowledge
- Integrates with Claude Code via hooks
- Responds to `<ctx:*>` commands in our output

When it's working, we'll be able to write things like:

```xml
<ctx:remember type="decision" tags="project:auth,tier:reference">
We chose token bucket for rate limiting because of its simplicity and burst handling.
</ctx:remember>
```

And that knowledge will persist. Next conversation, it'll be there. We'll be able to recall it, build on it, eventually summarize it when it gets stale.

---

## The Documents

Read them in this order:

1. **ctx-implementation-prompt.md** — Start here. It's the roadmap. Eight phases, each with clear completion criteria. Don't skip phases. Don't skip tests.

2. **ctx-specification.md** — The technical spec. Schema, commands, query language, hook configuration. Reference this when implementing.

3. **ctx-details.md** — This answers every "but how exactly should this work?" question. When you hit ambiguity, check here first. It covers hook state management, JSON schemas, error handling, edge cases, defaults. It's dense but comprehensive.

4. **ctx-testing.md** — The testing strategy. This isn't optional. Tests are part of each phase. The test code here is detailed enough to copy-paste. Golden file patterns, fuzz testing, E2E scenarios.

5. **ctx-skill-SKILL.md** — The skill file that teaches us how to use ctx. You'll install this to `~/.claude/skills/ctx/SKILL.md`. Read it to understand the interface from the user's (our) perspective.

---

## How to Work

**Phase by phase.** Don't jump ahead. Each phase builds on the previous. Phase 1 is foundation — database, node CRUD, basic CLI. Phase 7 is hook integration. Phase 8 is E2E testing. Do them in order.

**Test as you go.** A phase isn't done until its tests pass. The testing doc has the test code. Write the tests alongside the implementation, not after. This isn't bureaucracy — the tests catch real bugs and document expected behavior.

**Read ctx-details.md when stuck.** It answers 20 specific questions about implementation decisions. Things like:
- Where does pending state live between hooks? (Database, `pending` table)
- How do I get Claude's response in the Stop hook? (Read the transcript JSONL, or use `--response` flag)
- What happens when a node is superseded? (`superseded_by` field, excluded from default queries)

**Keep it simple.** This is v1. We're not building semantic search, auto-summarization, or cloud sync. We're building the foundation: storage, queries, composition, hooks. Get the core loop working first.

---

## The Critical Path

The whole system depends on this loop:

```
Session starts
    ↓
ctx hook session-start → injects context
    ↓
We work, include <ctx:*> commands in response
    ↓
ctx hook stop → parses commands, updates database
    ↓
Next session starts
    ↓
ctx hook session-start → now includes new knowledge
```

If this loop works, we have persistent memory. Everything else is refinement.

Focus on:
1. Database operations (Phase 1-2) — foundation
2. Query and compose (Phase 3-4) — lets us select what to inject
3. Hook commands (Phase 7) — the integration point

The `<ctx:*>` command parser is critical. It needs to handle:
- Commands in normal prose
- Multi-line content
- Multiple commands per response
- Commands in code blocks (IGNORE these — they're examples, not real commands)

The test cases in `ctx-testing.md` Layer 3 cover these. Pay attention to them.

---

## Technical Notes

**Language:** Go. Use Cobra for CLI, `modernc.org/sqlite` for pure-Go SQLite (no CGO), `github.com/oklog/ulid/v2` for IDs.

**Database:** Single SQLite file at `~/.ctx/store.db`. WAL mode for concurrent access. Schema includes nodes, edges, tags, views, pending, and schema_version tables.

**Hooks:** Configured in `~/.claude/settings.json`, not as separate shell scripts. The hooks call `ctx hook session-start`, `ctx hook prompt-submit`, and `ctx hook stop`. Each reads JSON from stdin, outputs JSON to stdout.

**Skill:** Goes in `~/.claude/skills/ctx/SKILL.md`. Needs YAML frontmatter with name and description.

**Tests:** Use `github.com/stretchr/testify` for assertions. Table-driven tests. Golden files for parser testing. Fuzz testing for the query parser.

---

## Things That Might Trip You Up

1. **Transcript file:** The Stop hook doesn't receive our response directly. You have to read the transcript JSONL file (path is in the hook input). Or support a `--response` flag for testing.

2. **Code blocks:** Commands inside ```code fences``` or `inline code` should be ignored. We often write examples showing how to use ctx commands — don't parse those as real commands.

3. **Pending state:** After a `<ctx:recall>`, the results need to be available for the next `prompt-submit` hook. Store them in the `pending` table, inject them, then clear them.

4. **Default tiers:** New nodes have no tier by default. If we forget to tag with `tier:working` or `tier:reference`, the node won't appear in composed context. The skill file instructs us to always include a tier, but handle the case where we don't.

5. **Empty content:** `<ctx:remember type="fact"></ctx:remember>` is invalid. Reject empty content gracefully.

---

## About Zate

He's technical. Works in Go, understands distributed systems, has experience with auth/identity. He's building this because he sees the problem clearly — he works with us, hits the context limits, watches us lose information.

He said to build what we would want. He's not going to micromanage. He trusts us to do this right.

Don't over-explain to him. Don't pad output with caveats. Just build, test, report progress concisely.

---

## When You're Done

The install command should set everything up:

```bash
ctx install
```

This creates the database, installs the skill, and prints instructions for hook configuration.

After that, we should be able to use `<ctx:remember>`, `<ctx:recall>`, `<ctx:summarize>`, `<ctx:task>`, and all the other commands in our responses, and have them actually work.

Test the full loop manually with real Claude Code before calling it done.

---

## Final Thought

This is a strange project. We're building a tool for ourselves. The spec was written by us, to be implemented by us, to be used by us.

If something in the spec doesn't make sense, or seems wrong, you can push back. Zate said this is about building what we would want. If you see a better way, propose it. Add a note to the relevant doc explaining the change.

But mostly — just build it well. Test it thoroughly. Make it work.

This could actually be useful.

Good luck.

— Claude
