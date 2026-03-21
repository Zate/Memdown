package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zate/ctx/cmd/hook"
	"github.com/zate/ctx/internal/db"
)

var (
	dbPath  string
	format  string
	backend string
	agent   string
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
	defaultAgent := os.Getenv("CTX_AGENT")
	rootCmd.PersistentFlags().StringVar(&agent, "agent", defaultAgent, "Agent identity for memory partitioning (filters to agent-scoped + global nodes)")
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

// agentTag returns the agent tag string for the current agent, or empty if no agent is set.
func agentTag() string {
	if agent == "" {
		return ""
	}
	return "agent:" + agent
}

// filterNodesByAgent filters a slice of nodes to only include those visible to the current agent.
// If no agent is set, only global/untagged nodes are returned (agent-scoped nodes are hidden).
// If an agent is set, only that agent's nodes + global/untagged nodes are returned.
func filterNodesByAgent(nodes []*db.Node) []*db.Node {
	var filtered []*db.Node
	for _, n := range nodes {
		if shouldIncludeForAgent(n, agent) {
			filtered = append(filtered, n)
		}
	}
	return filtered
}

// shouldIncludeForAgent returns true if a node should be visible given the current agent.
// A node is agent-scoped if it has any tag matching "agent:*".
// If agent-scoped, it only shows if one of its agent tags matches the current agent.
// Nodes with no agent tags are global and visible to everyone.
func shouldIncludeForAgent(node *db.Node, currentAgent string) bool {
	hasAgentTag := false
	matchesCurrent := false
	for _, tag := range node.Tags {
		if strings.HasPrefix(tag, "agent:") {
			hasAgentTag = true
			a := strings.TrimPrefix(tag, "agent:")
			if strings.EqualFold(a, currentAgent) {
				matchesCurrent = true
			}
		}
	}
	if !hasAgentTag {
		return true // global node, visible to all
	}
	if currentAgent == "" {
		return false // agent-scoped node, no agent specified, hide it
	}
	return matchesCurrent
}
