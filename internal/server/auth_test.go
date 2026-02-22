package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zate/ctx/internal/auth"
	"github.com/zate/ctx/internal/db"
	"github.com/zate/ctx/testutil"
)

// createAuthTables creates the users, devices, repo_mappings tables in SQLite
// so that auth-related server endpoints can be tested without Postgres.
func createAuthTables(t *testing.T, store db.Store) {
	t.Helper()
	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			username TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS devices (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id),
			name TEXT NOT NULL,
			token_hash TEXT NOT NULL,
			refresh_token_hash TEXT,
			last_seen TEXT,
			last_ip TEXT,
			revoked BOOLEAN NOT NULL DEFAULT FALSE,
			created_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS repo_mappings (
			id TEXT PRIMARY KEY,
			normalized_url TEXT UNIQUE NOT NULL,
			project_tag TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT ''
		)`,
	} {
		_, err := store.Exec(ddl)
		require.NoError(t, err)
	}
}

func setupAuthTestServer(t *testing.T, adminPassword string) (*Server, db.Store) {
	t.Helper()
	store := testutil.SetupTestDB(t)
	createAuthTables(t, store)
	cfg := DefaultConfig()
	cfg.AdminPassword = adminPassword
	srv := New(store, cfg)
	return srv, store
}

// insertTestDevice inserts a device with the given token directly into the DB.
// Returns the device ID.
func insertTestDevice(t *testing.T, store db.Store, name, token, refreshToken string, revoked bool) string {
	t.Helper()
	// Ensure admin user exists
	var userID string
	err := store.QueryRow("SELECT id FROM users WHERE username = 'admin'").Scan(&userID)
	if err != nil {
		userID = db.NewID()
		_, err = store.Exec(
			"INSERT INTO users (id, username, password_hash, created_at) VALUES ($1, 'admin', $2, '2025-01-01T00:00:00Z')",
			userID, auth.HashToken("test-admin-password"),
		)
		require.NoError(t, err)
	}

	deviceID := db.NewID()
	revokedInt := 0
	if revoked {
		revokedInt = 1
	}
	_, err = store.Exec(
		`INSERT INTO devices (id, user_id, name, token_hash, refresh_token_hash, last_seen, revoked, created_at)
		 VALUES ($1, $2, $3, $4, $5, '2025-01-01T00:00:00Z', $6, '2025-01-01T00:00:00Z')`,
		deviceID, userID, name, auth.HashToken(token), auth.HashToken(refreshToken), revokedInt,
	)
	require.NoError(t, err)
	return deviceID
}

// --- Auth Middleware Tests ---

func TestAuthMiddleware_NoPassword_AllowsAll(t *testing.T) {
	srv, _ := setupAuthTestServer(t, "") // No admin password
	w := doRequest(t, srv, "GET", "/api/status", nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuthMiddleware_WithPassword_RejectsUnauthenticated(t *testing.T) {
	srv, _ := setupAuthTestServer(t, "secret123")

	// API route without auth header should get 401
	w := doRequest(t, srv, "GET", "/api/status", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp["error"], "missing or invalid Authorization header")
}

func TestAuthMiddleware_WithPassword_AcceptsValidToken(t *testing.T) {
	srv, store := setupAuthTestServer(t, "secret123")

	token := "valid-test-token-abc123"
	insertTestDevice(t, store, "test-device", token, "refresh-xyz", false)

	req := httptest.NewRequest("GET", "/api/status", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuthMiddleware_WithPassword_RejectsInvalidToken(t *testing.T) {
	srv, store := setupAuthTestServer(t, "secret123")
	insertTestDevice(t, store, "test-device", "real-token", "refresh-xyz", false)

	req := httptest.NewRequest("GET", "/api/status", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthMiddleware_WithPassword_RejectsRevokedDevice(t *testing.T) {
	srv, store := setupAuthTestServer(t, "secret123")

	token := "revoked-device-token"
	insertTestDevice(t, store, "revoked-device", token, "refresh-xyz", true)

	req := httptest.NewRequest("GET", "/api/status", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestAuthMiddleware_SkipsHealthEndpoint(t *testing.T) {
	srv, _ := setupAuthTestServer(t, "secret123")

	// Health should work without auth even when admin password is set
	w := doRequest(t, srv, "GET", "/health", nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuthMiddleware_SkipsAuthEndpoints(t *testing.T) {
	srv, _ := setupAuthTestServer(t, "secret123")

	// Device init should work without bearer token
	w := doRequest(t, srv, "POST", "/api/auth/device", deviceInitRequest{
		DeviceName: "test-device",
	})
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuthMiddleware_SkipsDeviceApprovalPage(t *testing.T) {
	srv, _ := setupAuthTestServer(t, "secret123")

	// Device approval page should be accessible without bearer token
	req := httptest.NewRequest("GET", "/device/authorize", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

// --- Device Flow Tests ---

func TestDeviceInit(t *testing.T) {
	srv, _ := setupAuthTestServer(t, "secret123")

	w := doRequest(t, srv, "POST", "/api/auth/device", deviceInitRequest{
		DeviceName: "my-laptop",
	})
	require.Equal(t, http.StatusOK, w.Code)

	var resp deviceInitResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.DeviceCode)
	assert.NotEmpty(t, resp.UserCode)
	assert.NotEmpty(t, resp.VerificationURI)
	assert.Greater(t, resp.ExpiresIn, 0)
	assert.Equal(t, 5, resp.Interval)
}

func TestDeviceInit_DefaultName(t *testing.T) {
	srv, _ := setupAuthTestServer(t, "secret123")

	w := doRequest(t, srv, "POST", "/api/auth/device", deviceInitRequest{})
	require.Equal(t, http.StatusOK, w.Code)

	var resp deviceInitResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.DeviceCode)
}

func TestDeviceToken_Pending(t *testing.T) {
	srv, _ := setupAuthTestServer(t, "secret123")

	// Initiate flow
	w := doRequest(t, srv, "POST", "/api/auth/device", deviceInitRequest{DeviceName: "test"})
	require.Equal(t, http.StatusOK, w.Code)

	var initResp deviceInitResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &initResp))

	// Poll before approval — should get 202
	w = doRequest(t, srv, "POST", "/api/auth/token", deviceTokenRequest{
		DeviceCode: initResp.DeviceCode,
	})
	assert.Equal(t, http.StatusAccepted, w.Code)

	var pendingResp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &pendingResp))
	assert.Equal(t, "authorization_pending", pendingResp["error"])
}

func TestDeviceToken_Expired(t *testing.T) {
	srv, _ := setupAuthTestServer(t, "secret123")

	w := doRequest(t, srv, "POST", "/api/auth/token", deviceTokenRequest{
		DeviceCode: "nonexistent-code",
	})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDeviceToken_Denied(t *testing.T) {
	srv, _ := setupAuthTestServer(t, "secret123")

	// Initiate flow
	w := doRequest(t, srv, "POST", "/api/auth/device", deviceInitRequest{DeviceName: "test"})
	var initResp deviceInitResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &initResp))

	// Deny the flow
	srv.flows.Deny(initResp.UserCode)

	// Poll after denial — should get 403
	w = doRequest(t, srv, "POST", "/api/auth/token", deviceTokenRequest{
		DeviceCode: initResp.DeviceCode,
	})
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestDeviceToken_Approved(t *testing.T) {
	srv, _ := setupAuthTestServer(t, "secret123")

	// Initiate flow
	w := doRequest(t, srv, "POST", "/api/auth/device", deviceInitRequest{DeviceName: "test"})
	var initResp deviceInitResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &initResp))

	// Approve the flow
	srv.flows.Approve(initResp.UserCode, "device-123", "token-abc", "refresh-xyz")

	// Poll after approval — should get 200 with tokens
	w = doRequest(t, srv, "POST", "/api/auth/token", deviceTokenRequest{
		DeviceCode: initResp.DeviceCode,
	})
	require.Equal(t, http.StatusOK, w.Code)

	var tokenResp deviceTokenResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &tokenResp))
	assert.Equal(t, "token-abc", tokenResp.AccessToken)
	assert.Equal(t, "refresh-xyz", tokenResp.RefreshToken)
	assert.Equal(t, "Bearer", tokenResp.TokenType)
	assert.Equal(t, "device-123", tokenResp.DeviceID)
}

// --- Full Device Approval Flow via Web UI ---

func TestApprovalPage_NoCode(t *testing.T) {
	srv, _ := setupAuthTestServer(t, "secret123")

	req := httptest.NewRequest("GET", "/device/authorize", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Enter the user code")
}

func TestApprovalPage_InvalidPassword(t *testing.T) {
	srv, _ := setupAuthTestServer(t, "secret123")

	req := httptest.NewRequest("GET", "/device/authorize?user_code=ABCD-1234&admin_password=wrong", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid admin password")
}

func TestApprovalPage_InvalidCode(t *testing.T) {
	srv, _ := setupAuthTestServer(t, "secret123")

	req := httptest.NewRequest("GET", "/device/authorize?user_code=XXXX-9999&admin_password=secret123", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid or expired user code")
}

func TestApprovalPage_ValidCode(t *testing.T) {
	srv, _ := setupAuthTestServer(t, "secret123")

	// Initiate a device flow
	w := doRequest(t, srv, "POST", "/api/auth/device", deviceInitRequest{DeviceName: "my-laptop"})
	var initResp deviceInitResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &initResp))

	// Load the approval page with valid code and password
	req := httptest.NewRequest("GET", "/device/authorize?user_code="+initResp.UserCode+"&admin_password=secret123", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "my-laptop")
	assert.Contains(t, rec.Body.String(), "Approve")
	assert.Contains(t, rec.Body.String(), "Deny")
}

func TestApprovalSubmit_Approve(t *testing.T) {
	srv, _ := setupAuthTestServer(t, "secret123")

	// Initiate device flow
	w := doRequest(t, srv, "POST", "/api/auth/device", deviceInitRequest{DeviceName: "my-laptop"})
	var initResp deviceInitResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &initResp))

	// Submit approval
	form := url.Values{
		"user_code":      {initResp.UserCode},
		"admin_password": {"secret123"},
		"action":         {"approve"},
	}
	req := httptest.NewRequest("POST", "/device/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "approved")

	// Now poll for token — should succeed
	w = doRequest(t, srv, "POST", "/api/auth/token", deviceTokenRequest{
		DeviceCode: initResp.DeviceCode,
	})
	require.Equal(t, http.StatusOK, w.Code)

	var tokenResp deviceTokenResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &tokenResp))
	assert.NotEmpty(t, tokenResp.AccessToken)
	assert.NotEmpty(t, tokenResp.RefreshToken)
	assert.Equal(t, "Bearer", tokenResp.TokenType)
}

func TestApprovalSubmit_Deny(t *testing.T) {
	srv, _ := setupAuthTestServer(t, "secret123")

	// Initiate device flow
	w := doRequest(t, srv, "POST", "/api/auth/device", deviceInitRequest{DeviceName: "evil-device"})
	var initResp deviceInitResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &initResp))

	// Submit denial
	form := url.Values{
		"user_code":      {initResp.UserCode},
		"admin_password": {"secret123"},
		"action":         {"deny"},
	}
	req := httptest.NewRequest("POST", "/device/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "denied")

	// Now poll for token — should get 403
	w = doRequest(t, srv, "POST", "/api/auth/token", deviceTokenRequest{
		DeviceCode: initResp.DeviceCode,
	})
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestApprovalSubmit_WrongPassword(t *testing.T) {
	srv, _ := setupAuthTestServer(t, "secret123")

	form := url.Values{
		"user_code":      {"ABCD-1234"},
		"admin_password": {"wrong"},
		"action":         {"approve"},
	}
	req := httptest.NewRequest("POST", "/device/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Invalid admin password")
}

// --- Token Refresh Tests ---

func TestTokenRefresh_Valid(t *testing.T) {
	srv, store := setupAuthTestServer(t, "secret123")

	originalToken := "original-access-token"
	originalRefresh := "original-refresh-token"
	deviceID := insertTestDevice(t, store, "test-device", originalToken, originalRefresh, false)

	w := doRequest(t, srv, "POST", "/api/auth/refresh", refreshRequest{
		RefreshToken: originalRefresh,
		DeviceID:     deviceID,
	})
	require.Equal(t, http.StatusOK, w.Code)

	var resp deviceTokenResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.AccessToken)
	assert.NotEmpty(t, resp.RefreshToken)
	assert.NotEqual(t, originalToken, resp.AccessToken)
	assert.NotEqual(t, originalRefresh, resp.RefreshToken)
	assert.Equal(t, deviceID, resp.DeviceID)
}

func TestTokenRefresh_InvalidToken(t *testing.T) {
	srv, store := setupAuthTestServer(t, "secret123")

	insertTestDevice(t, store, "test-device", "token-abc", "refresh-xyz", false)

	w := doRequest(t, srv, "POST", "/api/auth/refresh", refreshRequest{
		RefreshToken: "wrong-refresh",
		DeviceID:     "wrong-device",
	})
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestTokenRefresh_RevokedDevice(t *testing.T) {
	srv, store := setupAuthTestServer(t, "secret123")

	refreshToken := "revoked-refresh"
	deviceID := insertTestDevice(t, store, "revoked-device", "token-abc", refreshToken, true)

	w := doRequest(t, srv, "POST", "/api/auth/refresh", refreshRequest{
		RefreshToken: refreshToken,
		DeviceID:     deviceID,
	})
	assert.Equal(t, http.StatusForbidden, w.Code)
}

// --- Device Management API Tests ---

func TestListDevices(t *testing.T) {
	srv, store := setupAuthTestServer(t, "secret123")

	token := "admin-access-token"
	insertTestDevice(t, store, "device-1", token, "refresh-1", false)
	insertTestDevice(t, store, "device-2", "token-2", "refresh-2", false)

	req := httptest.NewRequest("GET", "/api/devices", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var devices []deviceInfo
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &devices))
	assert.Len(t, devices, 2)
}

func TestListDevices_Unauthorized(t *testing.T) {
	srv, _ := setupAuthTestServer(t, "secret123")

	w := doRequest(t, srv, "GET", "/api/devices", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRevokeDevice(t *testing.T) {
	srv, store := setupAuthTestServer(t, "secret123")

	adminToken := "admin-token"
	insertTestDevice(t, store, "admin", adminToken, "refresh-admin", false)
	targetID := insertTestDevice(t, store, "target", "target-token", "refresh-target", false)

	req := httptest.NewRequest("POST", "/api/devices/"+targetID+"/revoke", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "revoked", resp["status"])
}

func TestRevokeDevice_NotFound(t *testing.T) {
	srv, store := setupAuthTestServer(t, "secret123")

	adminToken := "admin-token"
	insertTestDevice(t, store, "admin", adminToken, "refresh-admin", false)

	req := httptest.NewRequest("POST", "/api/devices/nonexistent/revoke", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// --- Server-Level Sync Tests ---

func TestSyncPush_Conflict_ServerNewer(t *testing.T) {
	srv, store := setupTestServer(t)

	// Create a node on the server
	node, err := store.CreateNode(db.CreateNodeInput{
		Type:    "fact",
		Content: "Server version",
		Tags:    []string{"tier:pinned"},
	})
	require.NoError(t, err)

	// Push a change where the node's UpdatedAt is older than server version
	olderTime := node.UpdatedAt.Add(-1 * time.Hour)
	pushNode := map[string]any{
		"id":         node.ID,
		"type":       "fact",
		"content":    "Client version (older)",
		"updated_at": olderTime.Format(time.RFC3339),
	}

	w := doRequest(t, srv, "POST", "/api/sync/push", map[string]any{
		"device_id":    "test-device",
		"sync_version": 0,
		"changes":      []map[string]any{{"node": pushNode, "deleted": false}},
	})
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(1), resp["conflicts"])
	assert.Equal(t, float64(0), resp["accepted"])
}

func TestSyncPush_Delete(t *testing.T) {
	srv, store := setupTestServer(t)

	node, err := store.CreateNode(db.CreateNodeInput{
		Type:    "fact",
		Content: "To be deleted via sync",
	})
	require.NoError(t, err)

	w := doRequest(t, srv, "POST", "/api/sync/push", map[string]any{
		"device_id":    "test-device",
		"sync_version": 0,
		"changes": []map[string]any{
			{"node": map[string]any{"id": node.ID}, "deleted": true},
		},
	})
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(1), resp["accepted"])

	// Verify node is deleted
	_, err = store.GetNode(node.ID)
	assert.Error(t, err)
}

func TestSyncPull_WithChanges(t *testing.T) {
	srv, store := setupTestServer(t)

	// Create a node and bump its sync_version
	node, err := store.CreateNode(db.CreateNodeInput{
		Type:    "fact",
		Content: "Pullable fact",
		Tags:    []string{"tier:pinned"},
	})
	require.NoError(t, err)
	_, err = store.Exec("UPDATE nodes SET sync_version = 1 WHERE id = $1", node.ID)
	require.NoError(t, err)

	w := doRequest(t, srv, "POST", "/api/sync/pull", map[string]any{
		"device_id":     "test-device",
		"since_version": 0,
	})
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	changes := resp["changes"].([]any)
	assert.Len(t, changes, 1)
	assert.Equal(t, float64(1), resp["sync_version"])
}

// --- Repo Mapping Tests ---

func TestCreateRepoMapping(t *testing.T) {
	srv, store := setupAuthTestServer(t, "secret123")

	token := "admin-token"
	insertTestDevice(t, store, "admin", token, "refresh-admin", false)

	req := httptest.NewRequest("POST", "/api/repo-mappings", strings.NewReader(`{"normalized_url":"github.com/user/repo","project_tag":"myproject"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "github.com/user/repo", resp["normalized_url"])
	assert.Equal(t, "myproject", resp["project_tag"])
}
