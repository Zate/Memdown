package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var unlinkType string

var unlinkCmd = &cobra.Command{
	Use:   "unlink <from-id> <to-id>",
	Short: "Unlink two nodes",
	Args:  cobra.ExactArgs(2),
	RunE:  runUnlink,
}

func init() {
	unlinkCmd.Flags().StringVar(&unlinkType, "type", "", "Edge type (optional, removes all if not specified)")
	rootCmd.AddCommand(unlinkCmd)
}

func runUnlink(cmd *cobra.Command, args []string) error {
	d, err := openDB()
	if err != nil {
		return err
	}
	defer d.Close()

	if err := d.DeleteEdge(args[0], args[1], unlinkType); err != nil {
		return err
	}

	fmt.Printf("Unlinked: %s → %s\n", args[0][:8], args[1][:8])
	return nil
}
