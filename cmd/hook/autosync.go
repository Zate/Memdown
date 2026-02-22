package hook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/zate/ctx/internal/db"
	ctxsync "github.com/zate/ctx/internal/sync"
)

// autoSyncConfig checks if auto_sync is enabled and credentials exist.
type autoSyncConfig struct {
	ServerURL string
	Token     string
	DeviceID  string
}

// loadAutoSyncConfig loads remote + auth config for auto-sync.
// Returns nil if auto-sync is not possible (missing config, not authenticated).
func loadAutoSyncConfig() *autoSyncConfig {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	// Check for auto_sync setting in server.yaml or env
	if os.Getenv("CTX_AUTO_SYNC") == "" {
		// Check server.yaml for auto_sync: true
		data, err := os.ReadFile(filepath.Join(home, ".ctx", "server.yaml"))
		if err != nil || !bytes.Contains(data, []byte("auto_sync: true")) {
			return nil
		}
	} else if os.Getenv("CTX_AUTO_SYNC") != "true" && os.Getenv("CTX_AUTO_SYNC") != "1" {
		return nil
	}

	// Load remote config
	remoteData, err := os.ReadFile(filepath.Join(home, ".ctx", "remote.json"))
	if err != nil {
		return nil
	}
	var remote struct {
		URL string `json:"url"`
	}
	if json.Unmarshal(remoteData, &remote) != nil || remote.URL == "" {
		return nil
	}

	// Load auth config
	authData, err := os.ReadFile(filepath.Join(home, ".ctx", "auth.json"))
	if err != nil {
		return nil
	}
	var auth struct {
		Token    string `json:"token"`
		DeviceID string `json:"device_id"`
	}
	if json.Unmarshal(authData, &auth) != nil || auth.Token == "" {
		return nil
	}

	return &autoSyncConfig{
		ServerURL: remote.URL,
		Token:     auth.Token,
		DeviceID:  auth.DeviceID,
	}
}

// autoSyncPull pulls remote changes on session start.
// Fails gracefully — errors are logged to stderr, never block the session.
func autoSyncPull(store db.Store) {
	cfg := loadAutoSyncConfig()
	if cfg == nil {
		return
	}

	state, err := ctxsync.LoadSyncState(cfg.ServerURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ctx: auto-sync pull: failed to load state: %v\n", err)
		return
	}

	pullReq := ctxsync.PullRequest{
		DeviceID:    cfg.DeviceID,
		SyncVersion: state.LastPullVersion,
	}

	body, _ := json.Marshal(pullReq)
	resp, err := authedPost(cfg.ServerURL+"/api/sync/pull", body, cfg.Token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ctx: auto-sync pull: server unreachable (offline mode)\n")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "ctx: auto-sync pull: server error %d\n", resp.StatusCode)
		return
	}

	respBody, _ := io.ReadAll(resp.Body)
	var pullResp ctxsync.PullResponse
	if json.Unmarshal(respBody, &pullResp) != nil {
		return
	}

	if len(pullResp.Changes) == 0 {
		return
	}

	applied, conflicts, err := ctxsync.ApplyRemoteChanges(store, pullResp.Changes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ctx: auto-sync pull: failed to apply: %v\n", err)
		return
	}

	state.LastPullVersion = pullResp.SyncVersion
	state.LastPullAt = time.Now().UTC().Format(time.RFC3339)
	_ = ctxsync.SaveSyncState(state)

	fmt.Fprintf(os.Stderr, "ctx: auto-sync pulled %d change(s), %d conflict(s)\n", applied, conflicts)
}

// autoSyncPush pushes local changes on session end.
// Fails gracefully — errors are logged to stderr, never block the session.
func autoSyncPush(store db.Store) {
	cfg := loadAutoSyncConfig()
	if cfg == nil {
		return
	}

	state, err := ctxsync.LoadSyncState(cfg.ServerURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ctx: auto-sync push: failed to load state: %v\n", err)
		return
	}

	changes, maxVersion, err := ctxsync.GetLocalChanges(store, state.LastPushVersion)
	if err != nil || len(changes) == 0 {
		return
	}

	pushReq := ctxsync.PushRequest{
		DeviceID:    cfg.DeviceID,
		SyncVersion: state.LastPushVersion,
		Changes:     changes,
	}

	body, _ := json.Marshal(pushReq)
	resp, err := authedPost(cfg.ServerURL+"/api/sync/push", body, cfg.Token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ctx: auto-sync push: server unreachable (offline mode)\n")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "ctx: auto-sync push: server error %d\n", resp.StatusCode)
		return
	}

	state.LastPushVersion = maxVersion
	state.LastPushAt = time.Now().UTC().Format(time.RFC3339)
	_ = ctxsync.SaveSyncState(state)

	fmt.Fprintf(os.Stderr, "ctx: auto-sync pushed %d change(s)\n", len(changes))
}

func authedPost(url string, body []byte, token string) (*http.Response, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	return client.Do(req)
}
