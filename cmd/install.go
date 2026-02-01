package cmd

import (
	"bufio"
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
	installBinDir string
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
	installCmd.Flags().StringVar(&installBinDir, "bin-dir", "", "Directory to install binary into (skips interactive prompt)")
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

	// 1. Determine binary install location and install
	binDir, err := chooseBinDir(home)
	if err != nil {
		return err
	}

	binaryResult, err := installBinary(binDir)
	if err != nil {
		return err
	}

	// 2. Create ~/.ctx/ and initialize database
	dbResult, err := installDatabase(home)
	if err != nil {
		return err
	}

	// 3. Install skill file
	skillResult, err := installSkill(home)
	if err != nil {
		return err
	}

	// 4. Configure hooks in ~/.claude/settings.json
	hooksResult, err := installHooks(home)
	if err != nil {
		return err
	}

	// 5. Add ctx section to ~/.claude/CLAUDE.md
	claudeMdResult, err := installClaudeMd(home)
	if err != nil {
		return err
	}

	// 6. Verify installation
	fmt.Println()
	if installDryRun {
		fmt.Println("[dry-run] === ctx installation summary ===")
	} else {
		fmt.Println("=== ctx installation summary ===")
	}
	fmt.Printf("  Binary:   %s\n", binaryResult)
	fmt.Printf("  Database: %s\n", dbResult)
	fmt.Printf("  Skill:    %s\n", skillResult)
	fmt.Printf("  Hooks:    %s\n", hooksResult)
	fmt.Printf("  CLAUDE.md: %s\n", claudeMdResult)

	// PATH check — warn if the chosen directory isn't in PATH
	if !installDryRun && !dirInPath(binDir) {
		fmt.Println()
		fmt.Printf("  WARNING: %s is not in your PATH.\n", binDir)
		fmt.Println("  Add it to your shell profile:")
		fmt.Printf("    export PATH=\"%s:$PATH\"\n", binDir)
	}

	fmt.Println()
	if installDryRun {
		fmt.Println("No changes made (dry-run mode).")
	} else {
		fmt.Println("Installation complete. Restart Claude Code to load the new hooks.")
	}

	return nil
}

// chooseBinDir determines where to install the ctx binary.
// Priority: --bin-dir flag > interactive prompt (scanning PATH for writable dirs).
func chooseBinDir(home string) (string, error) {
	// If explicitly provided via flag, use it directly.
	if installBinDir != "" {
		return installBinDir, nil
	}

	// Find candidate directories from PATH that are writable.
	candidates := findWritableBinDirs(home)

	// Always include ~/.local/bin as a fallback even if not currently in PATH.
	localBin := filepath.Join(home, ".local", "bin")
	if !containsDir(candidates, localBin) {
		candidates = append(candidates, localBin)
	}

	if len(candidates) == 0 {
		return localBin, nil
	}

	// Pick the best default: prefer ~/.local/bin, then /usr/local/bin, then first writable.
	best := candidates[0]
	for _, c := range candidates {
		if c == localBin {
			best = c
			break
		}
		if c == "/usr/local/bin" && best != localBin {
			best = c
		}
	}

	// In dry-run mode, don't prompt — just use the best candidate.
	if installDryRun {
		return best, nil
	}

	// Prompt the user.
	fmt.Printf("Install ctx binary to: %s\n", best)
	if len(candidates) > 1 {
		fmt.Println("Other directories in your PATH:")
		for i, c := range candidates {
			if c == best {
				continue
			}
			fmt.Printf("  %s\n", c)
			if i >= 5 {
				fmt.Printf("  ... (%d more)\n", len(candidates)-i-1)
				break
			}
		}
	}
	fmt.Print("Press Enter to accept, or type a different path: ")

	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)

	if line != "" {
		return line, nil
	}
	return best, nil
}

// findWritableBinDirs returns directories from PATH that exist and are writable by the current user.
func findWritableBinDirs(home string) []string {
	seen := make(map[string]bool)
	var dirs []string

	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" || seen[dir] {
			continue
		}
		seen[dir] = true

		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}

		// Check writability by trying to open a temp file.
		probe := filepath.Join(dir, ".ctx-probe-"+fmt.Sprintf("%d", os.Getpid()))
		f, err := os.OpenFile(probe, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			continue
		}
		f.Close()
		os.Remove(probe)

		dirs = append(dirs, dir)
	}
	return dirs
}

func containsDir(dirs []string, target string) bool {
	for _, d := range dirs {
		if d == target {
			return true
		}
	}
	return false
}

// installBinary copies the current executable to the chosen bin directory.
func installBinary(binDir string) (string, error) {
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
func installDatabase(home string) (string, error) {
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
func installSkill(home string) (string, error) {
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
func installHooks(home string) (string, error) {
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

// ctxClaudeMdSection is the content added to ~/.claude/CLAUDE.md.
const ctxClaudeMdMarker = "<!-- ctx:claude-md -->"

var ctxClaudeMdSection = ctxClaudeMdMarker + `
## Persistent Memory (ctx)

You have persistent memory across conversations via ` + "`ctx`" + `. It is already configured and active — hooks automatically inject your stored knowledge at session start and parse commands from your responses.

### Quick Reference

Store knowledge (processed after you respond):
` + "```" + `xml
<ctx:remember type="fact|decision|pattern|observation" tags="tier:reference,project:X">
Content to remember.
</ctx:remember>
` + "```" + `

Retrieve knowledge (results injected on next prompt):
` + "```" + `xml
<ctx:recall query="type:decision AND tag:project:X"/>
` + "```" + `

Other commands: ` + "`<ctx:status/>`" + `, ` + "`<ctx:task name=\"X\" action=\"start|end\"/>`" + `, ` + "`<ctx:summarize>`" + `, ` + "`<ctx:link>`" + `, ` + "`<ctx:supersede>`" + `, ` + "`<ctx:expand>`" + `.

### Rules
- Always include a ` + "`tier:`" + ` tag: ` + "`pinned`" + ` (always loaded), ` + "`reference`" + ` (loaded by default), ` + "`working`" + ` (current task), ` + "`off-context`" + ` (archived)
- Commands in code blocks are ignored (only bare commands in your response text are processed)
- Use ` + "`project:X`" + ` tags for cross-project organization
- Invoke the ` + "`ctx`" + ` skill for full documentation
<!-- ctx:claude-md:end -->
`

// installClaudeMd adds a ctx section to ~/.claude/CLAUDE.md, creating it if needed.
func installClaudeMd(home string) (string, error) {
	claudeDir := filepath.Join(home, ".claude")
	mdPath := filepath.Join(claudeDir, "CLAUDE.md")

	existing, err := os.ReadFile(mdPath)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("failed to read %s: %w", mdPath, err)
	}

	content := string(existing)

	// Check if already present
	if strings.Contains(content, ctxClaudeMdMarker) {
		// Replace existing section
		startIdx := strings.Index(content, ctxClaudeMdMarker)
		endMarker := "<!-- ctx:claude-md:end -->\n"
		endIdx := strings.Index(content, endMarker)
		if endIdx >= 0 {
			endIdx += len(endMarker)
			oldSection := content[startIdx:endIdx]
			if oldSection == ctxClaudeMdSection {
				return fmt.Sprintf("%s (already up to date)", mdPath), nil
			}
			if installDryRun {
				return fmt.Sprintf("%s (would update ctx section)", mdPath), nil
			}
			content = content[:startIdx] + ctxClaudeMdSection + content[endIdx:]
		} else {
			return fmt.Sprintf("%s (already configured)", mdPath), nil
		}
	} else {
		if installDryRun {
			if len(existing) == 0 {
				return fmt.Sprintf("%s (would create with ctx section)", mdPath), nil
			}
			return fmt.Sprintf("%s (would append ctx section)", mdPath), nil
		}
		// Append to existing content
		if len(content) > 0 && !strings.HasSuffix(content, "\n\n") {
			if !strings.HasSuffix(content, "\n") {
				content += "\n"
			}
			content += "\n"
		}
		content += ctxClaudeMdSection
	}

	if installDryRun {
		return fmt.Sprintf("%s (would update)", mdPath), nil
	}

	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create %s: %w", claudeDir, err)
	}

	if err := os.WriteFile(mdPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write %s: %w", mdPath, err)
	}

	if len(existing) == 0 {
		return fmt.Sprintf("%s (created)", mdPath), nil
	}
	return fmt.Sprintf("%s (ctx section added)", mdPath), nil
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

