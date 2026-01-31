package hook

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zate/ctx/internal/db"
	"github.com/zate/ctx/internal/query"
)

var promptSubmitCmd = &cobra.Command{
	Use:   "prompt-submit",
	Short: "Handle UserPromptSubmit hook",
	RunE:  runPromptSubmit,
}

func runPromptSubmit(cmd *cobra.Command, args []string) error {
	dbPath := cmd.Root().PersistentFlags().Lookup("db").Value.String()

	d, err := db.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ctx: failed to open database: %v\n", err)
		fmt.Println("{}")
		return nil
	}
	defer d.Close()

	var contextParts []string

	// Check for recall query
	recallQuery, err := d.GetPending("recall_query")
	if err == nil && recallQuery != "" {
		nodes, err := query.ExecuteQuery(d, recallQuery, false)
		if err == nil {
			var b strings.Builder
			fmt.Fprintf(&b, "## Recall Results\n\nQuery: `%s`\n\n", recallQuery)
			if len(nodes) == 0 {
				b.WriteString("No matching nodes found.\n")
			} else {
				fmt.Fprintf(&b, "Found %d nodes:\n\n", len(nodes))
				for _, n := range nodes {
					shortID := n.ID[:8]
					fmt.Fprintf(&b, "- [%s:%s] %s\n", n.Type, shortID, n.Content)
					if len(n.Tags) > 0 {
						fmt.Fprintf(&b, "  - Tags: %s\n", strings.Join(n.Tags, ", "))
					}
				}
			}
			b.WriteString("\n---\n")
			contextParts = append(contextParts, b.String())
		}
		d.DeletePending("recall_query")
	}

	// Check for recall_results (pre-computed)
	recallResults, err := d.GetPending("recall_results")
	if err == nil && recallResults != "" {
		contextParts = append(contextParts, recallResults)
		d.DeletePending("recall_results")
	}

	// Check for status output
	statusOutput, err := d.GetPending("status_output")
	if err == nil && statusOutput != "" {
		contextParts = append(contextParts, "## Memory Status\n\n"+statusOutput+"\n\n---\n")
		d.DeletePending("status_output")
	}

	// Check for expand nodes
	expandJSON, err := d.GetPending("expand_nodes")
	if err == nil && expandJSON != "" {
		var expandIDs []string
		json.Unmarshal([]byte(expandJSON), &expandIDs)

		if len(expandIDs) > 0 {
			var b strings.Builder
			b.WriteString("## Expanded Nodes\n\n")
			for _, id := range expandIDs {
				node, err := d.GetNode(id)
				if err != nil {
					continue
				}
				shortID := node.ID[:8]
				fmt.Fprintf(&b, "- [%s:%s] %s\n", node.Type, shortID, node.Content)
				if len(node.Tags) > 0 {
					fmt.Fprintf(&b, "  - Tags: %s\n", strings.Join(node.Tags, ", "))
				}
			}
			b.WriteString("\n---\n")
			contextParts = append(contextParts, b.String())
		}
		d.DeletePending("expand_nodes")
	}

	if len(contextParts) == 0 {
		fmt.Println("{}")
		return nil
	}

	additionalContext := strings.Join(contextParts, "\n")

	output := map[string]interface{}{
		"hookSpecificOutput": map[string]interface{}{
			"hookEventName":     "UserPromptSubmit",
			"additionalContext": additionalContext,
		},
	}

	data, _ := json.Marshal(output)
	fmt.Println(string(data))
	return nil
}
