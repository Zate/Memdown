package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show database status",
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	d, err := openDB()
	if err != nil {
		return err
	}
	defer d.Close()

	// Get file size
	info, _ := os.Stat(dbPath)
	var fileSize int64
	if info != nil {
		fileSize = info.Size()
	}

	// Count nodes by type
	type typeCount struct {
		Type  string `json:"type"`
		Count int    `json:"count"`
	}
	rows, err := d.Query("SELECT type, COUNT(*) FROM nodes WHERE superseded_by IS NULL GROUP BY type ORDER BY type")
	if err != nil {
		return err
	}
	defer rows.Close()

	var typeCounts []typeCount
	totalNodes := 0
	for rows.Next() {
		var tc typeCount
		rows.Scan(&tc.Type, &tc.Count)
		typeCounts = append(typeCounts, tc)
		totalNodes += tc.Count
	}

	// Total tokens
	var totalTokens int
	d.QueryRow("SELECT COALESCE(SUM(token_estimate), 0) FROM nodes WHERE superseded_by IS NULL").Scan(&totalTokens)

	// Edge count
	var edgeCount int
	d.QueryRow("SELECT COUNT(*) FROM edges").Scan(&edgeCount)

	// Unique tags
	var tagCount int
	d.QueryRow("SELECT COUNT(DISTINCT tag) FROM tags").Scan(&tagCount)

	// Tier breakdown
	type tierInfo struct {
		Tier   string `json:"tier"`
		Nodes  int    `json:"nodes"`
		Tokens int    `json:"tokens"`
	}
	tierRows, err := d.Query(`SELECT t.tag, COUNT(DISTINCT t.node_id), COALESCE(SUM(n.token_estimate), 0)
		FROM tags t JOIN nodes n ON t.node_id = n.id
		WHERE t.tag LIKE 'tier:%' AND n.superseded_by IS NULL
		GROUP BY t.tag ORDER BY t.tag`)
	if err != nil {
		return err
	}
	defer tierRows.Close()

	var tiers []tierInfo
	for tierRows.Next() {
		var ti tierInfo
		tierRows.Scan(&ti.Tier, &ti.Nodes, &ti.Tokens)
		tiers = append(tiers, ti)
	}

	switch format {
	case "json":
		out := map[string]interface{}{
			"database":     dbPath,
			"file_size":    fileSize,
			"total_nodes":  totalNodes,
			"total_tokens": totalTokens,
			"total_edges":  edgeCount,
			"unique_tags":  tagCount,
			"types":        typeCounts,
			"tiers":        tiers,
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
	default:
		fmt.Printf("Database: %s", dbPath)
		if fileSize > 0 {
			fmt.Printf(" (%.1f KB)", float64(fileSize)/1024)
		}
		fmt.Println()
		fmt.Printf("Nodes: %d (estimated %d tokens)\n", totalNodes, totalTokens)
		for _, tc := range typeCounts {
			fmt.Printf("  %s: %d\n", tc.Type, tc.Count)
		}
		fmt.Printf("Edges: %d\n", edgeCount)
		fmt.Printf("Tags: %d unique\n", tagCount)
		if len(tiers) > 0 {
			fmt.Println("\nTier breakdown:")
			for _, ti := range tiers {
				fmt.Printf("  %s: %d nodes (%d tokens)\n", ti.Tier, ti.Nodes, ti.Tokens)
			}
		}
	}

	return nil
}
