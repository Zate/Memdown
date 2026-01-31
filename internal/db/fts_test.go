package db_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zate/ctx/internal/db"
	"github.com/zate/ctx/testutil"
)

func TestFTSSearch(t *testing.T) {
	d := testutil.SetupTestDB(t)

	d.CreateNode(db.CreateNodeInput{Type: "fact", Content: "The quick brown fox"})
	d.CreateNode(db.CreateNodeInput{Type: "fact", Content: "The lazy dog"})
	d.CreateNode(db.CreateNodeInput{Type: "fact", Content: "Something else entirely"})

	results, err := d.Search("quick")

	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Contains(t, results[0].Content, "quick")
}

func TestFTSSearch_NoResults(t *testing.T) {
	d := testutil.SetupTestDB(t)

	d.CreateNode(db.CreateNodeInput{Type: "fact", Content: "The quick brown fox"})

	results, err := d.Search("elephant")

	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestFTSSearch_UpdatedContent(t *testing.T) {
	d := testutil.SetupTestDB(t)

	node, _ := d.CreateNode(db.CreateNodeInput{Type: "fact", Content: "original content"})

	results1, _ := d.Search("original")
	assert.Len(t, results1, 1)

	d.UpdateNode(node.ID, db.UpdateNodeInput{Content: testutil.Ptr("updated content")})

	results2, _ := d.Search("original")
	assert.Empty(t, results2)

	results3, _ := d.Search("updated")
	assert.Len(t, results3, 1)
}

func TestFTSSearch_DeletedContent(t *testing.T) {
	d := testutil.SetupTestDB(t)

	node, _ := d.CreateNode(db.CreateNodeInput{Type: "fact", Content: "deletable content"})

	results1, _ := d.Search("deletable")
	assert.Len(t, results1, 1)

	d.DeleteNode(node.ID)

	results2, _ := d.Search("deletable")
	assert.Empty(t, results2)
}
