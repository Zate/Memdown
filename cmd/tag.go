package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var tagCmd = &cobra.Command{
	Use:   "tag <id> <tag>...",
	Short: "Add tags to a node",
	Args:  cobra.MinimumNArgs(2),
	RunE:  runTag,
}

func init() {
	rootCmd.AddCommand(tagCmd)
}

func runTag(cmd *cobra.Command, args []string) error {
	d, err := openDB()
	if err != nil {
		return err
	}
	defer d.Close()

	nodeID, err := resolveArg(d, args[0])
	if err != nil {
		return err
	}
	for _, tag := range args[1:] {
		if err := d.AddTag(nodeID, tag); err != nil {
			return fmt.Errorf("failed to add tag %s: %w", tag, err)
		}
	}

	fmt.Printf("Tagged: %s with %s\n", nodeID[:8], joinStrings(args[1:], ", "))
	return nil
}
