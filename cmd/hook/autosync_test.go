package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupAutoSyncTestHome(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	ctxDir := filepath.Join(tmpDir, ".ctx")
	require.NoError(t, os.MkdirAll(ctxDir, 0755))
	return ctxDir
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0600))
}

func TestLoadAutoSyncConfig_EnvTrue(t *testing.T) {
	ctxDir := setupAutoSyncTestHome(t)
	t.Setenv("CTX_AUTO_SYNC", "true")

	// Set up remote and auth configs
	writeJSON(t, filepath.Join(ctxDir, "remote.json"), map[string]string{"url": "http://localhost:8377"})
	writeJSON(t, filepath.Join(ctxDir, "auth.json"), map[string]string{
		"token":     "test-token",
		"device_id": "device-123",
	})

	cfg := loadAutoSyncConfig()
	require.NotNil(t, cfg)
	assert.Equal(t, "http://localhost:8377", cfg.ServerURL)
	assert.Equal(t, "test-token", cfg.Token)
	assert.Equal(t, "device-123", cfg.DeviceID)
}

func TestLoadAutoSyncConfig_EnvOne(t *testing.T) {
	ctxDir := setupAutoSyncTestHome(t)
	t.Setenv("CTX_AUTO_SYNC", "1")

	writeJSON(t, filepath.Join(ctxDir, "remote.json"), map[string]string{"url": "http://localhost:8377"})
	writeJSON(t, filepath.Join(ctxDir, "auth.json"), map[string]string{
		"token":     "test-token",
		"device_id": "device-123",
	})

	cfg := loadAutoSyncConfig()
	assert.NotNil(t, cfg)
}

func TestLoadAutoSyncConfig_EnvFalse(t *testing.T) {
	setupAutoSyncTestHome(t)
	t.Setenv("CTX_AUTO_SYNC", "false")

	cfg := loadAutoSyncConfig()
	assert.Nil(t, cfg)
}

func TestLoadAutoSyncConfig_YAMLEnabled(t *testing.T) {
	ctxDir := setupAutoSyncTestHome(t)
	t.Setenv("CTX_AUTO_SYNC", "") // Unset env

	// Write server.yaml with auto_sync: true
	require.NoError(t, os.WriteFile(filepath.Join(ctxDir, "server.yaml"), []byte("port: 8377\nauto_sync: true\n"), 0600))
	writeJSON(t, filepath.Join(ctxDir, "remote.json"), map[string]string{"url": "http://localhost:8377"})
	writeJSON(t, filepath.Join(ctxDir, "auth.json"), map[string]string{
		"token":     "test-token",
		"device_id": "device-123",
	})

	cfg := loadAutoSyncConfig()
	require.NotNil(t, cfg)
	assert.Equal(t, "http://localhost:8377", cfg.ServerURL)
}

func TestLoadAutoSyncConfig_YAMLDisabled(t *testing.T) {
	ctxDir := setupAutoSyncTestHome(t)
	t.Setenv("CTX_AUTO_SYNC", "")

	// Write server.yaml with auto_sync: false
	require.NoError(t, os.WriteFile(filepath.Join(ctxDir, "server.yaml"), []byte("port: 8377\nauto_sync: false\n"), 0600))

	cfg := loadAutoSyncConfig()
	assert.Nil(t, cfg)
}

func TestLoadAutoSyncConfig_YAMLNoField(t *testing.T) {
	ctxDir := setupAutoSyncTestHome(t)
	t.Setenv("CTX_AUTO_SYNC", "")

	// server.yaml without auto_sync field — should default to false
	require.NoError(t, os.WriteFile(filepath.Join(ctxDir, "server.yaml"), []byte("port: 8377\n"), 0600))

	cfg := loadAutoSyncConfig()
	assert.Nil(t, cfg)
}

func TestLoadAutoSyncConfig_NoRemoteConfig(t *testing.T) {
	ctxDir := setupAutoSyncTestHome(t)
	t.Setenv("CTX_AUTO_SYNC", "true")

	// Only auth, no remote
	writeJSON(t, filepath.Join(ctxDir, "auth.json"), map[string]string{"token": "test-token"})

	cfg := loadAutoSyncConfig()
	assert.Nil(t, cfg)
}

func TestLoadAutoSyncConfig_NoAuthConfig(t *testing.T) {
	ctxDir := setupAutoSyncTestHome(t)
	t.Setenv("CTX_AUTO_SYNC", "true")

	// Only remote, no auth
	writeJSON(t, filepath.Join(ctxDir, "remote.json"), map[string]string{"url": "http://localhost:8377"})

	cfg := loadAutoSyncConfig()
	assert.Nil(t, cfg)
}

func TestLoadAutoSyncConfig_NoConfig(t *testing.T) {
	setupAutoSyncTestHome(t)
	t.Setenv("CTX_AUTO_SYNC", "")

	// No server.yaml at all
	cfg := loadAutoSyncConfig()
	assert.Nil(t, cfg)
}
