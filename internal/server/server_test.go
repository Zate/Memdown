package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zate/ctx/internal/db"
	"github.com/zate/ctx/testutil"
)

func setupTestServer(t *testing.T) (*Server, db.Store) {
	t.Helper()
	store := testutil.SetupTestDB(t)
	srv := New(store, DefaultConfig())
	return srv, store
}

func doRequest(t *testing.T, srv *Server, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		err := json.NewEncoder(&buf).Encode(body)
		require.NoError(t, err)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w
}

func TestHealthEndpoint(t *testing.T) {
	srv, _ := setupTestServer(t)
	w := doRequest(t, srv, "GET", "/health", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "ok", resp["status"])
}

func TestStatusEndpoint(t *testing.T) {
	srv, _ := setupTestServer(t)
	w := doRequest(t, srv, "GET", "/api/status", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp, "total_nodes")
	assert.Contains(t, resp, "total_tokens")
}

func TestNodeCRUD(t *testing.T) {
	srv, _ := setupTestServer(t)

	// Create
	w := doRequest(t, srv, "POST", "/api/nodes", createNodeRequest{
		Type:    "fact",
		Content: "Test fact content",
		Tags:    []string{"tier:pinned", "test"},
	})
	require.Equal(t, http.StatusCreated, w.Code)

	var node db.Node
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &node))
	assert.Equal(t, "fact", node.Type)
	assert.Equal(t, "Test fact content", node.Content)
	assert.Contains(t, node.Tags, "tier:pinned")

	// Get
	w = doRequest(t, srv, "GET", "/api/nodes/"+node.ID, nil)
	require.Equal(t, http.StatusOK, w.Code)

	var fetched db.Node
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &fetched))
	assert.Equal(t, node.ID, fetched.ID)

	// Get with short prefix
	w = doRequest(t, srv, "GET", "/api/nodes/"+node.ID[:8], nil)
	require.Equal(t, http.StatusOK, w.Code)

	// Update
	newContent := "Updated content"
	w = doRequest(t, srv, "PATCH", "/api/nodes/"+node.ID, updateNodeRequest{
		Content: &newContent,
	})
	require.Equal(t, http.StatusOK, w.Code)

	var updated db.Node
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &updated))
	assert.Equal(t, "Updated content", updated.Content)

	// Delete
	w = doRequest(t, srv, "DELETE", "/api/nodes/"+node.ID, nil)
	require.Equal(t, http.StatusOK, w.Code)

	// Verify deleted
	w = doRequest(t, srv, "GET", "/api/nodes/"+node.ID, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestEdges(t *testing.T) {
	srv, store := setupTestServer(t)

	n1, err := store.CreateNode(db.CreateNodeInput{Type: "fact", Content: "Node 1"})
	require.NoError(t, err)
	n2, err := store.CreateNode(db.CreateNodeInput{Type: "fact", Content: "Node 2"})
	require.NoError(t, err)

	// Create edge
	w := doRequest(t, srv, "POST", "/api/edges", createEdgeRequest{
		FromID: n1.ID,
		ToID:   n2.ID,
		Type:   "RELATES_TO",
	})
	require.Equal(t, http.StatusCreated, w.Code)

	// Get edges
	w = doRequest(t, srv, "GET", "/api/edges/"+n1.ID, nil)
	require.Equal(t, http.StatusOK, w.Code)

	var edges []*db.Edge
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &edges))
	assert.Len(t, edges, 1)
	assert.Equal(t, "RELATES_TO", edges[0].Type)

	// Delete edge
	w = doRequest(t, srv, "DELETE", "/api/edges", deleteEdgeRequest{
		FromID: n1.ID,
		ToID:   n2.ID,
		Type:   "RELATES_TO",
	})
	require.Equal(t, http.StatusOK, w.Code)
}

func TestTags(t *testing.T) {
	srv, store := setupTestServer(t)

	node, err := store.CreateNode(db.CreateNodeInput{Type: "fact", Content: "Taggable node"})
	require.NoError(t, err)

	// Add tags
	w := doRequest(t, srv, "POST", "/api/nodes/"+node.ID+"/tags", tagsRequest{
		Tags: []string{"foo", "bar"},
	})
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	tags := resp["tags"].([]any)
	assert.Len(t, tags, 2)

	// Remove tag
	w = doRequest(t, srv, "DELETE", "/api/nodes/"+node.ID+"/tags", tagsRequest{
		Tags: []string{"foo"},
	})
	require.Equal(t, http.StatusOK, w.Code)

	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	tags = resp["tags"].([]any)
	assert.Len(t, tags, 1)
}

func TestQuery(t *testing.T) {
	srv, store := setupTestServer(t)

	_, err := store.CreateNode(db.CreateNodeInput{
		Type:    "fact",
		Content: "A fact",
		Tags:    []string{"tier:pinned"},
	})
	require.NoError(t, err)

	_, err = store.CreateNode(db.CreateNodeInput{
		Type:    "decision",
		Content: "A decision",
		Tags:    []string{"tier:reference"},
	})
	require.NoError(t, err)

	w := doRequest(t, srv, "POST", "/api/query", queryRequest{
		Query: "type:fact",
	})
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(1), resp["count"])
}

func TestCompose(t *testing.T) {
	srv, store := setupTestServer(t)

	n, err := store.CreateNode(db.CreateNodeInput{
		Type:    "fact",
		Content: "Composable fact",
		Tags:    []string{"tier:pinned"},
	})
	require.NoError(t, err)

	w := doRequest(t, srv, "POST", "/api/compose", composeRequest{
		IDs: []string{n.ID},
	})
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(1), resp["node_count"])
}

func TestComposeWithTemplate(t *testing.T) {
	srv, store := setupTestServer(t)

	n, err := store.CreateNode(db.CreateNodeInput{
		Type:    "fact",
		Content: "Template fact",
		Tags:    []string{"tier:pinned"},
	})
	require.NoError(t, err)

	w := doRequest(t, srv, "POST", "/api/compose", composeRequest{
		IDs:      []string{n.ID},
		Template: "default",
	})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/markdown")
	assert.Contains(t, w.Body.String(), "Template fact")
}

func TestSyncPush(t *testing.T) {
	srv, store := setupTestServer(t)

	// Create a node to push
	node, err := store.CreateNode(db.CreateNodeInput{
		Type:    "fact",
		Content: "Pushable fact",
		Tags:    []string{"tier:pinned"},
	})
	require.NoError(t, err)

	w := doRequest(t, srv, "POST", "/api/sync/push", map[string]any{
		"device_id":    "test-device",
		"sync_version": 0,
		"changes": []map[string]any{
			{"node": node, "deleted": false},
		},
	})
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp, "accepted")
}

func TestSyncPull(t *testing.T) {
	srv, _ := setupTestServer(t)

	w := doRequest(t, srv, "POST", "/api/sync/pull", map[string]any{
		"device_id":     "test-device",
		"since_version": 0,
	})
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp, "changes")
}

func TestAdminDashboard(t *testing.T) {
	srv, _ := setupTestServer(t)
	w := doRequest(t, srv, "GET", "/admin", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/html")
	assert.Contains(t, w.Body.String(), "Dashboard")
}

func TestNodeBrowser(t *testing.T) {
	srv, store := setupTestServer(t)
	_, _ = store.CreateNode(db.CreateNodeInput{Type: "fact", Content: "Browsable fact"})

	w := doRequest(t, srv, "GET", "/admin/nodes", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Browsable fact")
}

func TestHealthEndpointResponse(t *testing.T) {
	srv, _ := setupTestServer(t)
	w := doRequest(t, srv, "GET", "/health", nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestCreateNodeValidation(t *testing.T) {
	srv, _ := setupTestServer(t)

	// Empty body
	w := doRequest(t, srv, "POST", "/api/nodes", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Invalid type
	w = doRequest(t, srv, "POST", "/api/nodes", createNodeRequest{
		Type:    "invalid",
		Content: "test",
	})
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Empty content
	w = doRequest(t, srv, "POST", "/api/nodes", createNodeRequest{
		Type:    "fact",
		Content: "",
	})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetNodeNotFound(t *testing.T) {
	srv, _ := setupTestServer(t)
	w := doRequest(t, srv, "GET", "/api/nodes/nonexistent", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
