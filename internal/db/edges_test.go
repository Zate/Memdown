package db_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zate/ctx/internal/db"
	"github.com/zate/ctx/testutil"
)

func TestEdgeCreate(t *testing.T) {
	d := testutil.SetupTestDB(t)

	n1, _ := d.CreateNode(db.CreateNodeInput{Type: "fact", Content: "a"})
	n2, _ := d.CreateNode(db.CreateNodeInput{Type: "fact", Content: "b"})

	edge, err := d.CreateEdge(n1.ID, n2.ID, "DEPENDS_ON")

	require.NoError(t, err)
	assert.Equal(t, n1.ID, edge.FromID)
	assert.Equal(t, n2.ID, edge.ToID)
	assert.Equal(t, "DEPENDS_ON", edge.Type)
}

func TestEdgeCreate_InvalidFromNode(t *testing.T) {
	d := testutil.SetupTestDB(t)

	n1, _ := d.CreateNode(db.CreateNodeInput{Type: "fact", Content: "a"})

	_, err := d.CreateEdge("nonexistent", n1.ID, "DEPENDS_ON")

	assert.Error(t, err)
}

func TestEdgeCreate_InvalidType(t *testing.T) {
	d := testutil.SetupTestDB(t)

	n1, _ := d.CreateNode(db.CreateNodeInput{Type: "fact", Content: "a"})
	n2, _ := d.CreateNode(db.CreateNodeInput{Type: "fact", Content: "b"})

	_, err := d.CreateEdge(n1.ID, n2.ID, "INVALID_TYPE")

	assert.Error(t, err)
}

func TestEdgeCreate_Duplicate(t *testing.T) {
	d := testutil.SetupTestDB(t)

	n1, _ := d.CreateNode(db.CreateNodeInput{Type: "fact", Content: "a"})
	n2, _ := d.CreateNode(db.CreateNodeInput{Type: "fact", Content: "b"})

	_, err1 := d.CreateEdge(n1.ID, n2.ID, "DEPENDS_ON")
	_, err2 := d.CreateEdge(n1.ID, n2.ID, "DEPENDS_ON")

	assert.NoError(t, err1)
	assert.NoError(t, err2) // Idempotent
}

func TestEdgeCascadeDelete(t *testing.T) {
	d := testutil.SetupTestDB(t)

	n1, _ := d.CreateNode(db.CreateNodeInput{Type: "fact", Content: "a"})
	n2, _ := d.CreateNode(db.CreateNodeInput{Type: "fact", Content: "b"})
	_, _ = d.CreateEdge(n1.ID, n2.ID, "DEPENDS_ON")

	err := d.DeleteNode(n1.ID)
	assert.NoError(t, err)

	edges, _ := d.GetEdgesFrom(n1.ID)
	assert.Empty(t, edges)
}

func TestEdgeTypes(t *testing.T) {
	validTypes := []string{"DERIVED_FROM", "DEPENDS_ON", "SUPERSEDES", "RELATES_TO", "CHILD_OF"}

	for _, edgeType := range validTypes {
		t.Run(edgeType, func(t *testing.T) {
			d := testutil.SetupTestDB(t)

			n1, _ := d.CreateNode(db.CreateNodeInput{Type: "fact", Content: "a"})
			n2, _ := d.CreateNode(db.CreateNodeInput{Type: "fact", Content: "b"})

			edge, err := d.CreateEdge(n1.ID, n2.ID, edgeType)

			require.NoError(t, err)
			assert.Equal(t, edgeType, edge.Type)
		})
	}
}

func TestEdgeGetDirections(t *testing.T) {
	d := testutil.SetupTestDB(t)

	n1, _ := d.CreateNode(db.CreateNodeInput{Type: "fact", Content: "a"})
	n2, _ := d.CreateNode(db.CreateNodeInput{Type: "fact", Content: "b"})
	n3, _ := d.CreateNode(db.CreateNodeInput{Type: "fact", Content: "c"})

	_, _ = d.CreateEdge(n1.ID, n2.ID, "DEPENDS_ON")
	_, _ = d.CreateEdge(n3.ID, n1.ID, "RELATES_TO")

	outEdges, _ := d.GetEdges(n1.ID, "out")
	assert.Len(t, outEdges, 1)

	inEdges, _ := d.GetEdges(n1.ID, "in")
	assert.Len(t, inEdges, 1)

	allEdges, _ := d.GetEdges(n1.ID, "both")
	assert.Len(t, allEdges, 2)
}
