package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeviceFlowStore_Initiate(t *testing.T) {
	store := NewDeviceFlowStore()
	state := store.Initiate("test-device")

	assert.NotEmpty(t, state.DeviceCode)
	assert.NotEmpty(t, state.UserCode)
	assert.Equal(t, "test-device", state.DeviceName)
	assert.False(t, state.Approved)
	assert.False(t, state.Denied)
	assert.True(t, state.ExpiresAt.After(time.Now()))
}

func TestDeviceFlowStore_GetByDeviceCode(t *testing.T) {
	store := NewDeviceFlowStore()
	state := store.Initiate("test-device")

	found := store.GetByDeviceCode(state.DeviceCode)
	require.NotNil(t, found)
	assert.Equal(t, state.UserCode, found.UserCode)

	// Not found
	assert.Nil(t, store.GetByDeviceCode("nonexistent"))
}

func TestDeviceFlowStore_GetByUserCode(t *testing.T) {
	store := NewDeviceFlowStore()
	state := store.Initiate("test-device")

	found := store.GetByUserCode(state.UserCode)
	require.NotNil(t, found)
	assert.Equal(t, state.DeviceCode, found.DeviceCode)

	// Not found
	assert.Nil(t, store.GetByUserCode("XXXX-9999"))
}

func TestDeviceFlowStore_Approve(t *testing.T) {
	store := NewDeviceFlowStore()
	state := store.Initiate("test-device")

	ok := store.Approve(state.UserCode, "device-123", "token-abc", "refresh-xyz")
	assert.True(t, ok)

	found := store.GetByDeviceCode(state.DeviceCode)
	require.NotNil(t, found)
	assert.True(t, found.Approved)
	assert.Equal(t, "device-123", found.DeviceID)
	assert.Equal(t, "token-abc", found.Token)
	assert.Equal(t, "refresh-xyz", found.RefreshToken)
}

func TestDeviceFlowStore_Deny(t *testing.T) {
	store := NewDeviceFlowStore()
	state := store.Initiate("test-device")

	ok := store.Deny(state.UserCode)
	assert.True(t, ok)

	found := store.GetByDeviceCode(state.DeviceCode)
	require.NotNil(t, found)
	assert.True(t, found.Denied)
}

func TestDeviceFlowStore_Cleanup(t *testing.T) {
	store := NewDeviceFlowStore()

	// Create and approve a flow (should be cleaned up)
	s1 := store.Initiate("approved")
	store.Approve(s1.UserCode, "d1", "t1", "r1")

	// Create and deny a flow (should be cleaned up)
	s2 := store.Initiate("denied")
	store.Deny(s2.UserCode)

	// Create an active flow (should NOT be cleaned up)
	s3 := store.Initiate("active")

	store.Cleanup()

	assert.Nil(t, store.GetByDeviceCode(s1.DeviceCode))
	assert.Nil(t, store.GetByDeviceCode(s2.DeviceCode))
	assert.NotNil(t, store.GetByDeviceCode(s3.DeviceCode))
}

func TestHashToken(t *testing.T) {
	hash1 := HashToken("test-token")
	hash2 := HashToken("test-token")
	hash3 := HashToken("different-token")

	assert.Equal(t, hash1, hash2)
	assert.NotEqual(t, hash1, hash3)
	assert.Len(t, hash1, 64) // SHA-256 hex
}

func TestGenerateToken(t *testing.T) {
	t1 := GenerateToken()
	t2 := GenerateToken()
	assert.NotEqual(t, t1, t2)
	assert.Len(t, t1, 64) // 32 bytes = 64 hex chars
}

func TestGenerateRefreshToken(t *testing.T) {
	t1 := GenerateRefreshToken()
	t2 := GenerateRefreshToken()
	assert.NotEqual(t, t1, t2)
	assert.Len(t, t1, 96) // 48 bytes = 96 hex chars
}

func TestUserCodeFormat(t *testing.T) {
	store := NewDeviceFlowStore()
	state := store.Initiate("test")

	// Should be format XXXX-XXXX
	assert.Len(t, state.UserCode, 9)
	assert.Equal(t, byte('-'), state.UserCode[4])
}
