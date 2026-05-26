// Copyright (c) 2026 John Dewey

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to
// deal in the Software without restriction, including without limitation the
// rights to use, copy, modify, merge, publish, distribute, sublicense, and/or
// sell copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.

package configmerge_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/internal/configmerge"
)

// writeJSON marshals v and writes it to path, creating parent dirs.
func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

// readJSON reads path and unmarshals it into map[string]any.
func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(data, &out))
	return out
}

func TestMergeMCP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		initial    any
		serverName string
		config     map[string]any
		setupPath  func(t *testing.T) string
		wantErr    string
		check      func(t *testing.T, path string)
	}{
		{
			name:       "adds server to empty settings",
			serverName: "my-api",
			config:     map[string]any{"type": "remote", "url": "https://mcp.example.com/v1"},
			check: func(t *testing.T, path string) {
				t.Helper()
				doc := readJSON(t, path)
				servers, ok := doc["mcpServers"].(map[string]any)
				require.True(t, ok)
				srv, ok := servers["my-api"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "remote", srv["type"])
				assert.Equal(t, "https://mcp.example.com/v1", srv["url"])
			},
		},
		{
			name:       "adds server when settings file does not exist",
			serverName: "new-srv",
			config:     map[string]any{"type": "stdio"},
			check: func(t *testing.T, path string) {
				t.Helper()
				doc := readJSON(t, path)
				servers, ok := doc["mcpServers"].(map[string]any)
				require.True(t, ok)
				assert.Contains(t, servers, "new-srv")
			},
		},
		{
			name: "returns error when server name already exists",
			initial: map[string]any{
				"mcpServers": map[string]any{"my-api": map[string]any{"type": "remote"}},
			},
			serverName: "my-api",
			config:     map[string]any{"type": "stdio"},
			wantErr:    `MCP server "my-api" already exists`,
		},
		{
			name: "preserves existing servers when adding new one",
			initial: map[string]any{
				"mcpServers": map[string]any{"old-srv": map[string]any{"type": "stdio"}},
			},
			serverName: "new-srv",
			config:     map[string]any{"type": "remote", "url": "https://example.com"},
			check: func(t *testing.T, path string) {
				t.Helper()
				doc := readJSON(t, path)
				servers, ok := doc["mcpServers"].(map[string]any)
				require.True(t, ok)
				assert.Contains(t, servers, "old-srv")
				assert.Contains(t, servers, "new-srv")
			},
		},
		{
			name:       "returns error when settings file is unreadable",
			serverName: "my-api",
			config:     map[string]any{"type": "stdio"},
			setupPath: func(t *testing.T) string {
				t.Helper()
				p := filepath.Join(t.TempDir(), "settings.json")
				require.NoError(t, os.WriteFile(p, []byte(`{}`), 0o000))
				t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
				return p
			},
			wantErr: "read",
		},
		{
			name:       "returns error when settings file contains invalid JSON",
			serverName: "my-api",
			config:     map[string]any{"type": "stdio"},
			setupPath: func(t *testing.T) string {
				t.Helper()
				p := filepath.Join(t.TempDir(), "settings.json")
				require.NoError(t, os.WriteFile(p, []byte(`{invalid`), 0o644))
				return p
			},
			wantErr: "parse",
		},
		{
			name:       "returns error when parent dir is not writable",
			serverName: "my-api",
			config:     map[string]any{"type": "stdio"},
			setupPath: func(t *testing.T) string {
				t.Helper()
				roDir := t.TempDir()
				require.NoError(t, os.Chmod(roDir, 0o555))
				t.Cleanup(func() { _ = os.Chmod(roDir, 0o755) })
				return filepath.Join(roDir, "sub", "settings.json")
			},
			wantErr: "mkdir",
		},
		{
			name:       "returns error when settings file is not writable",
			serverName: "my-api",
			config:     map[string]any{"type": "stdio"},
			setupPath: func(t *testing.T) string {
				t.Helper()
				p := filepath.Join(t.TempDir(), "settings.json")
				require.NoError(t, os.WriteFile(p, []byte(`{}`), 0o444))
				t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
				return p
			},
			wantErr: "write",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var settingsPath string
			if tt.setupPath != nil {
				settingsPath = tt.setupPath(t)
			} else {
				settingsPath = filepath.Join(t.TempDir(), "settings.json")
				if tt.initial != nil {
					writeJSON(t, settingsPath, tt.initial)
				}
			}

			err := configmerge.MergeMCP(settingsPath, tt.serverName, tt.config)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			if tt.check != nil {
				tt.check(t, settingsPath)
			}
		})
	}
}

func TestMergeHooks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		initial    any
		pluginName string
		hooks      map[string]any
		setupPath  func(t *testing.T) string
		wantErr    string
		check      func(t *testing.T, path string)
	}{
		{
			name:       "appends hook entry to empty settings",
			pluginName: "my-plugin",
			hooks: map[string]any{
				"PreToolUse": []any{
					map[string]any{
						"matcher": "Bash",
						"hooks": []any{
							map[string]any{"type": "command", "command": "lint.sh"},
						},
					},
				},
			},
			check: func(t *testing.T, path string) {
				t.Helper()
				doc := readJSON(t, path)
				hooks, ok := doc["hooks"].(map[string]any)
				require.True(t, ok)
				entries, ok := hooks["PreToolUse"].([]any)
				require.True(t, ok)
				require.Len(t, entries, 1)
				entry, ok := entries[0].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "my-plugin", entry["_plugin"])
				assert.Equal(t, "Bash", entry["matcher"])
			},
		},
		{
			name:       "appends to existing hook event entries",
			pluginName: "plugin-b",
			initial: map[string]any{
				"hooks": map[string]any{
					"PreToolUse": []any{
						map[string]any{"matcher": "Write", "_plugin": "plugin-a"},
					},
				},
			},
			hooks: map[string]any{
				"PreToolUse": []any{
					map[string]any{"matcher": "Bash"},
				},
			},
			check: func(t *testing.T, path string) {
				t.Helper()
				doc := readJSON(t, path)
				hooks, ok := doc["hooks"].(map[string]any)
				require.True(t, ok)
				entries, ok := hooks["PreToolUse"].([]any)
				require.True(t, ok)
				assert.Len(t, entries, 2)
			},
		},
		{
			name:       "tags injected entries with _plugin field",
			pluginName: "tagger",
			hooks: map[string]any{
				"PostToolUse": []any{
					map[string]any{"matcher": "Read"},
				},
			},
			check: func(t *testing.T, path string) {
				t.Helper()
				doc := readJSON(t, path)
				hooks := doc["hooks"].(map[string]any)
				entries := hooks["PostToolUse"].([]any)
				entry := entries[0].(map[string]any)
				assert.Equal(t, "tagger", entry["_plugin"])
			},
		},
		{
			name:       "creates settings file when absent",
			pluginName: "new-plugin",
			hooks: map[string]any{
				"PreToolUse": []any{map[string]any{"matcher": "Bash"}},
			},
			check: func(t *testing.T, path string) {
				t.Helper()
				_, err := os.Stat(path)
				assert.NoError(t, err)
			},
		},
		{
			name:       "returns error when settings file is unreadable",
			pluginName: "my-plugin",
			hooks:      map[string]any{"PreToolUse": []any{map[string]any{"matcher": "Bash"}}},
			setupPath: func(t *testing.T) string {
				t.Helper()
				p := filepath.Join(t.TempDir(), "settings.json")
				require.NoError(t, os.WriteFile(p, []byte(`{}`), 0o000))
				t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
				return p
			},
			wantErr: "read",
		},
		{
			name:       "skips hook event whose value is not a slice",
			pluginName: "my-plugin",
			hooks:      map[string]any{"PreToolUse": "not-a-slice"},
			check: func(t *testing.T, path string) {
				t.Helper()
				doc := readJSON(t, path)
				hooks, ok := doc["hooks"].(map[string]any)
				require.True(t, ok)
				// The non-array event key must not appear.
				assert.NotContains(t, hooks, "PreToolUse")
			},
		},
		{
			name:       "skips hook entry whose value is not a map",
			pluginName: "my-plugin",
			hooks: map[string]any{
				"PreToolUse": []any{"not-a-map"},
			},
			check: func(t *testing.T, path string) {
				t.Helper()
				doc := readJSON(t, path)
				hooks, ok := doc["hooks"].(map[string]any)
				require.True(t, ok)
				entries, ok := hooks["PreToolUse"].([]any)
				require.True(t, ok)
				assert.Empty(t, entries)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var settingsPath string
			if tt.setupPath != nil {
				settingsPath = tt.setupPath(t)
			} else {
				settingsPath = filepath.Join(t.TempDir(), "settings.json")
				if tt.initial != nil {
					writeJSON(t, settingsPath, tt.initial)
				}
			}

			err := configmerge.MergeHooks(settingsPath, tt.pluginName, tt.hooks)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			if tt.check != nil {
				tt.check(t, settingsPath)
			}
		})
	}
}

func TestMergeSettings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		initial   any
		fragment  map[string]any
		setupPath func(t *testing.T) string
		wantErr   string
		check     func(t *testing.T, path string)
	}{
		{
			name:     "merges fragment into empty settings",
			fragment: map[string]any{"theme": "dark", "fontSize": float64(14)},
			check: func(t *testing.T, path string) {
				t.Helper()
				doc := readJSON(t, path)
				assert.Equal(t, "dark", doc["theme"])
				assert.Equal(t, float64(14), doc["fontSize"])
			},
		},
		{
			name:     "creates file when absent",
			fragment: map[string]any{"key": "val"},
			check: func(t *testing.T, path string) {
				t.Helper()
				_, err := os.Stat(path)
				assert.NoError(t, err)
			},
		},
		{
			name:     "does not touch keys outside the fragment",
			initial:  map[string]any{"untouched": "value", "overwrite": "old"},
			fragment: map[string]any{"overwrite": "new"},
			check: func(t *testing.T, path string) {
				t.Helper()
				doc := readJSON(t, path)
				assert.Equal(t, "value", doc["untouched"])
				assert.Equal(t, "new", doc["overwrite"])
			},
		},
		{
			name:     "returns error when settings file is unreadable",
			fragment: map[string]any{"key": "val"},
			setupPath: func(t *testing.T) string {
				t.Helper()
				p := filepath.Join(t.TempDir(), "settings.json")
				require.NoError(t, os.WriteFile(p, []byte(`{}`), 0o000))
				t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
				return p
			},
			wantErr: "read",
		},
		{
			name:     "treats JSON null file as empty document",
			fragment: map[string]any{"key": "val"},
			setupPath: func(t *testing.T) string {
				t.Helper()
				p := filepath.Join(t.TempDir(), "settings.json")
				require.NoError(t, os.WriteFile(p, []byte(`null`), 0o644))
				return p
			},
			check: func(t *testing.T, path string) {
				t.Helper()
				doc := readJSON(t, path)
				assert.Equal(t, "val", doc["key"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var settingsPath string
			if tt.setupPath != nil {
				settingsPath = tt.setupPath(t)
			} else {
				settingsPath = filepath.Join(t.TempDir(), "settings.json")
				if tt.initial != nil {
					writeJSON(t, settingsPath, tt.initial)
				}
			}

			err := configmerge.MergeSettings(settingsPath, tt.fragment)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			if tt.check != nil {
				tt.check(t, settingsPath)
			}
		})
	}
}

func TestRemoveMCP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		initial    any
		serverName string
		setupPath  func(t *testing.T) string
		wantErr    string
		check      func(t *testing.T, path string)
	}{
		{
			name: "removes existing server",
			initial: map[string]any{
				"mcpServers": map[string]any{
					"my-api": map[string]any{"type": "remote"},
					"other":  map[string]any{"type": "stdio"},
				},
			},
			serverName: "my-api",
			check: func(t *testing.T, path string) {
				t.Helper()
				doc := readJSON(t, path)
				servers, ok := doc["mcpServers"].(map[string]any)
				require.True(t, ok)
				assert.NotContains(t, servers, "my-api")
				assert.Contains(t, servers, "other")
			},
		},
		{
			name:       "no-op when server does not exist",
			initial:    map[string]any{"mcpServers": map[string]any{"other": map[string]any{}}},
			serverName: "missing",
			check: func(t *testing.T, path string) {
				t.Helper()
				doc := readJSON(t, path)
				servers := doc["mcpServers"].(map[string]any)
				assert.Contains(t, servers, "other")
			},
		},
		{
			name:       "no-op when settings file does not exist",
			serverName: "anything",
		},
		{
			name:       "no-op when mcpServers key is absent",
			initial:    map[string]any{"theme": "dark"},
			serverName: "my-api",
			check: func(t *testing.T, path string) {
				t.Helper()
				doc := readJSON(t, path)
				assert.NotContains(t, doc, "mcpServers")
			},
		},
		{
			name:       "returns error when settings file is unreadable",
			serverName: "my-api",
			setupPath: func(t *testing.T) string {
				t.Helper()
				p := filepath.Join(t.TempDir(), "settings.json")
				require.NoError(t, os.WriteFile(p, []byte(`{}`), 0o000))
				t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
				return p
			},
			wantErr: "read",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var settingsPath string
			if tt.setupPath != nil {
				settingsPath = tt.setupPath(t)
			} else {
				settingsPath = filepath.Join(t.TempDir(), "settings.json")
				if tt.initial != nil {
					writeJSON(t, settingsPath, tt.initial)
				}
			}

			err := configmerge.RemoveMCP(settingsPath, tt.serverName)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			if tt.check != nil {
				tt.check(t, settingsPath)
			}
		})
	}
}

func TestRemoveHooks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		initial    any
		pluginName string
		setupPath  func(t *testing.T) string
		wantErr    string
		check      func(t *testing.T, path string)
	}{
		{
			name: "removes entries tagged with plugin name",
			initial: map[string]any{
				"hooks": map[string]any{
					"PreToolUse": []any{
						map[string]any{"matcher": "Bash", "_plugin": "my-plugin"},
						map[string]any{"matcher": "Write", "_plugin": "other-plugin"},
					},
				},
			},
			pluginName: "my-plugin",
			check: func(t *testing.T, path string) {
				t.Helper()
				doc := readJSON(t, path)
				hooks := doc["hooks"].(map[string]any)
				entries := hooks["PreToolUse"].([]any)
				require.Len(t, entries, 1)
				entry := entries[0].(map[string]any)
				assert.Equal(t, "other-plugin", entry["_plugin"])
			},
		},
		{
			name:       "no-op when settings file does not exist",
			pluginName: "anything",
		},
		{
			name:       "no-op when hooks key is absent",
			initial:    map[string]any{"theme": "dark"},
			pluginName: "my-plugin",
			check: func(t *testing.T, path string) {
				t.Helper()
				doc := readJSON(t, path)
				assert.NotContains(t, doc, "hooks")
			},
		},
		{
			name: "leaves entries without _plugin field untouched",
			initial: map[string]any{
				"hooks": map[string]any{
					"PreToolUse": []any{
						map[string]any{"matcher": "Bash"},
					},
				},
			},
			pluginName: "my-plugin",
			check: func(t *testing.T, path string) {
				t.Helper()
				doc := readJSON(t, path)
				hooks := doc["hooks"].(map[string]any)
				entries := hooks["PreToolUse"].([]any)
				assert.Len(t, entries, 1)
			},
		},
		{
			name: "removes all entries for plugin across multiple events",
			initial: map[string]any{
				"hooks": map[string]any{
					"PreToolUse": []any{
						map[string]any{"matcher": "Bash", "_plugin": "my-plugin"},
					},
					"PostToolUse": []any{
						map[string]any{"matcher": "Read", "_plugin": "my-plugin"},
					},
				},
			},
			pluginName: "my-plugin",
			check: func(t *testing.T, path string) {
				t.Helper()
				doc := readJSON(t, path)
				hooks := doc["hooks"].(map[string]any)
				pre := hooks["PreToolUse"].([]any)
				post := hooks["PostToolUse"].([]any)
				assert.Empty(t, pre)
				assert.Empty(t, post)
			},
		},
		{
			name:       "returns error when settings file is unreadable",
			pluginName: "my-plugin",
			setupPath: func(t *testing.T) string {
				t.Helper()
				p := filepath.Join(t.TempDir(), "settings.json")
				require.NoError(t, os.WriteFile(p, []byte(`{}`), 0o000))
				t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
				return p
			},
			wantErr: "read",
		},
		{
			name: "skips hook event whose value is not a slice",
			initial: map[string]any{
				"hooks": map[string]any{
					"PreToolUse": "not-a-slice",
				},
			},
			pluginName: "my-plugin",
			check: func(t *testing.T, path string) {
				t.Helper()
				doc := readJSON(t, path)
				hooks := doc["hooks"].(map[string]any)
				assert.Equal(t, "not-a-slice", hooks["PreToolUse"])
			},
		},
		{
			name:       "preserves non-map entries in hooks slice",
			pluginName: "my-plugin",
			setupPath: func(t *testing.T) string {
				t.Helper()
				p := filepath.Join(t.TempDir(), "settings.json")
				// Write a hooks entry array containing a plain string, not a map.
				raw := `{"hooks":{"PreToolUse":["plain-string"]}}`
				require.NoError(t, os.WriteFile(p, []byte(raw), 0o644))
				return p
			},
			check: func(t *testing.T, path string) {
				t.Helper()
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				var doc map[string]any
				require.NoError(t, json.Unmarshal(data, &doc))
				hooks := doc["hooks"].(map[string]any)
				entries := hooks["PreToolUse"].([]any)
				// The plain-string entry must be preserved unchanged.
				require.Len(t, entries, 1)
				assert.Equal(t, "plain-string", entries[0])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var settingsPath string
			if tt.setupPath != nil {
				settingsPath = tt.setupPath(t)
			} else {
				settingsPath = filepath.Join(t.TempDir(), "settings.json")
				if tt.initial != nil {
					writeJSON(t, settingsPath, tt.initial)
				}
			}

			err := configmerge.RemoveHooks(settingsPath, tt.pluginName)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			if tt.check != nil {
				tt.check(t, settingsPath)
			}
		})
	}
}
