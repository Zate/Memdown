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

	if err := d.RemoveTag(args[0], args[1]); err != nil {
		return err
	}

	fmt.Printf("Untagged: %s from %s\n", args[1], args[0][:8])
	return nil
}
