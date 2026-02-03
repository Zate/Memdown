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
