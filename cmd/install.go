package cmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var (
	installDryRun bool
	installForce  bool
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install ctx (binary, database, skill file, hooks)",
	Long:  "Fully automated installation: copies binary to ~/.local/bin, initializes database, installs skill file, and configures Claude Code hooks.",
	RunE:  runInstall,
}

func init() {
	installCmd.Flags().BoolVar(&installDryRun, "dry-run", false, "Preview changes without applying them")
	installCmd.Flags().BoolVar(&installForce, "force", false, "Force overwrite of existing binary")
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

// ctxHooks defines the hook entries ctx needs in settings.json.
var ctxHooks = map[string]any{
	"SessionStart": []any{
		map[string]any{
			"matcher": "",
			"hooks": []any{
				map[string]any{"type": "command", "command": "ctx hook session-start"},
			},
		},
	},
	"UserPromptSubmit": []any{
		map[string]any{
			"matcher": "",
			"hooks": []any{
				map[string]any{"type": "command", "command": "ctx hook prompt-submit"},
			},
		},
	},
	"Stop": []any{
		map[string]any{
			"matcher": "",
			"hooks": []any{
				map[string]any{"type": "command", "command": "ctx hook stop"},
			},
		},
	},
}

func runInstall(cmd *cobra.Command, args []string) error {
	home := os.Getenv("HOME")
	if home == "" {
		return fmt.Errorf("HOME environment variable not set")
	}

	prefix := ""
	if installDryRun {
		prefix = "[dry-run] "
	}

	// 1. Install binary to ~/.local/bin/ctx
	binaryResult, err := installBinary(home, prefix)
	if err != nil {
		return err
	}

	// 2. Create ~/.ctx/ and initialize database
	dbResult, err := installDatabase(home, prefix)
	if err != nil {
		return err
	}

	// 3. Install skill file
	skillResult, err := installSkill(home, prefix)
	if err != nil {
		return err
	}

	// 4. Configure hooks in ~/.claude/settings.json
	hooksResult, err := installHooks(home, prefix)
	if err != nil {
		return err
	}

	// 5. Verify installation
	fmt.Println()
	fmt.Printf("%s=== ctx installation summary ===\n", prefix)
	fmt.Printf("  Binary:   %s\n", binaryResult)
	fmt.Printf("  Database: %s\n", dbResult)
	fmt.Printf("  Skill:    %s\n", skillResult)
	fmt.Printf("  Hooks:    %s\n", hooksResult)

	// PATH check
	if !installDryRun {
		binDir := filepath.Join(home, ".local", "bin")
		if !dirInPath(binDir) {
			fmt.Println()
			fmt.Printf("  WARNING: %s is not in your PATH.\n", binDir)
			fmt.Println("  Add it to your shell profile:")
			fmt.Printf("    export PATH=\"%s:$PATH\"\n", binDir)
		}
	}

	fmt.Println()
	if installDryRun {
		fmt.Println("No changes made (dry-run mode).")
	} else {
		fmt.Println("Installation complete. Restart Claude Code to load the new hooks.")
	}

	return nil
}

// installBinary copies the current executable to ~/.local/bin/ctx.
func installBinary(home, _ string) (string, error) {
	binDir := filepath.Join(home, ".local", "bin")
	destPath := filepath.Join(binDir, "ctx")

	srcPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to determine executable path: %w", err)
	}
	srcPath, err = filepath.EvalSymlinks(srcPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve executable path: %w", err)
	}

	// Check if already installed and identical
	if !installForce {
		if same, _ := filesIdentical(srcPath, destPath); same {
			return fmt.Sprintf("%s (already up to date)", destPath), nil
		}
	}

	if installDryRun {
		if _, err := os.Stat(destPath); err == nil {
			return fmt.Sprintf("%s (would overwrite)", destPath), nil
		}
		return fmt.Sprintf("%s (would install)", destPath), nil
	}

	if err := os.MkdirAll(binDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create %s: %w", binDir, err)
	}

	if err := copyFile(srcPath, destPath, 0755); err != nil {
		return "", fmt.Errorf("failed to copy binary: %w", err)
	}

	return fmt.Sprintf("%s (installed)", destPath), nil
}

// installDatabase creates ~/.ctx/ and initializes the database.
func installDatabase(home, _ string) (string, error) {
	ctxDir := filepath.Join(home, ".ctx")
	dbPath := filepath.Join(ctxDir, "store.db")

	if installDryRun {
		if _, err := os.Stat(dbPath); err == nil {
			return fmt.Sprintf("%s (exists)", dbPath), nil
		}
		return fmt.Sprintf("%s (would create)", dbPath), nil
	}

	if err := os.MkdirAll(ctxDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create %s: %w", ctxDir, err)
	}

	d, err := openDB()
	if err != nil {
		return "", fmt.Errorf("failed to initialize database: %w", err)
	}
	d.Close()

	return fmt.Sprintf("%s (ready)", dbPath), nil
}

// installSkill writes the skill file to ~/.claude/skills/ctx/SKILL.md.
func installSkill(home, _ string) (string, error) {
	skillDir := filepath.Join(home, ".claude", "skills", "ctx")
	skillPath := filepath.Join(skillDir, "SKILL.md")

	if installDryRun {
		if _, err := os.Stat(skillPath); err == nil {
			return fmt.Sprintf("%s (would overwrite)", skillPath), nil
		}
		return fmt.Sprintf("%s (would create)", skillPath), nil
	}

	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create %s: %w", skillDir, err)
	}

	if err := os.WriteFile(skillPath, []byte(skillContent), 0644); err != nil {
		return "", fmt.Errorf("failed to write skill file: %w", err)
	}

	return fmt.Sprintf("%s (installed)", skillPath), nil
}

// installHooks merges ctx hooks into ~/.claude/settings.json.
func installHooks(home, _ string) (string, error) {
	settingsPath := filepath.Join(home, ".claude", "settings.json")

	// Read existing settings
	settings := make(map[string]any)
	existingData, err := os.ReadFile(settingsPath)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("failed to read %s: %w", settingsPath, err)
	}
	if err == nil {
		if err := json.Unmarshal(existingData, &settings); err != nil {
			return "", fmt.Errorf("failed to parse %s: %w", settingsPath, err)
		}
	}

	// Check if hooks already configured
	if hooksAlreadyConfigured(settings) {
		return fmt.Sprintf("%s (already configured)", settingsPath), nil
	}

	if installDryRun {
		return fmt.Sprintf("%s (would add SessionStart, UserPromptSubmit, Stop hooks)", settingsPath), nil
	}

	// Backup existing settings
	if len(existingData) > 0 {
		backupPath := settingsPath + ".bak"
		if err := os.WriteFile(backupPath, existingData, 0644); err != nil {
			return "", fmt.Errorf("failed to backup settings: %w", err)
		}
	}

	// Merge hooks
	mergeHooks(settings)

	// Write updated settings
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal settings: %w", err)
	}

	if err := os.WriteFile(settingsPath, append(out, '\n'), 0644); err != nil {
		return "", fmt.Errorf("failed to write %s: %w", settingsPath, err)
	}

	return fmt.Sprintf("%s (hooks added)", settingsPath), nil
}

// hooksAlreadyConfigured checks if ctx hooks are already present in settings.
func hooksAlreadyConfigured(settings map[string]any) bool {
	hooks, ok := settings["hooks"]
	if !ok {
		return false
	}
	hooksMap, ok := hooks.(map[string]any)
	if !ok {
		return false
	}

	// Check all three hook types
	for _, hookType := range []string{"SessionStart", "UserPromptSubmit", "Stop"} {
		if !hookTypeHasCtx(hooksMap, hookType) {
			return false
		}
	}
	return true
}

// hookTypeHasCtx checks if a specific hook type already has a ctx hook entry.
func hookTypeHasCtx(hooksMap map[string]any, hookType string) bool {
	entries, ok := hooksMap[hookType]
	if !ok {
		return false
	}
	arr, ok := entries.([]any)
	if !ok {
		return false
	}
	for _, entry := range arr {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		innerHooks, ok := entryMap["hooks"]
		if !ok {
			continue
		}
		innerArr, ok := innerHooks.([]any)
		if !ok {
			continue
		}
		for _, h := range innerArr {
			hMap, ok := h.(map[string]any)
			if !ok {
				continue
			}
			if cmdStr, ok := hMap["command"].(string); ok {
				if strings.Contains(cmdStr, "ctx hook") {
					return true
				}
			}
		}
	}
	return false
}

// mergeHooks adds ctx hooks to the settings map, preserving existing entries.
func mergeHooks(settings map[string]any) {
	hooks, ok := settings["hooks"]
	if !ok {
		hooks = make(map[string]any)
		settings["hooks"] = hooks
	}
	hooksMap, ok := hooks.(map[string]any)
	if !ok {
		hooksMap = make(map[string]any)
		settings["hooks"] = hooksMap
	}

	for hookType, ctxEntry := range ctxHooks {
		ctxArr, _ := ctxEntry.([]any)
		if hookTypeHasCtx(hooksMap, hookType) {
			continue
		}

		existing, ok := hooksMap[hookType]
		if !ok {
			hooksMap[hookType] = ctxArr
			continue
		}

		// Append ctx hooks to existing array
		existingArr, ok := existing.([]any)
		if !ok {
			hooksMap[hookType] = ctxArr
			continue
		}
		hooksMap[hookType] = append(existingArr, ctxArr...)
	}
}

// filesIdentical returns true if two files have the same SHA-256 hash.
func filesIdentical(path1, path2 string) (bool, error) {
	h1, err := fileHash(path1)
	if err != nil {
		return false, err
	}
	h2, err := fileHash(path2)
	if err != nil {
		return false, err
	}
	return bytes.Equal(h1, h2), nil
}

func fileHash(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

// copyFile copies src to dst with the given permissions.
func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// dirInPath checks if a directory is in the current PATH.
func dirInPath(dir string) bool {
	pathEnv := os.Getenv("PATH")
	for _, p := range filepath.SplitList(pathEnv) {
		if p == dir {
			return true
		}
	}
	return false
}

