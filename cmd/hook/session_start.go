package hook

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/zate/ctx/internal/db"
	"github.com/zate/ctx/internal/view"
)

var sessionStartCmd = &cobra.Command{
	Use:   "session-start",
	Short: "Handle SessionStart hook",
	RunE:  runSessionStart,
}

func runSessionStart(cmd *cobra.Command, args []string) error {
	dbPath := cmd.Root().PersistentFlags().Lookup("db").Value.String()

	d, err := db.Open(dbPath)
	if err != nil {
		// Fail gracefully - return empty output
		fmt.Fprintf(os.Stderr, "ctx: failed to open database: %v\n", err)
		fmt.Println("{}")
		return nil
	}
	defer d.Close()

	// Get default view query
	var queryStr string
	var budget int
	err = d.QueryRow("SELECT query, budget FROM views WHERE name = 'default'").Scan(&queryStr, &budget)
	if err != nil {
		queryStr = "tag:tier:pinned OR tag:tier:reference OR tag:tier:working"
		budget = 50000
	}

	// Check for expand_nodes pending
	expandJSON, err := d.GetPending("expand_nodes")
	var expandIDs []string
	if err == nil && expandJSON != "" {
		json.Unmarshal([]byte(expandJSON), &expandIDs)
		d.DeletePending("expand_nodes")
	}

	result, err := view.Compose(d, view.ComposeOptions{
		Query:  queryStr,
		Budget: budget,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "ctx: failed to compose context: %v\n", err)
		fmt.Println("{}")
		return nil
	}

	// Add expanded nodes if any
	if len(expandIDs) > 0 {
		for _, id := range expandIDs {
			node, err := d.GetNode(id)
			if err != nil {
				continue
			}
			// Check if already included
			found := false
			for _, n := range result.Nodes {
				if n.ID == id {
					found = true
					break
				}
			}
			if !found {
				result.Nodes = append(result.Nodes, node)
				result.TotalTokens += node.TokenEstimate
				result.NodeCount++
			}
		}
	}

	context := view.RenderMarkdown(result)

	output := map[string]interface{}{
		"hookSpecificOutput": map[string]interface{}{
			"hookEventName":     "SessionStart",
			"additionalContext": context,
		},
	}

	data, _ := json.Marshal(output)
	fmt.Println(string(data))
	return nil
}
