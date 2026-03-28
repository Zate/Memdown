package view_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zate/ctx/internal/db"
	"github.com/zate/ctx/internal/view"
	"github.com/zate/ctx/testutil"
)

func createNode(t *testing.T, d db.Store, nodeType, content string, tags []string) *db.Node {
	t.Helper()
	node, err := d.CreateNode(db.CreateNodeInput{
		Type:    nodeType,
		Content: content,
		Tags:    tags,
	})
	require.NoError(t, err)
	return node
}

func TestCompose_ProjectFiltering_ExcludesOtherProjects(t *testing.T) {
	d := testutil.SetupTestDB(t)

	createNode(t, d, "fact", "ctx-specific fact", []string{"tier:reference", "project:ctx"})
	createNode(t, d, "fact", "memdown-specific fact", []string{"tier:reference", "project:memdown"})
	createNode(t, d, "fact", "global fact no project tag", []string{"tier:reference"})

	result, err := view.Compose(d, view.ComposeOptions{
		Query:   "tag:tier:reference",
		Budget:  50000,
		Project: "memdown",
	})
	require.NoError(t, err)

	// Should include memdown + global, exclude ctx
	assert.Equal(t, 2, result.NodeCount)
	contents := nodeContents(result.Nodes)
	assert.Contains(t, contents, "memdown-specific fact")
	assert.Contains(t, contents, "global fact no project tag")
	assert.NotContains(t, contents, "ctx-specific fact")
}

func TestCompose_ProjectFiltering_IncludesGlobalTag(t *testing.T) {
	d := testutil.SetupTestDB(t)

	createNode(t, d, "fact", "explicitly global", []string{"tier:reference", "project:global"})
	createNode(t, d, "fact", "other project", []string{"tier:reference", "project:other"})

	result, err := view.Compose(d, view.ComposeOptions{
		Query:   "tag:tier:reference",
		Budget:  50000,
		Project: "myproject",
	})
	require.NoError(t, err)

	assert.Equal(t, 1, result.NodeCount)
	assert.Equal(t, "explicitly global", result.Nodes[0].Content)
}

func TestCompose_ProjectFiltering_EmptyProjectLoadsAll(t *testing.T) {
	d := testutil.SetupTestDB(t)

	createNode(t, d, "fact", "project-a fact", []string{"tier:reference", "project:a"})
	createNode(t, d, "fact", "project-b fact", []string{"tier:reference", "project:b"})
	createNode(t, d, "fact", "untagged fact", []string{"tier:reference"})
	createNode(t, d, "fact", "global fact", []string{"tier:reference", "project:global"})

	result, err := view.Compose(d, view.ComposeOptions{
		Query:  "tag:tier:reference",
		Budget: 50000,
		// Project is empty string - no project filtering, include everything
	})
	require.NoError(t, err)

	// Empty project = no filtering, all nodes included
	assert.Equal(t, 4, result.NodeCount)
	contents := nodeContents(result.Nodes)
	assert.Contains(t, contents, "untagged fact")
	assert.Contains(t, contents, "global fact")
	assert.Contains(t, contents, "project-a fact")
	assert.Contains(t, contents, "project-b fact")
}

func TestCompose_ProjectFiltering_CaseInsensitive(t *testing.T) {
	d := testutil.SetupTestDB(t)

	createNode(t, d, "fact", "mixed case", []string{"tier:reference", "project:MyProject"})

	result, err := view.Compose(d, view.ComposeOptions{
		Query:   "tag:tier:reference",
		Budget:  50000,
		Project: "myproject",
	})
	require.NoError(t, err)

	assert.Equal(t, 1, result.NodeCount)
}

func TestCompose_ProjectFiltering_MultipleProjectTags(t *testing.T) {
	d := testutil.SetupTestDB(t)

	// Node tagged with multiple projects
	createNode(t, d, "fact", "shared between a and b", []string{"tier:reference", "project:a", "project:b"})
	createNode(t, d, "fact", "only project c", []string{"tier:reference", "project:c"})

	result, err := view.Compose(d, view.ComposeOptions{
		Query:   "tag:tier:reference",
		Budget:  50000,
		Project: "a",
	})
	require.NoError(t, err)

	assert.Equal(t, 1, result.NodeCount)
	assert.Equal(t, "shared between a and b", result.Nodes[0].Content)
}

func nodeContents(nodes []*db.Node) []string {
	var contents []string
	for _, n := range nodes {
		contents = append(contents, n.Content)
	}
	return contents
}

func TestCompose_DefaultQuery_ExcludesReference(t *testing.T) {
	d := testutil.SetupTestDB(t)

	createNode(t, d, "fact", "pinned fact", []string{"tier:pinned"})
	createNode(t, d, "decision", "reference decision", []string{"tier:reference"})
	createNode(t, d, "observation", "working observation", []string{"tier:working"})

	// Use the new default query (pinned OR working, no reference)
	result, err := view.Compose(d, view.ComposeOptions{
		Query:  "tag:tier:pinned OR tag:tier:working",
		Budget: 50000,
	})
	require.NoError(t, err)

	assert.Equal(t, 2, result.NodeCount)
	contents := nodeContents(result.Nodes)
	assert.Contains(t, contents, "pinned fact")
	assert.Contains(t, contents, "working observation")
	assert.NotContains(t, contents, "reference decision")
}

func TestCompose_ReferenceStats_CountsByType(t *testing.T) {
	d := testutil.SetupTestDB(t)

	createNode(t, d, "decision", "ref decision 1", []string{"tier:reference"})
	createNode(t, d, "decision", "ref decision 2", []string{"tier:reference"})
	createNode(t, d, "fact", "ref fact 1", []string{"tier:reference"})
	createNode(t, d, "pattern", "ref pattern 1", []string{"tier:reference"})
	createNode(t, d, "fact", "pinned fact", []string{"tier:pinned"})

	result, err := view.Compose(d, view.ComposeOptions{
		Query:                 "tag:tier:pinned OR tag:tier:working",
		Budget:                50000,
		IncludeReferenceStats: true,
	})
	require.NoError(t, err)

	// Only pinned loaded
	assert.Equal(t, 1, result.NodeCount)
	assert.Equal(t, "pinned fact", result.Nodes[0].Content)

	// Reference stats counted
	assert.Equal(t, 4, result.ReferenceCount)
	assert.Equal(t, 2, result.ReferenceByType["decision"])
	assert.Equal(t, 1, result.ReferenceByType["fact"])
	assert.Equal(t, 1, result.ReferenceByType["pattern"])
}

func TestCompose_ReferenceStats_RespectsProjectScope(t *testing.T) {
	d := testutil.SetupTestDB(t)

	createNode(t, d, "decision", "project-a ref", []string{"tier:reference", "project:alpha"})
	createNode(t, d, "decision", "project-b ref", []string{"tier:reference", "project:beta"})
	createNode(t, d, "fact", "global ref", []string{"tier:reference", "project:global"})
	createNode(t, d, "fact", "unscoped ref", []string{"tier:reference"})

	result, err := view.Compose(d, view.ComposeOptions{
		Query:                 "tag:tier:pinned OR tag:tier:working",
		Budget:                50000,
		Project:               "alpha",
		IncludeReferenceStats: true,
	})
	require.NoError(t, err)

	// Should count alpha + global + unscoped, exclude beta
	assert.Equal(t, 3, result.ReferenceCount)
	assert.Equal(t, 1, result.ReferenceByType["decision"])
	assert.Equal(t, 2, result.ReferenceByType["fact"])
}

func TestRenderMarkdown_ShowsReferenceAvailability(t *testing.T) {
	result := &view.ComposeResult{
		NodeCount:       1,
		TotalTokens:     100,
		ReferenceCount:  5,
		ReferenceByType: map[string]int{"decision": 3, "fact": 2},
	}

	output := view.RenderMarkdown(result)
	assert.Contains(t, output, "**Reference available:** 5 nodes not auto-loaded")
	assert.Contains(t, output, "3 decisions")
	assert.Contains(t, output, "2 facts")
}

func TestRenderMarkdown_HidesReferenceWhenZero(t *testing.T) {
	result := &view.ComposeResult{
		NodeCount:      1,
		TotalTokens:    100,
		ReferenceCount: 0,
	}

	output := view.RenderMarkdown(result)
	assert.NotContains(t, output, "Reference available")
}

// BUG-1: compose with no --project should not filter out project-scoped nodes
func TestCompose_NoProjectFlag_IncludesAllProjects(t *testing.T) {
	d := testutil.SetupTestDB(t)

	createNode(t, d, "fact", "book fact", []string{"tier:pinned", "project:Book"})
	createNode(t, d, "decision", "glint decision", []string{"tier:pinned", "project:glint"})
	createNode(t, d, "fact", "global fact", []string{"tier:pinned"})

	result, err := view.Compose(d, view.ComposeOptions{
		Query:  "tag:tier:pinned",
		Budget: 50000,
		// No Project set — should include ALL nodes
	})
	require.NoError(t, err)

	assert.Equal(t, 3, result.NodeCount, "all nodes should be included when no project filter")
	contents := nodeContents(result.Nodes)
	assert.Contains(t, contents, "book fact")
	assert.Contains(t, contents, "glint decision")
	assert.Contains(t, contents, "global fact")
}

// BUG-2: compose --ids should bypass project and agent filtering
func TestCompose_ExplicitIDs_BypassFiltering(t *testing.T) {
	d := testutil.SetupTestDB(t)

	// Create a node with agent and project tags
	node := createNode(t, d, "decision", "nyx-specific decision", []string{
		"tier:pinned", "project:Book", "agent:nyx",
	})

	// Compose by ID without --agent or --project — should still find it
	result, err := view.Compose(d, view.ComposeOptions{
		IDs:    []string{node.ID[:8]}, // short prefix
		Budget: 50000,
		// No Agent, no Project — should NOT filter explicit IDs
	})
	require.NoError(t, err)

	assert.Equal(t, 1, result.NodeCount, "explicit IDs should bypass agent/project filtering")
	assert.Equal(t, "nyx-specific decision", result.Nodes[0].Content)
}

func TestCompose_ExplicitIDs_WithDifferentAgent(t *testing.T) {
	d := testutil.SetupTestDB(t)

	node := createNode(t, d, "fact", "agent-scoped fact", []string{
		"tier:pinned", "agent:nyx",
	})

	// Request by ID with a DIFFERENT agent — should still return it
	result, err := view.Compose(d, view.ComposeOptions{
		IDs:    []string{node.ID},
		Budget: 50000,
		Agent:  "other-agent",
	})
	require.NoError(t, err)

	assert.Equal(t, 1, result.NodeCount, "explicit IDs should bypass agent filtering even with different agent")
}

func TestCompose_ExplicitIDs_WithDifferentProject(t *testing.T) {
	d := testutil.SetupTestDB(t)

	node := createNode(t, d, "fact", "project-scoped fact", []string{
		"tier:pinned", "project:alpha",
	})

	// Request by ID with a DIFFERENT project — should still return it
	result, err := view.Compose(d, view.ComposeOptions{
		IDs:     []string{node.ID},
		Budget:  50000,
		Project: "beta",
	})
	require.NoError(t, err)

	assert.Equal(t, 1, result.NodeCount, "explicit IDs should bypass project filtering")
}
