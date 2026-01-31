package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var tagsPrefix string

var tagsCmd = &cobra.Command{
	Use:   "tags",
	Short: "List all tags",
	RunE:  runTags,
}

func init() {
	tagsCmd.Flags().StringVar(&tagsPrefix, "prefix", "", "Filter by prefix")
	rootCmd.AddCommand(tagsCmd)
}

func runTags(cmd *cobra.Command, args []string) error {
	d, err := openDB()
	if err != nil {
		return err
	}
	defer d.Close()

	var tags []string
	if tagsPrefix != "" {
		tags, err = d.ListTagsByPrefix(tagsPrefix)
	} else {
		tags, err = d.ListAllTags()
	}
	if err != nil {
		return err
	}

	switch format {
	case "json":
		data, _ := json.MarshalIndent(tags, "", "  ")
		fmt.Println(string(data))
	default:
		if len(tags) == 0 {
			fmt.Println("No tags found.")
			return nil
		}
		for _, t := range tags {
			fmt.Println(t)
		}
	}

	return nil
}
