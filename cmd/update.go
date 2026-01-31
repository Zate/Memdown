package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zate/ctx/internal/db"
)

var (
	updateContent string
	updateType    string
	updateMeta    string
)

var updateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update a node",
	Args:  cobra.ExactArgs(1),
	RunE:  runUpdate,
}

func init() {
	updateCmd.Flags().StringVar(&updateContent, "content", "", "New content")
	updateCmd.Flags().StringVar(&updateType, "type", "", "New type")
	updateCmd.Flags().StringVar(&updateMeta, "meta", "", "New metadata JSON")
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	d, err := openDB()
	if err != nil {
		return err
	}
	defer d.Close()

	input := db.UpdateNodeInput{}
	if updateContent != "" {
		input.Content = &updateContent
	}
	if updateType != "" {
		input.Type = &updateType
	}
	if updateMeta != "" {
		input.Metadata = &updateMeta
	}

	node, err := d.UpdateNode(args[0], input)
	if err != nil {
		return err
	}

	switch format {
	case "json":
		data, _ := json.MarshalIndent(node, "", "  ")
		fmt.Println(string(data))
	default:
		fmt.Printf("Updated: %s\n", node.ID)
	}

	return nil
}
