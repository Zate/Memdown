package db_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zate/ctx/internal/db"
	"github.com/zate/ctx/testutil"
)

func TestTagAdd(t *testing.T) {
	d := testutil.SetupTestDB(t)

	node, _ := d.CreateNode(db.CreateNodeInput{Type: "fact", Content: "a"})

	err := d.AddTag(node.ID, "project:test")

	assert.NoError(t, err)

	tags, _ := d.GetTags(node.ID)
	assert.Contains(t, tags, "project:test")
}

func TestTagAdd_Idempotent(t *testing.T) {
	d := testutil.SetupTestDB(t)

	node, _ := d.CreateNode(db.CreateNodeInput{Type: "fact", Content: "a"})

	err1 := d.AddTag(node.ID, "project:test")
	err2 := d.AddTag(node.ID, "project:test")

	assert.NoError(t, err1)
	assert.NoError(t, err2)

	tags, _ := d.GetTags(node.ID)
	count := 0
	for _, tag := range tags {
		if tag == "project:test" {
			count++
		}
	}
	assert.Equal(t, 1, count)
}

func TestTagRemove(t *testing.T) {
	d := testutil.SetupTestDB(t)

	node, _ := d.CreateNode(db.CreateNodeInput{Type: "fact", Content: "a"})
	_ = d.AddTag(node.ID, "project:test")

	err := d.RemoveTag(node.ID, "project:test")

	assert.NoError(t, err)

	tags, _ := d.GetTags(node.ID)
	assert.NotContains(t, tags, "project:test")
}

func TestTagList_AllTags(t *testing.T) {
	d := testutil.SetupTestDB(t)

	n1, _ := d.CreateNode(db.CreateNodeInput{Type: "fact", Content: "a"})
	n2, _ := d.CreateNode(db.CreateNodeInput{Type: "fact", Content: "b"})

	_ = d.AddTag(n1.ID, "project:a")
	_ = d.AddTag(n1.ID, "tier:reference")
	_ = d.AddTag(n2.ID, "project:b")

	tags, err := d.ListAllTags()

	require.NoError(t, err)
	assert.Len(t, tags, 3)
}

func TestTagList_ByPrefix(t *testing.T) {
	d := testutil.SetupTestDB(t)

	node, _ := d.CreateNode(db.CreateNodeInput{Type: "fact", Content: "a"})
	_ = d.AddTag(node.ID, "project:a")
	_ = d.AddTag(node.ID, "project:b")
	_ = d.AddTag(node.ID, "tier:reference")

	tags, err := d.ListTagsByPrefix("project:")

	require.NoError(t, err)
	assert.Len(t, tags, 2)
}

func TestTagCascadeDelete(t *testing.T) {
	d := testutil.SetupTestDB(t)

	node, _ := d.CreateNode(db.CreateNodeInput{Type: "fact", Content: "a"})
	_ = d.AddTag(node.ID, "project:test")

	_ = d.DeleteNode(node.ID)

	tags, _ := d.ListAllTags()
	assert.Empty(t, tags)
}
