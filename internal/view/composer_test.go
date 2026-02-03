package view_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zate/ctx/internal/db"
	"github.com/zate/ctx/internal/view"
	"github.com/zate/ctx/testutil"
)

func createNode(t *testing.T, d *db.DB, nodeType, content string, tags []string) *db.Node {
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

func TestCompose_ProjectFiltering_NoProjectLoadsAll(t *testing.T) {
	d := testutil.SetupTestDB(t)

	createNode(t, d, "fact", "project-a fact", []string{"tier:reference", "project:a"})
	createNode(t, d, "fact", "project-b fact", []string{"tier:reference", "project:b"})
	createNode(t, d, "fact", "untagged fact", []string{"tier:reference"})

	result, err := view.Compose(d, view.ComposeOptions{
		Query:  "tag:tier:reference",
		Budget: 50000,
	})
	require.NoError(t, err)

	// No project filter — load everything
	assert.Equal(t, 3, result.NodeCount)
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
