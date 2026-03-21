package hook

import (
	agentpkg "github.com/zate/ctx/internal/agent"
	"github.com/zate/ctx/internal/db"
)

// filterNodesByAgent filters nodes by agent scope using the shared agent package.
func filterNodesByAgent(nodes []*db.Node, currentAgent string) []*db.Node {
	return agentpkg.FilterNodes(nodes, currentAgent)
}
