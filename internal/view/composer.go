package view

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/zate/ctx/internal/db"
	"github.com/zate/ctx/internal/query"
)

type ComposeOptions struct {
	Query  string
	Budget int
}

type ComposeResult struct {
	Nodes      []*db.Node
	TotalTokens int
	NodeCount   int
	RenderedAt  time.Time
}

func Compose(d *db.DB, opts ComposeOptions) (*ComposeResult, error) {
	var nodes []*db.Node
	var err error

	if opts.Query != "" {
		nodes, err = query.ExecuteQuery(d, opts.Query, false)
	} else {
		nodes, err = d.ListNodes(db.ListOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	// Sort by priority: pinned > reference > working > other > recency
	sort.SliceStable(nodes, func(i, j int) bool {
		pi := tierPriority(nodes[i].Tags)
		pj := tierPriority(nodes[j].Tags)
		if pi != pj {
			return pi < pj
		}
		return nodes[i].CreatedAt.After(nodes[j].CreatedAt)
	})

	// Apply budget
	result := &ComposeResult{
		RenderedAt: time.Now().UTC(),
	}

	if opts.Budget <= 0 {
		return result, nil
	}

	for _, n := range nodes {
		if result.TotalTokens+n.TokenEstimate > opts.Budget {
			continue
		}
		result.Nodes = append(result.Nodes, n)
		result.TotalTokens += n.TokenEstimate
		result.NodeCount++
	}

	return result, nil
}

func tierPriority(tags []string) int {
	for _, t := range tags {
		switch t {
		case "tier:pinned":
			return 0
		case "tier:reference":
			return 1
		case "tier:working":
			return 2
		}
	}
	return 3
}

func RenderMarkdown(result *ComposeResult) string {
	var b strings.Builder

	fmt.Fprintf(&b, "<!-- ctx: %d nodes, %d tokens, rendered at %s -->\n\n",
		result.NodeCount, result.TotalTokens, result.RenderedAt.Format(time.RFC3339))

	// Always include a compact usage primer so the agent knows how to use ctx
	b.WriteString("You have persistent memory via `ctx`. Use these XML commands in your responses (they are processed automatically after you respond):\n")
	b.WriteString("- `<ctx:remember type=\"fact|decision|pattern|observation\" tags=\"tier:reference,project:X\">content</ctx:remember>` — store knowledge\n")
	b.WriteString("- `<ctx:recall query=\"type:decision AND tag:project:X\"/>` — retrieve knowledge (results appear on next prompt)\n")
	b.WriteString("- `<ctx:status/>` — check memory status\n")
	b.WriteString("- `<ctx:task name=\"X\" action=\"start|end\"/>` — mark task boundaries\n")
	b.WriteString("Always include a `tier:` tag (pinned, reference, working, off-context). Invoke the `ctx` skill for full reference.\n\n")

	// Group by tier then type
	groups := map[string][]*db.Node{
		"pinned":    {},
		"reference": {},
		"working":   {},
		"other":     {},
	}

	for _, n := range result.Nodes {
		tier := "other"
		for _, t := range n.Tags {
			switch t {
			case "tier:pinned":
				tier = "pinned"
			case "tier:reference":
				tier = "reference"
			case "tier:working":
				tier = "working"
			}
		}
		groups[tier] = append(groups[tier], n)
	}

	renderGroup := func(title string, nodes []*db.Node) {
		if len(nodes) == 0 {
			return
		}
		fmt.Fprintf(&b, "## %s\n\n", title)

		// Sub-group by type
		byType := map[string][]*db.Node{}
		typeOrder := []string{}
		for _, n := range nodes {
			if _, exists := byType[n.Type]; !exists {
				typeOrder = append(typeOrder, n.Type)
			}
			byType[n.Type] = append(byType[n.Type], n)
		}

		for _, t := range typeOrder {
			if len(typeOrder) > 1 {
				fmt.Fprintf(&b, "### %s\n\n", titleCase(t))
			}
			for _, n := range byType[t] {
				shortID := n.ID[:8]
				content := n.Content
				if len(content) > 200 {
					content = content[:200] + "..."
				}
				fmt.Fprintf(&b, "- [%s:%s] %s\n", n.Type, shortID, content)
				if len(n.Tags) > 0 {
					fmt.Fprintf(&b, "  - Tags: %s\n", strings.Join(n.Tags, ", "))
				}
			}
			b.WriteString("\n")
		}
	}

	renderGroup("Pinned", groups["pinned"])
	renderGroup("Reference", groups["reference"])
	renderGroup("Working Context", groups["working"])
	renderGroup("Other", groups["other"])

	b.WriteString("<!-- ctx:end -->\n")
	return b.String()
}

func titleCase(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func RenderText(result *ComposeResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Context: %d nodes, %d tokens\n\n", result.NodeCount, result.TotalTokens)
	for _, n := range result.Nodes {
		preview := n.Content
		if len(preview) > 80 {
			preview = preview[:80] + "..."
		}
		tags := ""
		if len(n.Tags) > 0 {
			tags = " [" + strings.Join(n.Tags, ", ") + "]"
		}
		fmt.Fprintf(&b, "[%s] %s: %s%s\n", n.ID[:8], n.Type, preview, tags)
	}
	return b.String()
}
