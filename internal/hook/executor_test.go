package hook_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zate/ctx/internal/db"
	"github.com/zate/ctx/internal/hook"
	"github.com/zate/ctx/testutil"
)

func TestExecuteRemember_Dedup(t *testing.T) {
	d := testutil.SetupTestDB(t)

	cmds := []hook.CtxCommand{
		{
			Type:    "remember",
			Attrs:   map[string]string{"type": "fact", "tags": "tier:pinned"},
			Content: "Always run tests before committing.",
		},
	}

	// First execution should create the node
	errs := hook.ExecuteCommandsWithErrors(d, cmds)
	assert.Empty(t, errs)

	nodes, err := d.ListNodes(db.ListOptions{Type: "fact"})
	require.NoError(t, err)
	assert.Len(t, nodes, 1)
	originalID := nodes[0].ID

	// Second execution with identical content should not create a duplicate
	errs = hook.ExecuteCommandsWithErrors(d, cmds)
	assert.Empty(t, errs)

	nodes, err = d.ListNodes(db.ListOptions{Type: "fact"})
	require.NoError(t, err)
	assert.Len(t, nodes, 1)
	assert.Equal(t, originalID, nodes[0].ID)
}

func TestExecuteRemember_DedupMergesTags(t *testing.T) {
	d := testutil.SetupTestDB(t)

	// Create with one tag
	cmds1 := []hook.CtxCommand{
		{
			Type:    "remember",
			Attrs:   map[string]string{"type": "fact", "tags": "tier:reference"},
			Content: "SQLite uses WAL mode.",
		},
	}
	errs := hook.ExecuteCommandsWithErrors(d, cmds1)
	assert.Empty(t, errs)

	nodes, err := d.ListNodes(db.ListOptions{Type: "fact"})
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	originalID := nodes[0].ID

	// Re-remember with additional tag — should merge, not duplicate
	cmds2 := []hook.CtxCommand{
		{
			Type:    "remember",
			Attrs:   map[string]string{"type": "fact", "tags": "tier:reference,project:ctx"},
			Content: "SQLite uses WAL mode.",
		},
	}
	errs = hook.ExecuteCommandsWithErrors(d, cmds2)
	assert.Empty(t, errs)

	nodes, err = d.ListNodes(db.ListOptions{Type: "fact"})
	require.NoError(t, err)
	assert.Len(t, nodes, 1)
	assert.Equal(t, originalID, nodes[0].ID)

	// Verify both tags exist
	tags, err := d.GetTags(originalID)
	require.NoError(t, err)
	assert.Contains(t, tags, "tier:reference")
	assert.Contains(t, tags, "project:ctx")
}

func TestExecuteRemember_DifferentContentNotDeduped(t *testing.T) {
	d := testutil.SetupTestDB(t)

	cmds := []hook.CtxCommand{
		{
			Type:    "remember",
			Attrs:   map[string]string{"type": "fact"},
			Content: "First fact.",
		},
		{
			Type:    "remember",
			Attrs:   map[string]string{"type": "fact"},
			Content: "Second fact.",
		},
	}

	errs := hook.ExecuteCommandsWithErrors(d, cmds)
	assert.Empty(t, errs)

	nodes, err := d.ListNodes(db.ListOptions{Type: "fact"})
	require.NoError(t, err)
	assert.Len(t, nodes, 2)
}

func TestExecuteRemember_AutoProjectTag(t *testing.T) {
	d := testutil.SetupTestDB(t)

	// Set current project in pending
	require.NoError(t, d.SetPending("current_project", "memdown"))

	cmds := []hook.CtxCommand{
		{
			Type:    "remember",
			Attrs:   map[string]string{"type": "fact", "tags": "tier:reference"},
			Content: "This should get auto-tagged with project:memdown.",
		},
	}
	errs := hook.ExecuteCommandsWithErrors(d, cmds)
	assert.Empty(t, errs)

	nodes, err := d.ListNodes(db.ListOptions{Type: "fact"})
	require.NoError(t, err)
	require.Len(t, nodes, 1)

	tags, err := d.GetTags(nodes[0].ID)
	require.NoError(t, err)
	assert.Contains(t, tags, "tier:reference")
	assert.Contains(t, tags, "project:memdown")
}

func TestExecuteRemember_NoAutoProjectTagWhenExplicit(t *testing.T) {
	d := testutil.SetupTestDB(t)

	// Set current project in pending
	require.NoError(t, d.SetPending("current_project", "memdown"))

	cmds := []hook.CtxCommand{
		{
			Type:    "remember",
			Attrs:   map[string]string{"type": "fact", "tags": "tier:reference,project:other"},
			Content: "Already has a project tag.",
		},
	}
	errs := hook.ExecuteCommandsWithErrors(d, cmds)
	assert.Empty(t, errs)

	nodes, err := d.ListNodes(db.ListOptions{Type: "fact"})
	require.NoError(t, err)
	require.Len(t, nodes, 1)

	tags, err := d.GetTags(nodes[0].ID)
	require.NoError(t, err)
	assert.Contains(t, tags, "project:other")
	assert.NotContains(t, tags, "project:memdown")
}

func TestExecuteRemember_NoAutoProjectTagWhenNoPending(t *testing.T) {
	d := testutil.SetupTestDB(t)

	// No current_project in pending

	cmds := []hook.CtxCommand{
		{
			Type:    "remember",
			Attrs:   map[string]string{"type": "fact", "tags": "tier:reference"},
			Content: "No project tag should be added.",
		},
	}
	errs := hook.ExecuteCommandsWithErrors(d, cmds)
	assert.Empty(t, errs)

	nodes, err := d.ListNodes(db.ListOptions{Type: "fact"})
	require.NoError(t, err)
	require.Len(t, nodes, 1)

	tags, err := d.GetTags(nodes[0].ID)
	require.NoError(t, err)
	for _, tag := range tags {
		assert.False(t, len(tag) > 8 && tag[:8] == "project:", "unexpected project tag: %s", tag)
	}
}

func TestExecuteRemember_DifferentTypeNotDeduped(t *testing.T) {
	d := testutil.SetupTestDB(t)

	cmds := []hook.CtxCommand{
		{
			Type:    "remember",
			Attrs:   map[string]string{"type": "fact"},
			Content: "Same content different type.",
		},
		{
			Type:    "remember",
			Attrs:   map[string]string{"type": "decision"},
			Content: "Same content different type.",
		},
	}

	errs := hook.ExecuteCommandsWithErrors(d, cmds)
	assert.Empty(t, errs)

	allFacts, _ := d.ListNodes(db.ListOptions{Type: "fact"})
	allDecisions, _ := d.ListNodes(db.ListOptions{Type: "decision"})
	assert.Len(t, allFacts, 1)
	assert.Len(t, allDecisions, 1)
}

// uniquePrefix returns the shortest prefix of id that doesn't match any other ID's prefix.
// For test use: finds first char position where ids diverge, returns prefix up to that point + 1.
func uniquePrefix(id string, otherIDs ...string) string {
	for i := 1; i <= len(id); i++ {
		prefix := id[:i]
		unique := true
		for _, other := range otherIDs {
			if len(other) >= i && other[:i] == prefix {
				unique = false
				break
			}
		}
		if unique {
			return prefix
		}
	}
	return id
}

func TestExecuteSupersede_ShortID(t *testing.T) {
	d := testutil.SetupTestDB(t)

	n1, err := d.CreateNode(db.CreateNodeInput{Type: "fact", Content: "old fact"})
	require.NoError(t, err)
	n2, err := d.CreateNode(db.CreateNodeInput{Type: "fact", Content: "new fact"})
	require.NoError(t, err)

	// Use unique prefixes (may be longer than 8 chars if created in same ms)
	p1 := uniquePrefix(n1.ID, n2.ID)
	p2 := uniquePrefix(n2.ID, n1.ID)
	cmds := []hook.CtxCommand{
		{
			Type:  "supersede",
			Attrs: map[string]string{"old": p1, "new": p2},
		},
	}
	errs := hook.ExecuteCommandsWithErrors(d, cmds)
	assert.Empty(t, errs)

	// Verify the supersede actually took effect
	node, err := d.GetNode(n1.ID)
	require.NoError(t, err)
	require.NotNil(t, node.SupersededBy)
	assert.Equal(t, n2.ID, *node.SupersededBy)
}

func TestExecuteLink_ShortID(t *testing.T) {
	d := testutil.SetupTestDB(t)

	n1, err := d.CreateNode(db.CreateNodeInput{Type: "fact", Content: "node A"})
	require.NoError(t, err)
	n2, err := d.CreateNode(db.CreateNodeInput{Type: "fact", Content: "node B"})
	require.NoError(t, err)

	p1 := uniquePrefix(n1.ID, n2.ID)
	p2 := uniquePrefix(n2.ID, n1.ID)
	cmds := []hook.CtxCommand{
		{
			Type:  "link",
			Attrs: map[string]string{"from": p1, "to": p2, "type": "RELATES_TO"},
		},
	}
	errs := hook.ExecuteCommandsWithErrors(d, cmds)
	assert.Empty(t, errs)

	// Verify edge was created
	edges, err := d.GetEdgesFrom(n1.ID)
	require.NoError(t, err)
	require.Len(t, edges, 1)
	assert.Equal(t, n2.ID, edges[0].ToID)
}

func TestExecuteExpand_ShortID(t *testing.T) {
	d := testutil.SetupTestDB(t)

	// Create a summary node and a source node, linked by DERIVED_FROM
	source, err := d.CreateNode(db.CreateNodeInput{Type: "fact", Content: "source fact"})
	require.NoError(t, err)
	summary, err := d.CreateNode(db.CreateNodeInput{Type: "summary", Content: "summary of facts"})
	require.NoError(t, err)
	_, err = d.CreateEdge(summary.ID, source.ID, "DERIVED_FROM")
	require.NoError(t, err)

	// Expand using unique prefix
	prefix := uniquePrefix(summary.ID, source.ID)
	cmds := []hook.CtxCommand{
		{
			Type:  "expand",
			Attrs: map[string]string{"node": prefix},
		},
	}
	errs := hook.ExecuteCommandsWithErrors(d, cmds)
	assert.Empty(t, errs)

	// Verify the expand_nodes pending was set
	pending, err := d.GetPending("expand_nodes")
	require.NoError(t, err)
	assert.Contains(t, pending, source.ID)
}

func TestExecuteSummarize_ShortID(t *testing.T) {
	d := testutil.SetupTestDB(t)

	n1, err := d.CreateNode(db.CreateNodeInput{Type: "fact", Content: "fact one"})
	require.NoError(t, err)
	n2, err := d.CreateNode(db.CreateNodeInput{Type: "fact", Content: "fact two"})
	require.NoError(t, err)

	p1 := uniquePrefix(n1.ID, n2.ID)
	p2 := uniquePrefix(n2.ID, n1.ID)
	// Summarize using short IDs
	cmds := []hook.CtxCommand{
		{
			Type:    "summarize",
			Attrs:   map[string]string{"nodes": p1 + "," + p2},
			Content: "Summary of two facts.",
		},
	}
	errs := hook.ExecuteCommandsWithErrors(d, cmds)
	assert.Empty(t, errs)

	// Verify summary node was created and linked
	summaries, err := d.ListNodes(db.ListOptions{Type: "summary"})
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, "Summary of two facts.", summaries[0].Content)

	edges, err := d.GetEdgesFrom(summaries[0].ID)
	require.NoError(t, err)
	assert.Len(t, edges, 2)
}
