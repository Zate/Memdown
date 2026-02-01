package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHooksAlreadyConfigured_Empty(t *testing.T) {
	settings := map[string]any{}
	assert.False(t, hooksAlreadyConfigured(settings))
}

func TestHooksAlreadyConfigured_NoCtxHooks(t *testing.T) {
	settings := map[string]any{
		"hooks": map[string]any{
			"SessionStart": []any{},
			"Stop":         []any{},
		},
	}
	assert.False(t, hooksAlreadyConfigured(settings))
}

func TestHooksAlreadyConfigured_AllPresent(t *testing.T) {
	settings := map[string]any{
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{
					"matcher": "",
					"hooks": []any{
						map[string]any{"type": "command", "command": "ctx hook session-start"},
					},
				},
			},
			"UserPromptSubmit": []any{
				map[string]any{
					"matcher": "",
					"hooks": []any{
						map[string]any{"type": "command", "command": "ctx hook prompt-submit"},
					},
				},
			},
			"Stop": []any{
				map[string]any{
					"matcher": "",
					"hooks": []any{
						map[string]any{"type": "command", "command": "ctx hook stop"},
					},
				},
			},
		},
	}
	assert.True(t, hooksAlreadyConfigured(settings))
}

func TestHooksAlreadyConfigured_Partial(t *testing.T) {
	settings := map[string]any{
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{
					"matcher": "",
					"hooks": []any{
						map[string]any{"type": "command", "command": "ctx hook session-start"},
					},
				},
			},
		},
	}
	assert.False(t, hooksAlreadyConfigured(settings))
}

func TestMergeHooks_IntoEmpty(t *testing.T) {
	settings := map[string]any{}
	mergeHooks(settings)

	hooks, ok := settings["hooks"].(map[string]any)
	require.True(t, ok)

	for _, hookType := range []string{"SessionStart", "UserPromptSubmit", "Stop"} {
		arr, ok := hooks[hookType].([]any)
		require.True(t, ok, "missing %s", hookType)
		assert.Len(t, arr, 1)
	}
}

func TestMergeHooks_PreservesExisting(t *testing.T) {
	settings := map[string]any{
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{
					"matcher": "",
					"hooks": []any{
						map[string]any{"type": "command", "command": "some-other-hook"},
					},
				},
			},
			"SessionEnd": []any{},
			"PreCompact": []any{},
		},
	}

	mergeHooks(settings)

	hooks := settings["hooks"].(map[string]any)

	// SessionStart should have both entries
	sessionStart := hooks["SessionStart"].([]any)
	assert.Len(t, sessionStart, 2, "should preserve existing + add ctx")

	// Other hook types should still exist
	assert.NotNil(t, hooks["SessionEnd"])
	assert.NotNil(t, hooks["PreCompact"])

	// Ctx hooks should be present
	assert.True(t, hookTypeHasCtx(hooks, "SessionStart"))
	assert.True(t, hookTypeHasCtx(hooks, "UserPromptSubmit"))
	assert.True(t, hookTypeHasCtx(hooks, "Stop"))
}

func TestMergeHooks_Idempotent(t *testing.T) {
	settings := map[string]any{}
	mergeHooks(settings)
	mergeHooks(settings) // second call

	hooks := settings["hooks"].(map[string]any)
	for _, hookType := range []string{"SessionStart", "UserPromptSubmit", "Stop"} {
		arr := hooks[hookType].([]any)
		assert.Len(t, arr, 1, "%s should not duplicate", hookType)
	}
}

func TestMergeHooks_RoundtripThroughJSON(t *testing.T) {
	// Simulate reading settings.json that already has ctx hooks
	original := map[string]any{
		"model": "opus",
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{
					"matcher": "",
					"hooks": []any{
						map[string]any{"type": "command", "command": "ctx hook session-start"},
					},
				},
			},
			"UserPromptSubmit": []any{
				map[string]any{
					"matcher": "",
					"hooks": []any{
						map[string]any{"type": "command", "command": "ctx hook prompt-submit"},
					},
				},
			},
			"Stop": []any{
				map[string]any{
					"matcher": "",
					"hooks": []any{
						map[string]any{"type": "command", "command": "ctx hook stop"},
					},
				},
			},
		},
	}

	// Marshal and unmarshal (simulates file read)
	data, err := json.Marshal(original)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))

	// Should detect as already configured
	assert.True(t, hooksAlreadyConfigured(parsed))
}

func TestFilesIdentical(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "a")
	f2 := filepath.Join(dir, "b")
	f3 := filepath.Join(dir, "c")

	require.NoError(t, os.WriteFile(f1, []byte("hello"), 0644))
	require.NoError(t, os.WriteFile(f2, []byte("hello"), 0644))
	require.NoError(t, os.WriteFile(f3, []byte("world"), 0644))

	same, err := filesIdentical(f1, f2)
	require.NoError(t, err)
	assert.True(t, same)

	same, err = filesIdentical(f1, f3)
	require.NoError(t, err)
	assert.False(t, same)
}

func TestFilesIdentical_MissingFile(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "a")
	require.NoError(t, os.WriteFile(f1, []byte("hello"), 0644))

	_, err := filesIdentical(f1, filepath.Join(dir, "nonexistent"))
	assert.Error(t, err)
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")

	content := []byte("binary content here")
	require.NoError(t, os.WriteFile(src, content, 0644))

	require.NoError(t, copyFile(src, dst, 0755))

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, content, got)

	info, err := os.Stat(dst)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0755), info.Mode().Perm())
}

func TestDirInPath(t *testing.T) {
	// PATH is set, so test with known values
	assert.False(t, dirInPath("/nonexistent/unlikely/path"))
}
