package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var showWithEdges bool

var showCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show a node",
	Args:  cobra.ExactArgs(1),
	RunE:  runShow,
}

func init() {
	showCmd.Flags().BoolVar(&showWithEdges, "with-edges", false, "Include edges")
	rootCmd.AddCommand(showCmd)
}

func runShow(cmd *cobra.Command, args []string) error {
	d, err := openDB()
	if err != nil {
		return err
	}
	defer d.Close()

	node, err := d.GetNode(args[0])
	if err != nil {
		return err
	}

	switch format {
	case "json":
		out := map[string]interface{}{
			"id":             node.ID,
			"type":           node.Type,
			"content":        node.Content,
			"token_estimate": node.TokenEstimate,
			"created_at":     node.CreatedAt,
			"updated_at":     node.UpdatedAt,
			"metadata":       node.Metadata,
			"tags":           node.Tags,
		}
		if node.Summary != nil {
			out["summary"] = *node.Summary
		}
		if node.SupersededBy != nil {
			out["superseded_by"] = *node.SupersededBy
		}
		if showWithEdges {
			edges, _ := d.GetEdges(node.ID, "both")
			out["edges"] = edges
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
	default:
		fmt.Printf("ID:      %s\n", node.ID)
		fmt.Printf("Type:    %s\n", node.Type)
		fmt.Printf("Content: %s\n", node.Content)
		fmt.Printf("Tokens:  %d\n", node.TokenEstimate)
		fmt.Printf("Created: %s\n", node.CreatedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("Updated: %s\n", node.UpdatedAt.Format("2006-01-02 15:04:05"))
		if len(node.Tags) > 0 {
			fmt.Printf("Tags:    %s\n", joinStrings(node.Tags, ", "))
		}
		if node.Summary != nil {
			fmt.Printf("Summary: %s\n", *node.Summary)
		}
		if node.SupersededBy != nil {
			fmt.Printf("Superseded by: %s\n", *node.SupersededBy)
		}
		if showWithEdges {
			edges, _ := d.GetEdges(node.ID, "both")
			if len(edges) > 0 {
				fmt.Println("Edges:")
				for _, e := range edges {
					if e.FromID == node.ID {
						fmt.Printf("  → %s (%s) %s\n", e.ToID, e.Type, e.ID)
					} else {
						fmt.Printf("  ← %s (%s) %s\n", e.FromID, e.Type, e.ID)
					}
				}
			}
		}
	}

	return nil
}

func joinStrings(ss []string, sep string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}
