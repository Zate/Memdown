package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a node",
	Args:  cobra.ExactArgs(1),
	RunE:  runDelete,
}

func init() {
	rootCmd.AddCommand(deleteCmd)
}

func runDelete(cmd *cobra.Command, args []string) error {
	d, err := openDB()
	if err != nil {
		return err
	}
	defer d.Close()

	id, err := resolveArg(d, args[0])
	if err != nil {
		return err
	}

	if err := d.DeleteNode(id); err != nil {
		return err
	}

	fmt.Printf("Deleted: %s\n", id)
	return nil
}
