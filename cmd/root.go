package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/zate/ctx/cmd/hook"
	"github.com/zate/ctx/internal/db"
)

var (
	dbPath string
	format string
)

var rootCmd = &cobra.Command{
	Use:   "ctx",
	Short: "Persistent context management for Claude",
	Long:  "A CLI tool for managing persistent, structured memory across conversations.",
}

func init() {
	home, _ := os.UserHomeDir()
	defaultDB := filepath.Join(home, ".ctx", "store.db")
	if envDB := os.Getenv("CTX_DB"); envDB != "" {
		defaultDB = envDB
	}
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", defaultDB, "Database file path")
	rootCmd.PersistentFlags().StringVar(&format, "format", "text", "Output format: text, json, markdown")
	rootCmd.AddCommand(hook.HookCmd)
}

func Execute() error {
	return rootCmd.Execute()
}

func openDB() (*db.DB, error) {
	d, err := db.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	return d, nil
}

// resolveArg resolves a node ID prefix to a full ID using the database.
func resolveArg(d *db.DB, prefix string) (string, error) {
	resolved, err := d.ResolveID(prefix)
	if err != nil {
		return "", fmt.Errorf("cannot resolve ID %q: %w", prefix, err)
	}
	return resolved, nil
}
