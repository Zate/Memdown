package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFindCtxBinary(t *testing.T) {
	// Should not error — either finds ctx in PATH or falls back to current executable
	path, err := findCtxBinary()
	assert.NoError(t, err)
	assert.NotEmpty(t, path)
}
