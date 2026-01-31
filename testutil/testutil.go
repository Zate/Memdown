package testutil

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zate/ctx/internal/db"
)

// SetupTestDB creates a test database and returns it.
func SetupTestDB(t *testing.T) *db.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { database.Close() })
	return database
}

// Ptr returns a pointer to the value.
func Ptr[T any](v T) *T {
	return &v
}
