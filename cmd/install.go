package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install ctx (database, skill file, hook instructions)",
	RunE:  runInstall,
}

func init() {
	rootCmd.AddCommand(installCmd)
}

var skillContent = `---
name: ctx
description: Persistent memory system for managing context across conversations. Use when you need to remember information, recall past knowledge, manage task context, or when the user references past discussions or asks about what you remember.
---

# ctx: Your Persistent Memory System

You have access to a persistent memory system. This allows you to store, organize, query, and manage knowledge across conversations.

## Core Concepts

### Node Types

| Type | Purpose | Example |
|------|---------|---------|
| ` + "`fact`" + ` | Stable knowledge | "User prefers Go for backend services" |
| ` + "`decision`" + ` | A choice with rationale | "Chose SQLite for single-binary requirement" |
| ` + "`pattern`" + ` | Recurring approach | "This codebase uses explicit errors over panics" |
| ` + "`observation`" + ` | Current/temporary context | "The auth bug seems related to token refresh" |
| ` + "`hypothesis`" + ` | Unvalidated idea | "Maybe the race condition is in cache invalidation" |
| ` + "`open-question`" + ` | Unresolved question | "How should federation handoff work?" |
| ` + "`summary`" + ` | Compressed knowledge | Derived from multiple source nodes |
| ` + "`source`" + ` | External content | Ingested file or document |

### Tiers

Tag nodes with tiers to control context composition:

- ` + "`tier:pinned`" + ` — Always loaded
- ` + "`tier:reference`" + ` — Loaded by default
- ` + "`tier:working`" + ` — Current task context
- ` + "`tier:off-context`" + ` — Stored, not loaded

### Commands

Issue commands using XML tags in your responses. They are processed after you respond.

**Remember:**
` + "```" + `xml
<ctx:remember type="fact" tags="project:myproject,tier:reference">
The API uses OAuth 2.0 with PKCE for public clients.
</ctx:remember>
` + "```" + `

**Recall:**
` + "```" + `xml
<ctx:recall query="type:decision AND tag:project:myproject"/>
` + "```" + `

**Summarize:**
` + "```" + `xml
<ctx:summarize nodes="01HQ1234,01HQ5678" archive="true">
Summary content here.
</ctx:summarize>
` + "```" + `

**Link:**
` + "```" + `xml
<ctx:link from="01HQ1234" to="01HQ5678" type="DEPENDS_ON"/>
` + "```" + `

**Status:**
` + "```" + `xml
<ctx:status/>
` + "```" + `

**Task boundaries:**
` + "```" + `xml
<ctx:task name="implement-auth" action="start"/>
<ctx:task name="implement-auth" action="end" summarize="true"/>
` + "```" + `

**Expand summary:**
` + "```" + `xml
<ctx:expand node="01HQ1234"/>
` + "```" + `

**Supersede:**
` + "```" + `xml
<ctx:supersede old="01HQ1234" new="01HQ5678"/>
` + "```" + `

### Best Practices

1. Always include a tier tag when remembering
2. Use task boundaries to organize working memory
3. Summarize proactively when a thread concludes
4. Use project tags for cross-project organization
5. Check status periodically
`

func runInstall(cmd *cobra.Command, args []string) error {
	home := os.Getenv("HOME")

	// 1. Create ~/.ctx/ and initialize database
	ctxDir := filepath.Join(home, ".ctx")
	if err := os.MkdirAll(ctxDir, 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", ctxDir, err)
	}

	d, err := openDB()
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	d.Close()

	// 2. Install skill file
	skillDir := filepath.Join(home, ".claude", "skills", "ctx")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", skillDir, err)
	}

	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(skillContent), 0644); err != nil {
		return fmt.Errorf("failed to write skill file: %w", err)
	}

	fmt.Println("ctx installed successfully!")
	fmt.Println()
	fmt.Printf("Database: %s\n", filepath.Join(ctxDir, "store.db"))
	fmt.Printf("Skill: %s\n", skillPath)
	fmt.Println()
	fmt.Println("To enable hooks, add to ~/.claude/settings.json:")
	fmt.Println()
	fmt.Println(`{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "",
        "hooks": [{"type": "command", "command": "ctx hook session-start"}]
      }
    ],
    "UserPromptSubmit": [
      {
        "matcher": "",
        "hooks": [{"type": "command", "command": "ctx hook prompt-submit"}]
      }
    ],
    "Stop": [
      {
        "matcher": "",
        "hooks": [{"type": "command", "command": "ctx hook stop"}]
      }
    ]
  }
}`)
	fmt.Println()
	fmt.Println("Then restart Claude Code to load the new configuration.")

	return nil
}
