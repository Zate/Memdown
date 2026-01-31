package db_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zate/ctx/internal/db"
)

func TestDatabaseOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(path)
	require.NoError(t, err)
	defer d.Close()
}

func TestDatabaseOpenCreatesDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "dir", "test.db")
	d, err := db.Open(path)
	require.NoError(t, err)
	defer d.Close()
}

func TestDatabaseMigrationIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	d1, err := db.Open(path)
	require.NoError(t, err)
	d1.Close()

	// Open again - should not fail
	d2, err := db.Open(path)
	require.NoError(t, err)
	d2.Close()
}

func TestDefaultViewCreated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(path)
	require.NoError(t, err)
	defer d.Close()

	var name string
	err = d.QueryRow("SELECT name FROM views WHERE name = 'default'").Scan(&name)
	assert.NoError(t, err)
	assert.Equal(t, "default", name)
}
