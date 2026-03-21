// Package agent provides agent-scoped memory partitioning utilities.
//
// Nodes tagged with "agent:<name>" are scoped to that agent. Nodes without
// any agent tag are global and visible to all agents. When an agent is
// specified, only that agent's nodes and global nodes are returned. When
// no agent is specified, agent-scoped nodes are hidden.
package agent

import (
	"fmt"
	"strings"

	"github.com/zate/ctx/internal/db"
)

// ShouldInclude returns true if a node should be visible given the current agent.
// A node is agent-scoped if it has any tag matching "agent:*".
// If agent-scoped, it only shows if one of its agent tags matches the current agent.
// Nodes with no agent tags are global and visible to everyone.
func ShouldInclude(node *db.Node, currentAgent string) bool {
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

// FilterNodes filters a slice of nodes to only include those visible to the given agent.
func FilterNodes(nodes []*db.Node, currentAgent string) []*db.Node {
	var filtered []*db.Node
	for _, n := range nodes {
		if ShouldInclude(n, currentAgent) {
			filtered = append(filtered, n)
		}
	}
	return filtered
}

// Tag returns the agent tag string for the given agent name, or empty if no agent.
func Tag(agentName string) string {
	if agentName == "" {
		return ""
	}
	return "agent:" + agentName
}

// FilterSQL returns a SQL fragment that filters nodes by agent scope.
// The nodes table must be aliased as "n".
// If currentAgent is set: include nodes tagged with that agent + nodes with no agent:* tag.
// If currentAgent is empty: include only nodes with no agent:* tag.
func FilterSQL(currentAgent string) string {
	if currentAgent != "" {
		return fmt.Sprintf(
			` AND (n.id IN (SELECT node_id FROM tags WHERE tag = 'agent:%s') OR n.id NOT IN (SELECT node_id FROM tags WHERE tag LIKE 'agent:%%'))`,
			currentAgent,
		)
	}
	return ` AND n.id NOT IN (SELECT node_id FROM tags WHERE tag LIKE 'agent:%')`
}
