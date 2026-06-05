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

package driver_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/internal/driver"
	"github.com/retr0h/agentpack/internal/target"
)

func TestValidateMCPName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name:  "accepts a plain identifier",
			input: "my-server",
		},
		{
			name:  "accepts dots underscores and digits",
			input: "my_server.v2-3",
		},
		{
			name:    "rejects an empty name",
			input:   "",
			wantErr: "mcp server name is empty",
		},
		{
			name:    "rejects path traversal",
			input:   "../../evil",
			wantErr: "invalid mcp server name",
		},
		{
			name:    "rejects a path separator",
			input:   "a/b",
			wantErr: "invalid mcp server name",
		},
		{
			name:    "rejects whitespace",
			input:   "a b",
			wantErr: "invalid mcp server name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := driver.ValidateMCPName(tt.input)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			assert.NoError(t, err)
		})
	}
}

func TestInstallMCP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(t *testing.T) (srcDir, mcpPath string)
		cancelCtx bool
		wantErr   string
		check     func(t *testing.T, mcpPath string)
	}{
		{
			name: "no-op when mcp dir is absent",
			setup: func(t *testing.T) (string, string) {
				t.Helper()

				return t.TempDir(), filepath.Join(t.TempDir(), "mcp.json")
			},
		},
		{
			name: "returns error when context is cancelled in the entry loop",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				p := filepath.Join(src, "mcp", "srv.json")
				require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
				require.NoError(t, os.WriteFile(p, []byte(`{"name":"srv"}`), 0o644))

				return src, filepath.Join(t.TempDir(), "mcp.json")
			},
			cancelCtx: true,
			wantErr:   "context canceled",
		},
		{
			name: "returns error when mcp dir is unreadable",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				d := filepath.Join(src, "mcp")
				require.NoError(t, os.MkdirAll(d, 0o755))
				require.NoError(t, os.Chmod(d, 0o000))
				t.Cleanup(func() { _ = os.Chmod(d, 0o755) })

				return src, filepath.Join(t.TempDir(), "mcp.json")
			},
			wantErr: "read mcp dir",
		},
		{
			name: "returns error when mcp json file is unreadable",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				p := filepath.Join(src, "mcp", "srv.json")
				require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
				require.NoError(t, os.WriteFile(p, []byte(`{"name":"srv"}`), 0o000))
				t.Cleanup(func() { _ = os.Chmod(p, 0o644) })

				return src, filepath.Join(t.TempDir(), "mcp.json")
			},
			wantErr: "read mcp/",
		},
		{
			name: "returns error when mcp json contains invalid JSON",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				mcpDir := filepath.Join(src, "mcp")
				require.NoError(t, os.MkdirAll(mcpDir, 0o755))
				require.NoError(
					t,
					os.WriteFile(filepath.Join(mcpDir, "srv.json"), []byte(`{invalid`), 0o644),
				)

				return src, filepath.Join(t.TempDir(), "mcp.json")
			},
			wantErr: "parse mcp/",
		},
		{
			name: "returns error when name field is missing",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				mcpDir := filepath.Join(src, "mcp")
				require.NoError(t, os.MkdirAll(mcpDir, 0o755))
				require.NoError(
					t,
					os.WriteFile(
						filepath.Join(mcpDir, "srv.json"),
						[]byte(`{"type":"stdio"}`),
						0o644,
					),
				)

				return src, filepath.Join(t.TempDir(), "mcp.json")
			},
			wantErr: "mcp server name is empty",
		},
		{
			name: "returns error when name has unsafe characters",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				mcpDir := filepath.Join(src, "mcp")
				require.NoError(t, os.MkdirAll(mcpDir, 0o755))
				require.NoError(
					t,
					os.WriteFile(
						filepath.Join(mcpDir, "srv.json"),
						[]byte(`{"name":"../../evil","type":"stdio"}`),
						0o644,
					),
				)

				return src, filepath.Join(t.TempDir(), "mcp.json")
			},
			wantErr: "invalid mcp server name",
		},
		{
			name: "skips directories and non-json entries in mcp dir",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(src, "mcp", "subdir"), 0o755))
				require.NoError(
					t,
					os.WriteFile(filepath.Join(src, "mcp", "readme.txt"), []byte("skip me"), 0o644),
				)

				return src, filepath.Join(t.TempDir(), "mcp.json")
			},
		},
		{
			name: "returns error when mcp server already exists in settings",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				mcpDir := filepath.Join(src, "mcp")
				require.NoError(t, os.MkdirAll(mcpDir, 0o755))
				data, err := json.Marshal(map[string]any{
					"name": "duplicate-api",
					"type": "remote",
					"url":  "https://mcp.example.com/v1",
				})
				require.NoError(t, err)
				require.NoError(
					t,
					os.WriteFile(filepath.Join(mcpDir, "duplicate-api.json"), data, 0o644),
				)
				destDir := t.TempDir()
				mcpPath := filepath.Join(destDir, "mcp.json")
				existing, err := json.Marshal(map[string]any{
					"mcpServers": map[string]any{
						"duplicate-api": map[string]any{"type": "stdio"},
					},
				})
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(mcpPath, existing, 0o644))

				return src, mcpPath
			},
			wantErr: "merge mcp",
		},
		{
			name: "merges mcp server into target file",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				mcpDir := filepath.Join(src, "mcp")
				require.NoError(t, os.MkdirAll(mcpDir, 0o755))
				data, err := json.Marshal(map[string]any{
					"name": "my-api",
					"type": "remote",
					"url":  "https://mcp.example.com/v1",
				})
				require.NoError(t, err)
				require.NoError(
					t,
					os.WriteFile(filepath.Join(mcpDir, "my-api.json"), data, 0o644),
				)

				return src, filepath.Join(t.TempDir(), "mcp.json")
			},
			check: func(t *testing.T, mcpPath string) {
				t.Helper()
				data, err := os.ReadFile(mcpPath)
				require.NoError(t, err)
				var doc map[string]any
				require.NoError(t, json.Unmarshal(data, &doc))
				servers, ok := doc["mcpServers"].(map[string]any)
				require.True(t, ok)
				srv, ok := servers["my-api"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "remote", srv["type"])
				assert.Equal(t, "https://mcp.example.com/v1", srv["url"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srcDir, mcpPath := tt.setup(t)

			ctx := context.Background()
			if tt.cancelCtx {
				cancelCtx, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelCtx
			}

			err := driver.InstallMCP(ctx, srcDir, mcpPath)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, mcpPath)
			}
		})
	}
}

func TestInstallHooksJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (srcDir, hooksPath string)
		wantErr string
		check   func(t *testing.T, hooksPath string)
	}{
		{
			name: "no-op when hooks dir is absent",
			setup: func(t *testing.T) (string, string) {
				t.Helper()

				return t.TempDir(), filepath.Join(t.TempDir(), "hooks.json")
			},
		},
		{
			name: "returns error when hooks.json is unreadable",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				p := filepath.Join(src, "hooks", "hooks.json")
				require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
				require.NoError(t, os.WriteFile(p, []byte(`{}`), 0o000))
				t.Cleanup(func() { _ = os.Chmod(p, 0o644) })

				return src, filepath.Join(t.TempDir(), "hooks.json")
			},
			wantErr: "read hooks/hooks.json",
		},
		{
			name: "returns error when hooks.json contains invalid JSON",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				hooksDir := filepath.Join(src, "hooks")
				require.NoError(t, os.MkdirAll(hooksDir, 0o755))
				require.NoError(
					t,
					os.WriteFile(filepath.Join(hooksDir, "hooks.json"), []byte(`{invalid`), 0o644),
				)

				return src, filepath.Join(t.TempDir(), "hooks.json")
			},
			wantErr: "parse hooks/hooks.json",
		},
		{
			name: "merges hooks into target file",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				hooksDir := filepath.Join(src, "hooks")
				require.NoError(t, os.MkdirAll(hooksDir, 0o755))
				data, err := json.Marshal(map[string]any{
					"PreToolUse": []any{
						map[string]any{
							"command":    "echo lint",
							"showOutput": true,
						},
					},
				})
				require.NoError(t, err)
				require.NoError(
					t,
					os.WriteFile(filepath.Join(hooksDir, "hooks.json"), data, 0o644),
				)

				return src, filepath.Join(t.TempDir(), "hooks.json")
			},
			check: func(t *testing.T, hooksPath string) {
				t.Helper()
				data, err := os.ReadFile(hooksPath)
				require.NoError(t, err)
				var doc map[string]any
				require.NoError(t, json.Unmarshal(data, &doc))
				hooks, ok := doc["hooks"].(map[string]any)
				require.True(t, ok)
				_, ok = hooks["PreToolUse"]
				assert.True(t, ok)
			},
		},
		{
			name: "returns error when existing hooksPath is unreadable",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				hooksDir := filepath.Join(src, "hooks")
				require.NoError(t, os.MkdirAll(hooksDir, 0o755))
				data, err := json.Marshal(map[string]any{
					"PreToolUse": []any{
						map[string]any{"command": "echo lint", "showOutput": true},
					},
				})
				require.NoError(t, err)
				require.NoError(
					t,
					os.WriteFile(filepath.Join(hooksDir, "hooks.json"), data, 0o644),
				)
				destDir := t.TempDir()
				hooksPath := filepath.Join(destDir, "hooks.json")
				require.NoError(t, os.WriteFile(hooksPath, []byte(`{}`), 0o000))
				t.Cleanup(func() { _ = os.Chmod(hooksPath, 0o644) })

				return src, hooksPath
			},
			wantErr: "merge hooks",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srcDir, hooksPath := tt.setup(t)
			err := driver.InstallHooksJSON(context.Background(), srcDir, hooksPath, "test-plugin")

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, hooksPath)
			}
		})
	}
}

func TestInstallSkillEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(t *testing.T) (target.ContentEntry, string, string, func(string, os.FileMode) error)
		wantFiles int
		wantErr   string
	}{
		{
			name: "copies entry tree and enumerates files",
			setup: func(t *testing.T) (target.ContentEntry, string, string, func(string, os.FileMode) error) {
				t.Helper()
				baseDir := t.TempDir()
				skillsDir := filepath.Join(baseDir, "skills")
				entryRoot := t.TempDir()
				require.NoError(
					t,
					os.WriteFile(filepath.Join(entryRoot, "rule.md"), []byte("# Rule"), 0o644),
				)

				return target.ContentEntry{Name: "demo", Type: "skill", Root: entryRoot},
					skillsDir, baseDir, os.MkdirAll
			},
			wantFiles: 1,
		},
		{
			name: "mkdirAll failure propagates error",
			setup: func(t *testing.T) (target.ContentEntry, string, string, func(string, os.FileMode) error) {
				t.Helper()

				return target.ContentEntry{Name: "demo", Type: "skill", Root: t.TempDir()},
					"/skills", "/base",
					func(_ string, _ os.FileMode) error { return errors.New("disk full") }
			},
			wantErr: "mkdir skills dir",
		},
		{
			name: "empty entry root produces no files",
			setup: func(t *testing.T) (target.ContentEntry, string, string, func(string, os.FileMode) error) {
				t.Helper()
				baseDir := t.TempDir()
				skillsDir := filepath.Join(baseDir, "skills")
				entryRoot := t.TempDir()

				return target.ContentEntry{Name: "empty", Type: "skill", Root: entryRoot},
					skillsDir, baseDir, os.MkdirAll
			},
			wantFiles: 0,
		},
		{
			name: "copy tree error propagates when source file is unreadable",
			setup: func(t *testing.T) (target.ContentEntry, string, string, func(string, os.FileMode) error) {
				t.Helper()
				if os.Getuid() == 0 {
					t.Skip("root bypasses file permissions")
				}
				baseDir := t.TempDir()
				skillsDir := filepath.Join(baseDir, "skills")
				entryRoot := t.TempDir()
				p := filepath.Join(entryRoot, "secret.md")
				require.NoError(t, os.WriteFile(p, []byte("secret"), 0o000))
				t.Cleanup(func() { _ = os.Chmod(p, 0o644) })

				return target.ContentEntry{Name: "locked", Type: "skill", Root: entryRoot},
					skillsDir, baseDir, os.MkdirAll
			},
			wantErr: "copy skills",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			entry, skillsDir, baseDir, mkdirAll := tt.setup(t)
			files, err := driver.InstallSkillEntry(
				context.Background(),
				entry,
				skillsDir,
				baseDir,
				mkdirAll,
			)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Len(t, files, tt.wantFiles)

			for _, f := range files {
				assert.NotEmpty(t, f.Path)
				assert.Len(t, f.SHA256, 64)
			}
		})
	}
}
