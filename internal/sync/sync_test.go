package sync

import (
	"testing"
	"time"

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

func TestGetLocalChanges_WithModifiedNodes(t *testing.T) {
	store := testutil.SetupTestDB(t)

	// Create a node and bump its sync_version
	node, err := store.CreateNode(db.CreateNodeInput{
		Type:    "fact",
		Content: "Synced fact",
		Tags:    []string{"tier:pinned"},
	})
	require.NoError(t, err)

	_, err = store.Exec("UPDATE nodes SET sync_version = 1 WHERE id = ?", node.ID)
	require.NoError(t, err)

	changes, maxV, err := GetLocalChanges(store, 0)
	require.NoError(t, err)
	assert.Len(t, changes, 1)
	assert.Equal(t, int64(1), maxV)
	assert.Equal(t, "Synced fact", changes[0].Node.Content)
	assert.Contains(t, changes[0].Node.Tags, "tier:pinned")

	// Query with since_version=1 should return nothing
	changes2, _, err := GetLocalChanges(store, 1)
	require.NoError(t, err)
	assert.Empty(t, changes2)
}

func TestApplyRemoteChanges_Conflict_LocalNewer(t *testing.T) {
	store := testutil.SetupTestDB(t)

	// Create a local node
	node, err := store.CreateNode(db.CreateNodeInput{
		Type:    "fact",
		Content: "Local version",
	})
	require.NoError(t, err)

	// Apply a remote change where the remote node is older than local
	remoteNode := &db.Node{
		ID:        node.ID,
		Type:      "fact",
		Content:   "Remote version (older)",
		UpdatedAt: node.UpdatedAt.Add(-1 * time.Hour), // older than local
	}

	applied, conflicts, err := ApplyRemoteChanges(store, []NodeChange{{Node: remoteNode}})
	require.NoError(t, err)
	assert.Equal(t, 0, applied)
	assert.Equal(t, 1, conflicts)

	// Local version should be preserved
	got, err := store.GetNode(node.ID)
	require.NoError(t, err)
	assert.Equal(t, "Local version", got.Content)
}

func TestApplyRemoteChanges_Update_RemoteNewer(t *testing.T) {
	store := testutil.SetupTestDB(t)

	// Create a local node
	node, err := store.CreateNode(db.CreateNodeInput{
		Type:    "fact",
		Content: "Local version",
		Tags:    []string{"tier:pinned"},
	})
	require.NoError(t, err)

	// Apply a remote change where the remote node is newer
	remoteNode := &db.Node{
		ID:        node.ID,
		Type:      "decision",
		Content:   "Remote version (newer)",
		UpdatedAt: node.UpdatedAt.Add(1 * time.Hour),
		Tags:      []string{"tier:pinned", "project:new-tag"},
	}

	applied, conflicts, err := ApplyRemoteChanges(store, []NodeChange{{Node: remoteNode}})
	require.NoError(t, err)
	assert.Equal(t, 1, applied)
	assert.Equal(t, 0, conflicts)

	// Verify update
	got, err := store.GetNode(node.ID)
	require.NoError(t, err)
	assert.Equal(t, "Remote version (newer)", got.Content)
	assert.Equal(t, "decision", got.Type)

	// Verify tags were synced (new tag added)
	tags, err := store.GetTags(node.ID)
	require.NoError(t, err)
	assert.Contains(t, tags, "project:new-tag")
	assert.Contains(t, tags, "tier:pinned")
}

func TestApplyRemoteChanges_DeleteNonexistent(t *testing.T) {
	store := testutil.SetupTestDB(t)

	// Deleting a node that doesn't exist locally should not error
	changes := []NodeChange{
		{Node: &db.Node{ID: "nonexistent-id"}, Deleted: true},
	}

	applied, conflicts, err := ApplyRemoteChanges(store, changes)
	require.NoError(t, err)
	assert.Equal(t, 0, applied)
	assert.Equal(t, 0, conflicts)
}

func TestSaveSyncState(t *testing.T) {
	// Use a temp home directory to avoid polluting real config
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	state := &SyncState{
		ServerURL:       "http://test:8377",
		LastPushVersion: 5,
		LastPullVersion: 3,
		LastPushAt:      "2025-01-01T00:00:00Z",
		LastPullAt:      "2025-01-01T00:00:00Z",
	}

	err := SaveSyncState(state)
	require.NoError(t, err)

	// Load it back
	loaded, err := LoadSyncState("http://test:8377")
	require.NoError(t, err)
	assert.Equal(t, int64(5), loaded.LastPushVersion)
	assert.Equal(t, int64(3), loaded.LastPullVersion)

	// Loading a different server should return empty state
	other, err := LoadSyncState("http://other:8377")
	require.NoError(t, err)
	assert.Equal(t, int64(0), other.LastPushVersion)
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
