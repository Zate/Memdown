package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/zate/ctx/internal/view"
)

var (
	composeQuery  string
	composeBudget int
)

var composeCmd = &cobra.Command{
	Use:   "compose",
	Short: "Compose context from query",
	RunE:  runCompose,
}

func init() {
	defaultBudget := 50000
	if envBudget := os.Getenv("CTX_DEFAULT_BUDGET"); envBudget != "" {
		if n, err := strconv.Atoi(envBudget); err == nil {
			defaultBudget = n
		}
	}
	composeCmd.Flags().StringVar(&composeQuery, "query", "", "Query expression")
	composeCmd.Flags().IntVar(&composeBudget, "budget", defaultBudget, "Token budget")
	rootCmd.AddCommand(composeCmd)
}

func runCompose(cmd *cobra.Command, args []string) error {
	d, err := openDB()
	if err != nil {
		return err
	}
	defer d.Close()

	result, err := view.Compose(d, view.ComposeOptions{
		Query:  composeQuery,
		Budget: composeBudget,
	})
	if err != nil {
		return err
	}

	switch format {
	case "json":
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
	case "markdown":
		fmt.Print(view.RenderMarkdown(result))
	default:
		fmt.Print(view.RenderText(result))
	}

	return nil
}
