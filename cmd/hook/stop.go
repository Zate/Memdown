package hook

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zate/ctx/internal/db"
	hookpkg "github.com/zate/ctx/internal/hook"
)

var stopResponse string

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Handle Stop hook",
	RunE:  runStop,
}

func init() {
	stopCmd.Flags().StringVar(&stopResponse, "response", "", "Claude's response text (for testing; otherwise reads transcript)")
}

func runStop(cmd *cobra.Command, args []string) error {
	dbPath := cmd.Root().PersistentFlags().Lookup("db").Value.String()

	d, err := db.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ctx: failed to open database: %v\n", err)
		fmt.Println("{}")
		return nil
	}
	defer d.Close()

	var response string

	if stopResponse != "" {
		response = stopResponse
	} else {
		// Read stdin for hook input
		var input map[string]interface{}
		decoder := json.NewDecoder(os.Stdin)
		if err := decoder.Decode(&input); err != nil {
			fmt.Fprintf(os.Stderr, "ctx: failed to read hook input: %v\n", err)
			fmt.Println("{}")
			return nil
		}

		// Try to get transcript path and read last assistant response
		if transcriptPath, ok := input["transcript_path"].(string); ok && transcriptPath != "" {
			resp, err := readLastAssistantResponse(transcriptPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "ctx: failed to read transcript: %v\n", err)
				fmt.Println("{}")
				return nil
			}
			response = resp
		}
	}

	if response == "" {
		fmt.Println("{}")
		return nil
	}

	// Parse ctx commands
	commands := hookpkg.ParseCtxCommands(response)
	if len(commands) == 0 {
		fmt.Println("{}")
		return nil
	}

	// Execute commands and track remember successes
	errs := hookpkg.ExecuteCommandsWithErrors(d, commands)
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "ctx: %v\n", e)
		}
	}

	// Count successful remember commands for session tracking
	rememberCount := 0
	for _, cmd := range commands {
		if cmd.Type == "remember" {
			rememberCount++
		}
	}
	// Subtract failures (conservative: count remember errs)
	rememberErrCount := 0
	for _, e := range errs {
		if strings.HasPrefix(e.Error(), "remember") {
			rememberErrCount++
		}
	}
	successCount := rememberCount - rememberErrCount

	// Update session store count
	if successCount > 0 {
		existing, err := d.GetPending("session_store_count")
		prev := 0
		if err == nil && existing != "" {
			fmt.Sscanf(existing, "%d", &prev)
		}
		d.SetPending("session_store_count", fmt.Sprintf("%d", prev+successCount))
	}

	// Store last_session_stores for next session's awareness
	storeCount, err := d.GetPending("session_store_count")
	if err == nil && storeCount != "" {
		d.SetPending("last_session_stores", storeCount)
	} else {
		d.SetPending("last_session_stores", "0")
	}

	fmt.Println("{}")
	return nil
}

func readLastAssistantResponse(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	// JSONL format - each line is a JSON object
	lines := splitLines(string(data))
	var lastResponse string

	for _, line := range lines {
		if line == "" {
			continue
		}
		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		if entry["type"] == "assistant" {
			if msg, ok := entry["message"].(map[string]interface{}); ok {
				if content, ok := msg["content"].(string); ok {
					lastResponse = content
				}
			}
		}
	}

	return lastResponse, nil
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
