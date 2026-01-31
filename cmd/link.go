package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var linkType string

var linkCmd = &cobra.Command{
	Use:   "link <from-id> <to-id>",
	Short: "Link two nodes",
	Args:  cobra.ExactArgs(2),
	RunE:  runLink,
}

func init() {
	linkCmd.Flags().StringVar(&linkType, "type", "RELATES_TO", "Edge type")
	rootCmd.AddCommand(linkCmd)
}

func runLink(cmd *cobra.Command, args []string) error {
	d, err := openDB()
	if err != nil {
		return err
	}
	defer d.Close()

	edge, err := d.CreateEdge(args[0], args[1], linkType)
	if err != nil {
		return err
	}

	switch format {
	case "json":
		data, _ := json.MarshalIndent(edge, "", "  ")
		fmt.Println(string(data))
	default:
		fmt.Printf("Linked: %s → %s (%s)\n", args[0][:8], args[1][:8], linkType)
	}

	return nil
}
