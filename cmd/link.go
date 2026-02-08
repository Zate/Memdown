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

	fromID, err := resolveArg(d, args[0])
	if err != nil {
		return err
	}
	toID, err := resolveArg(d, args[1])
	if err != nil {
		return err
	}

	edge, err := d.CreateEdge(fromID, toID, linkType)
	if err != nil {
		return err
	}

	switch format {
	case "json":
		data, _ := json.MarshalIndent(edge, "", "  ")
		fmt.Println(string(data))
	default:
		fmt.Printf("Linked: %s → %s (%s)\n", fromID[:8], toID[:8], linkType)
	}

	return nil
}
