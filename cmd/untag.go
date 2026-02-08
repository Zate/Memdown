package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var untagCmd = &cobra.Command{
	Use:   "untag <id> <tag>",
	Short: "Remove a tag from a node",
	Args:  cobra.ExactArgs(2),
	RunE:  runUntag,
}

func init() {
	rootCmd.AddCommand(untagCmd)
}

func runUntag(cmd *cobra.Command, args []string) error {
	d, err := openDB()
	if err != nil {
		return err
	}
	defer d.Close()

	id, err := resolveArg(d, args[0])
	if err != nil {
		return err
	}

	if err := d.RemoveTag(id, args[1]); err != nil {
		return err
	}

	fmt.Printf("Untagged: %s from %s\n", args[1], id[:8])
	return nil
}
