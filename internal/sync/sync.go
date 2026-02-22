package sync

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/zate/ctx/internal/db"
)

// SyncState tracks the sync state for a remote server.
type SyncState struct {
	ServerURL       string `json:"server_url"`
	LastPushVersion int64  `json:"last_push_version"`
	LastPullVersion int64  `json:"last_pull_version"`
	LastPushAt      string `json:"last_push_at,omitempty"`
	LastPullAt      string `json:"last_pull_at,omitempty"`
}

// NodeChange represents a node to be synced.
type NodeChange struct {
	Node    *db.Node `json:"node"`
	Deleted bool     `json:"deleted,omitempty"`
}

// PushRequest is sent to the server during push.
type PushRequest struct {
	DeviceID    string       `json:"device_id"`
	SyncVersion int64        `json:"sync_version"`
	Changes     []NodeChange `json:"changes"`
}

// PushResponse is returned by the server after push.
type PushResponse struct {
	Accepted    int   `json:"accepted"`
	Conflicts   int   `json:"conflicts"`
	SyncVersion int64 `json:"sync_version"`
}

// PullRequest is sent to the server during pull.
type PullRequest struct {
	DeviceID    string `json:"device_id"`
	SyncVersion int64  `json:"since_version"`
}

// PullResponse is returned by the server after pull.
type PullResponse struct {
	Changes     []NodeChange `json:"changes"`
	SyncVersion int64        `json:"sync_version"`
}

// StatusResult shows the sync state comparison.
type StatusResult struct {
	LocalVersion  int64 `json:"local_version"`
	ServerVersion int64 `json:"server_version"`
	LocalOnly     int   `json:"local_only"`
	ServerOnly    int   `json:"server_only"`
	Conflicts     int   `json:"conflicts"`
}

// GetLocalChanges returns nodes modified since the given sync version.
func GetLocalChanges(store db.Store, sinceVersion int64) ([]NodeChange, int64, error) {
	rows, err := store.Query(
		`SELECT id, type, content, summary, token_estimate, superseded_by, created_at, updated_at, metadata, sync_version
		 FROM nodes WHERE sync_version > ? ORDER BY sync_version ASC`,
		sinceVersion,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query local changes: %w", err)
	}
	defer rows.Close()

	var changes []NodeChange
	var maxVersion int64

	for rows.Next() {
		node := &db.Node{}
		var summary, supersededBy sql.NullString
		var createdAt, updatedAt string
		var syncVersion int64

		if err := rows.Scan(&node.ID, &node.Type, &node.Content, &summary, &node.TokenEstimate,
			&supersededBy, &createdAt, &updatedAt, &node.Metadata, &syncVersion); err != nil {
			return nil, 0, fmt.Errorf("failed to scan node: %w", err)
		}

		if summary.Valid {
			s := summary.String
			node.Summary = &s
		}
		if supersededBy.Valid {
			s := supersededBy.String
			node.SupersededBy = &s
		}
		node.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		node.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

		tags, _ := store.GetTags(node.ID)
		node.Tags = tags

		changes = append(changes, NodeChange{Node: node})
		if syncVersion > maxVersion {
			maxVersion = syncVersion
		}
	}

	return changes, maxVersion, nil
}

// ApplyRemoteChanges applies pulled changes to the local store.
// Returns the number of applied changes and any conflicts.
func ApplyRemoteChanges(store db.Store, changes []NodeChange) (applied int, conflicts int, err error) {
	for _, change := range changes {
		if change.Deleted {
			if delErr := store.DeleteNode(change.Node.ID); delErr != nil {
				// Node might not exist locally — that's fine
				continue
			}
			applied++
			continue
		}

		// Check if node exists locally
		existing, getErr := store.GetNode(change.Node.ID)
		if getErr != nil {
			// Node doesn't exist locally — create it
			_, createErr := store.CreateNode(db.CreateNodeInput{
				Type:     change.Node.Type,
				Content:  change.Node.Content,
				Summary:  change.Node.Summary,
				Metadata: change.Node.Metadata,
				Tags:     change.Node.Tags,
			})
			if createErr != nil {
				return applied, conflicts, fmt.Errorf("failed to create node %s: %w", change.Node.ID, createErr)
			}
			applied++
			continue
		}

		// Node exists — check for conflict (different content)
		if existing.UpdatedAt.After(change.Node.UpdatedAt) {
			// Local is newer — conflict (last-write-wins keeps local)
			conflicts++
			continue
		}

		// Remote is newer — update local
		content := change.Node.Content
		nodeType := change.Node.Type
		_, updateErr := store.UpdateNode(change.Node.ID, db.UpdateNodeInput{
			Content: &content,
			Type:    &nodeType,
			Summary: change.Node.Summary,
		})
		if updateErr != nil {
			return applied, conflicts, fmt.Errorf("failed to update node %s: %w", change.Node.ID, updateErr)
		}

		// Sync tags
		existingTags, _ := store.GetTags(change.Node.ID)
		existingTagMap := make(map[string]bool)
		for _, t := range existingTags {
			existingTagMap[t] = true
		}
		for _, t := range change.Node.Tags {
			if !existingTagMap[t] {
				_ = store.AddTag(change.Node.ID, t)
			}
		}
		applied++
	}

	return applied, conflicts, nil
}

// LoadSyncState loads sync state from ~/.ctx/sync_state.json.
func LoadSyncState(serverURL string) (*SyncState, error) {
	path, err := syncStatePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &SyncState{ServerURL: serverURL}, nil
		}
		return nil, err
	}

	var states map[string]*SyncState
	if err := json.Unmarshal(data, &states); err != nil {
		return &SyncState{ServerURL: serverURL}, nil
	}

	if state, ok := states[serverURL]; ok {
		return state, nil
	}
	return &SyncState{ServerURL: serverURL}, nil
}

// SaveSyncState saves sync state to ~/.ctx/sync_state.json.
func SaveSyncState(state *SyncState) error {
	path, err := syncStatePath()
	if err != nil {
		return err
	}

	// Load existing states
	states := make(map[string]*SyncState)
	data, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(data, &states)
	}

	states[state.ServerURL] = state

	newData, err := json.MarshalIndent(states, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, newData, 0600)
}

// NormalizeGitURL normalizes a git remote URL to a canonical form like "github.com/user/repo".
func NormalizeGitURL(raw string) string {
	url := raw
	// Strip protocol prefixes (may need multiple passes for ssh://git@...)
	for _, prefix := range []string{"ssh://", "https://", "http://"} {
		if len(url) > len(prefix) && url[:len(prefix)] == prefix {
			url = url[len(prefix):]
			break
		}
	}
	// Strip git@ prefix (may remain after ssh:// removal)
	if len(url) > 4 && url[:4] == "git@" {
		url = url[4:]
	}
	// Convert git@host:path to host/path
	for i := 0; i < len(url); i++ {
		if url[i] == ':' {
			// Check no slash before the colon (otherwise it's a port)
			hasSlash := false
			for j := 0; j < i; j++ {
				if url[j] == '/' {
					hasSlash = true
					break
				}
			}
			if !hasSlash {
				url = url[:i] + "/" + url[i+1:]
			}
			break
		}
	}
	// Remove trailing .git
	if len(url) > 4 && url[len(url)-4:] == ".git" {
		url = url[:len(url)-4]
	}
	return url
}

func syncStatePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ctx", "sync_state.json"), nil
}
