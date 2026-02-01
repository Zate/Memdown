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
	Nodes             []*db.Node
	TotalTokens       int
	NodeCount         int
	RenderedAt        time.Time
	LastSessionStores int  // -1 means unknown/not set
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
		RenderedAt:        time.Now().UTC(),
		LastSessionStores: -1,
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

	header := fmt.Sprintf("<!-- ctx: %d nodes, %d tokens, rendered at %s",
		result.NodeCount, result.TotalTokens, result.RenderedAt.Format(time.RFC3339))
	if result.LastSessionStores > 0 {
		header += fmt.Sprintf(" | last session: %d nodes stored", result.LastSessionStores)
	} else if result.LastSessionStores == 0 {
		header += " | last session: no new knowledge stored"
	}
	header += " -->\n\n"
	b.WriteString(header)

	// Always include usage primer with WHEN to store (not just HOW)
	b.WriteString("You have persistent memory via `ctx`. Commands are processed after you respond.\n\n")
	b.WriteString("**Store knowledge when:**\n")
	b.WriteString("- You make or learn a **decision** → `<ctx:remember type=\"decision\" tags=\"tier:reference\">...</ctx:remember>`\n")
	b.WriteString("- You discover a **preference** or convention → `type=\"fact\"`\n")
	b.WriteString("- You see a recurring **pattern** → `type=\"pattern\"`\n")
	b.WriteString("- Debugging reveals a **root cause** → `type=\"observation\"`\n")
	b.WriteString("- An idea worth revisiting → `type=\"hypothesis\"`\n")
	b.WriteString("- A question can't be answered now → `type=\"open-question\"`\n\n")
	b.WriteString("**Other commands:** `<ctx:recall query=\"...\"/>`, `<ctx:status/>`, `<ctx:task name=\"X\" action=\"start|end\"/>`\n")
	b.WriteString("Always include a `tier:` tag. Invoke the `ctx` skill for full reference.\n\n")

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
