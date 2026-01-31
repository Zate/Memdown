package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zate/ctx/internal/db"
)

var addCmd = &cobra.Command{
	Use:   "add [content]",
	Short: "Add a new node",
	RunE:  runAdd,
}

var (
	addType  string
	addTags  []string
	addMeta  []string
	addStdin bool
)

func init() {
	addCmd.Flags().StringVar(&addType, "type", "", "Node type (required)")
	addCmd.MarkFlagRequired("type")
	addCmd.Flags().StringArrayVar(&addTags, "tag", nil, "Tags (repeatable)")
	addCmd.Flags().StringArrayVar(&addMeta, "meta", nil, "Metadata key=value (repeatable)")
	addCmd.Flags().BoolVar(&addStdin, "stdin", false, "Read content from stdin")
	rootCmd.AddCommand(addCmd)
}

func runAdd(cmd *cobra.Command, args []string) error {
	d, err := openDB()
	if err != nil {
		return err
	}
	defer d.Close()

	var content string
	if addStdin {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("failed to read stdin: %w", err)
		}
		content = strings.TrimSpace(string(data))
	} else if len(args) > 0 {
		content = strings.Join(args, " ")
	} else {
		return fmt.Errorf("content is required (provide as argument or use --stdin)")
	}

	metadata := "{}"
	if len(addMeta) > 0 {
		m := make(map[string]string)
		for _, kv := range addMeta {
			parts := strings.SplitN(kv, "=", 2)
			if len(parts) == 2 {
				m[parts[0]] = parts[1]
			}
		}
		data, _ := json.Marshal(m)
		metadata = string(data)
	}

	node, err := d.CreateNode(db.CreateNodeInput{
		Type:     addType,
		Content:  content,
		Metadata: metadata,
		Tags:     addTags,
	})
	if err != nil {
		return err
	}

	switch format {
	case "json":
		data, _ := json.MarshalIndent(node, "", "  ")
		fmt.Println(string(data))
	default:
		fmt.Printf("Added: %s\n", node.ID)
	}

	return nil
}
