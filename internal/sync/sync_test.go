package sync

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zate/ctx/internal/db"
	"github.com/zate/ctx/testutil"
)

func TestGetLocalChanges_Empty(t *testing.T) {
	store := testutil.SetupTestDB(t)
	changes, maxV, err := GetLocalChanges(store, 0)
	require.NoError(t, err)
	assert.Empty(t, changes)
	assert.Equal(t, int64(0), maxV)
}

func TestApplyRemoteChanges_CreateNew(t *testing.T) {
	store := testutil.SetupTestDB(t)

	// Create a change to apply
	changes := []NodeChange{
		{
			Node: &db.Node{
				ID:      "test-node-1",
				Type:    "fact",
				Content: "Remote fact",
				Tags:    []string{"tier:pinned"},
			},
		},
	}

	applied, conflicts, err := ApplyRemoteChanges(store, changes)
	require.NoError(t, err)
	assert.Equal(t, 1, applied)
	assert.Equal(t, 0, conflicts)
}

func TestApplyRemoteChanges_Delete(t *testing.T) {
	store := testutil.SetupTestDB(t)

	// Create a node first
	node, err := store.CreateNode(db.CreateNodeInput{
		Type:    "fact",
		Content: "To be deleted",
	})
	require.NoError(t, err)

	changes := []NodeChange{
		{Node: &db.Node{ID: node.ID}, Deleted: true},
	}

	applied, _, err := ApplyRemoteChanges(store, changes)
	require.NoError(t, err)
	assert.Equal(t, 1, applied)

	// Verify deleted
	_, err = store.GetNode(node.ID)
	assert.Error(t, err)
}

func TestSyncState(t *testing.T) {
	state, err := LoadSyncState("http://test-server:8377")
	require.NoError(t, err)
	assert.Equal(t, "http://test-server:8377", state.ServerURL)
	assert.Equal(t, int64(0), state.LastPushVersion)
}

func TestNormalizeGitURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://github.com/user/repo.git", "github.com/user/repo"},
		{"git@github.com:user/repo.git", "github.com/user/repo"},
		{"https://github.com/user/repo", "github.com/user/repo"},
		{"ssh://git@github.com/user/repo.git", "github.com/user/repo"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := NormalizeGitURL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
