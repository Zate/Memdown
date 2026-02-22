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
	dbPath  string
	format  string
	backend string
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
	defaultBackend := "sqlite"
	if envBackend := os.Getenv("CTX_BACKEND"); envBackend != "" {
		defaultBackend = envBackend
	}
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", defaultDB, "Database path (file path for sqlite, connection string for postgres)")
	rootCmd.PersistentFlags().StringVar(&format, "format", "text", "Output format: text, json, markdown")
	rootCmd.PersistentFlags().StringVar(&backend, "backend", defaultBackend, "Database backend: sqlite, postgres")
	rootCmd.AddCommand(hook.HookCmd)
}

func Execute() error {
	return rootCmd.Execute()
}

func openDB() (db.Store, error) {
	switch backend {
	case "postgres", "postgresql":
		d, err := db.OpenPostgres(dbPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open postgres database: %w", err)
		}
		return d, nil
	case "sqlite", "":
		d, err := db.Open(dbPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open database: %w", err)
		}
		return d, nil
	default:
		return nil, fmt.Errorf("unknown backend %q: use 'sqlite' or 'postgres'", backend)
	}
}

// resolveArg resolves a node ID prefix to a full ID using the database.
func resolveArg(d db.Store, prefix string) (string, error) {
	resolved, err := d.ResolveID(prefix)
	if err != nil {
		return "", fmt.Errorf("cannot resolve ID %q: %w", prefix, err)
	}
	return resolved, nil
}
