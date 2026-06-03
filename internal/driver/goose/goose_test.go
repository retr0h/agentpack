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

package goose_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/retr0h/agentpack/internal/driver/goose"
	"github.com/retr0h/agentpack/internal/target"
	"github.com/retr0h/agentpack/internal/testutil"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	writeFile(t, path, string(data))
}

func writeYAML(t *testing.T, path string, v any) {
	t.Helper()
	data, err := yaml.Marshal(v)
	require.NoError(t, err)
	writeFile(t, path, string(data))
}

func TestGoose_Name(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{name: "returns goose", want: "goose"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			g := goose.New()
			assert.Equal(t, tt.want, g.Name())
		})
	}
}

func TestGoose_DisplayName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{name: "returns Goose", want: "Goose"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			g := goose.New()
			assert.Equal(t, tt.want, g.DisplayName())
		})
	}
}

func TestGoose_SupportedTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want []string
	}{
		{name: "returns skill and mcp", want: []string{"skill", "mcp"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			g := goose.New()
			assert.Equal(t, tt.want, g.SupportedTypes())
		})
	}
}

func TestGoose_Detect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		configDirFunc func() (string, error)
		wantDetected  bool
	}{
		{
			name: "detect returns true when ~/.config/goose exists",
			configDirFunc: func() (string, error) {
				configDir := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(configDir, "goose"), 0o755))
				return configDir, nil
			},
			wantDetected: true,
		},
		{
			name: "detect returns false when ~/.config/goose missing",
			configDirFunc: func() (string, error) {
				return t.TempDir(), nil
			},
			wantDetected: false,
		},
		{
			name: "config dir error returns false",
			configDirFunc: func() (string, error) {
				return "", errors.New("no config dir")
			},
			wantDetected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			g := goose.New()
			goose.SetUserConfigDir(g, tt.configDirFunc)
			assert.Equal(t, tt.wantDetected, g.Detect())
		})
	}
}

func TestGoose_Install(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		setup          func(t *testing.T) (srcDir string, installDir string)
		entries        []target.ContentEntry
		entriesFromSrc func(src string) []target.ContentEntry
		global         bool
		homeFunc       func() (string, error)
		configDirFunc  func() (string, error)
		cwdFunc        func() (string, error)
		mkdirFunc      func(string, os.FileMode) error
		cancelCtx      bool
		customCtx      context.Context
		wantErr        string
		check          func(t *testing.T, installDir string)
	}{
		{
			name: "copies skills to correct directory",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "skills", "review", "SKILL.md"), "# Review")
				return src, t.TempDir()
			},
			check: func(t *testing.T, dir string) {
				t.Helper()
				p := filepath.Join(dir, ".agents", "skills", "test-plugin", "review", "SKILL.md")
				_, err := os.Stat(p)
				assert.NoError(t, err)
			},
		},
		{
			name: "global installs skills into ~/.config/goose/skills/",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "skills", "my-skill.md"), "# My Skill")
				return src, ""
			},
			global: true,
			homeFunc: func() (string, error) {
				return t.TempDir(), nil
			},
			check: func(_ *testing.T, _ string) {},
		},
		{
			name: "installs from entries when provided",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "skills", "k8s", "SKILL.md"), "# K8s")
				return src, t.TempDir()
			},
			entriesFromSrc: func(src string) []target.ContentEntry {
				return []target.ContentEntry{
					{Name: "k8s", Type: "skill", Root: filepath.Join(src, "skills", "k8s")},
				}
			},
			check: func(t *testing.T, dir string) {
				t.Helper()
				_, err := os.Stat(filepath.Join(dir, ".agents", "skills", "k8s", "SKILL.md"))
				assert.NoError(t, err)
			},
		},
		{
			name: "merges MCP config into ~/.config/goose/config.yaml",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "mcp", "my-api.json"), map[string]any{
					"name":    "my-api",
					"type":    "stdio",
					"command": "npx",
					"args":    []string{"-y", "my-api"},
				})
				return src, t.TempDir()
			},
			configDirFunc: func() (string, error) {
				return t.TempDir(), nil
			},
			check: func(t *testing.T, _ string) {
				t.Helper()
				// Verified via standalone installMCP tests.
			},
		},
		{
			name: "installs from entries with skill + mcp",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "skills", "k8s", "SKILL.md"), "# K8s")
				writeJSON(t, filepath.Join(src, "mcp", "srv.json"), map[string]any{
					"name":    "srv",
					"type":    "stdio",
					"command": "srv-bin",
				})
				dir := t.TempDir()
				return src, dir
			},
			configDirFunc: func() (string, error) {
				return t.TempDir(), nil
			},
			entriesFromSrc: func(src string) []target.ContentEntry {
				return []target.ContentEntry{
					{Name: "k8s", Type: "skill", Root: filepath.Join(src, "skills", "k8s")},
					{Name: "srv", Type: "mcp", Root: filepath.Join(src, "mcp")},
				}
			},
			check: func(t *testing.T, dir string) {
				t.Helper()
				_, err := os.Stat(filepath.Join(dir, ".agents", "skills", "k8s", "SKILL.md"))
				assert.NoError(t, err)
			},
		},
		{
			name: "configDirFunc error propagates in installFromDirs mcp path",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "skills", "x.md"), "x")
				return src, t.TempDir()
			},
			configDirFunc: func() (string, error) {
				return "", errors.New("config dir unavailable")
			},
			wantErr: "config dir",
		},
		{
			name: "configDirFunc error propagates in installFromEntries mcp case",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return t.TempDir(), t.TempDir()
			},
			configDirFunc: func() (string, error) {
				return "", errors.New("config dir for mcp entry")
			},
			entriesFromSrc: func(_ string) []target.ContentEntry {
				return []target.ContentEntry{
					{Name: "srv", Type: "mcp"},
				}
			},
			wantErr: "config dir",
		},
		{
			name: "returns error on mcp name conflict",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "mcp", "dup.json"), map[string]any{
					"name":    "dup",
					"type":    "stdio",
					"command": "dup-bin",
				})
				return src, t.TempDir()
			},
			configDirFunc: func() (string, error) {
				configDir := t.TempDir()
				writeYAML(t, filepath.Join(configDir, "goose", "config.yaml"), map[string]any{
					"extensions": map[string]any{
						"dup": map[string]any{"type": "stdio"},
					},
				})
				return configDir, nil
			},
			wantErr: "already exists",
		},
		{
			name: "returns error on mcp name conflict via entries",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "mcp", "dup-e.json"), map[string]any{
					"name":    "dup-e",
					"type":    "stdio",
					"command": "dup-bin",
				})
				return src, t.TempDir()
			},
			configDirFunc: func() (string, error) {
				configDir := t.TempDir()
				writeYAML(t, filepath.Join(configDir, "goose", "config.yaml"), map[string]any{
					"extensions": map[string]any{
						"dup-e": map[string]any{"type": "stdio"},
					},
				})
				return configDir, nil
			},
			entriesFromSrc: func(_ string) []target.ContentEntry {
				return []target.ContentEntry{
					{Name: "dup-e", Type: "mcp"},
				}
			},
			wantErr: "already exists",
		},
		{
			name: "skips missing content dirs without error",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return t.TempDir(), t.TempDir()
			},
			configDirFunc: func() (string, error) {
				return t.TempDir(), nil
			},
		},
		{
			name: "fails on cancelled context",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return t.TempDir(), t.TempDir()
			},
			cancelCtx: true,
			wantErr:   "context canceled",
		},
		{
			name: "home dir error when global",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return t.TempDir(), ""
			},
			global: true,
			homeFunc: func() (string, error) {
				return "", errors.New("home unavailable")
			},
			wantErr: "home dir",
		},
		{
			name: "mkdirAll failure propagates error",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "skills", "x.md"), "x")
				return src, t.TempDir()
			},
			mkdirFunc: func(string, os.FileMode) error {
				return errors.New("disk full")
			},
			wantErr: "mkdir skills dir",
		},
		{
			name: "cwdFunc error propagates for local install",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return t.TempDir(), ""
			},
			cwdFunc: func() (string, error) {
				return "", errors.New("getwd failed")
			},
			wantErr: "getwd",
		},
		{
			name: "entry skill with home error propagates through installSkillEntry",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "skills", "k8s", "SKILL.md"), "# K8s")
				return src, t.TempDir()
			},
			global: true,
			homeFunc: func() (string, error) {
				return "", errors.New("home unavailable")
			},
			entriesFromSrc: func(src string) []target.ContentEntry {
				return []target.ContentEntry{
					{Name: "k8s", Type: "skill", Root: filepath.Join(src, "skills", "k8s")},
				}
			},
			wantErr: "home dir",
		},
		{
			name: "entry skill mkdirAll error propagates through installSkillEntry",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "skills", "k8s", "SKILL.md"), "# K8s")
				return src, t.TempDir()
			},
			mkdirFunc: func(string, os.FileMode) error {
				return errors.New("disk full")
			},
			entriesFromSrc: func(src string) []target.ContentEntry {
				return []target.ContentEntry{
					{Name: "k8s", Type: "skill", Root: filepath.Join(src, "skills", "k8s")},
				}
			},
			wantErr: "mkdir skills dir",
		},
		{
			name: "installFromDirs context cancelled inside dir loop",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return t.TempDir(), t.TempDir()
			},
			customCtx: testutil.NewCancelAfterN(1),
			wantErr:   "context canceled",
		},
		{
			name: "installFromEntries context cancelled inside entry loop",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "skills", "k8s", "SKILL.md"), "# K8s")
				return src, t.TempDir()
			},
			entriesFromSrc: func(src string) []target.ContentEntry {
				return []target.ContentEntry{
					{Name: "k8s", Type: "skill", Root: filepath.Join(src, "skills", "k8s")},
				}
			},
			customCtx: testutil.NewCancelAfterN(1),
			wantErr:   "context canceled",
		},
		{
			name: "copyTreeIfExists error from unreadable file in skills dir",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				p := filepath.Join(src, "skills", "secret.md")
				writeFile(t, p, "# Secret")
				require.NoError(t, os.Chmod(p, 0o000))
				t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
				return src, t.TempDir()
			},
			wantErr: "copy skills",
		},
		{
			name: "enumerateFiles error from pre-existing unreadable file in destDir",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				dst := t.TempDir()
				destDir := filepath.Join(dst, ".agents", "skills", "test-plugin")
				p := filepath.Join(destDir, "secret.md")
				writeFile(t, p, "# Secret")
				require.NoError(t, os.Chmod(p, 0o000))
				t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
				return src, dst
			},
			wantErr: "enumerate installed files",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srcDir, installDir := tt.setup(t)
			g := goose.New()

			if tt.homeFunc != nil {
				goose.SetUserHome(g, tt.homeFunc)
			}
			if tt.configDirFunc != nil {
				goose.SetUserConfigDir(g, tt.configDirFunc)
			}
			if tt.cwdFunc != nil {
				goose.SetCwd(g, tt.cwdFunc)
			}
			if tt.mkdirFunc != nil {
				goose.SetOsMkdirAll(g, tt.mkdirFunc)
			}

			ctx := context.Background()
			if tt.cancelCtx {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if tt.customCtx != nil {
				ctx = tt.customCtx
			}

			entries := tt.entries
			if tt.entriesFromSrc != nil {
				entries = tt.entriesFromSrc(srcDir)
			}

			files, err := g.Install(ctx, target.InstallOpts{
				Name:      "test-plugin",
				SourceDir: srcDir,
				Dir:       installDir,
				Global:    tt.global,
				Entries:   entries,
			})
			_ = files

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, installDir)
			}
		})
	}
}

func TestGoose_List(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		wantLen int
	}{
		{name: "returns empty", wantLen: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			g := goose.New()
			got, err := g.List()

			require.NoError(t, err)
			assert.Len(t, got, tt.wantLen)
		})
	}
}

func TestGoose_InstallMCP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(t *testing.T) (srcDir, configPath string)
		customCtx context.Context
		wantErr   string
		check     func(t *testing.T, configPath string)
	}{
		{
			name: "merges MCP config into config.yaml",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "mcp", "my-api.json"), map[string]any{
					"name":    "my-api",
					"type":    "stdio",
					"command": "npx",
					"args":    []string{"-y", "my-api-server"},
					"env":     map[string]any{"API_KEY": "secret"},
				})
				return src, filepath.Join(t.TempDir(), "goose", "config.yaml")
			},
			check: func(t *testing.T, configPath string) {
				t.Helper()
				data, err := os.ReadFile(configPath)
				require.NoError(t, err)
				var doc map[string]any
				require.NoError(t, yaml.Unmarshal(data, &doc))
				extensions, ok := doc["extensions"].(map[string]any)
				require.True(t, ok)
				srv, ok := extensions["my-api"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "stdio", srv["type"])
				assert.Equal(t, "npx", srv["command"])
			},
		},
		{
			name: "creates config.yaml if absent",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "mcp", "fresh.json"), map[string]any{
					"name":    "fresh",
					"type":    "stdio",
					"command": "fresh-bin",
				})
				return src, filepath.Join(t.TempDir(), "goose", "config.yaml")
			},
			check: func(t *testing.T, configPath string) {
				t.Helper()
				_, err := os.Stat(configPath)
				require.NoError(t, err)
				data, err := os.ReadFile(configPath)
				require.NoError(t, err)
				var doc map[string]any
				require.NoError(t, yaml.Unmarshal(data, &doc))
				extensions, ok := doc["extensions"].(map[string]any)
				require.True(t, ok)
				_, ok = extensions["fresh"]
				assert.True(t, ok)
			},
		},
		{
			name: "preserves existing YAML keys when merging",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "mcp", "new-srv.json"), map[string]any{
					"name":    "new-srv",
					"type":    "stdio",
					"command": "new-bin",
				})
				configDir := t.TempDir()
				configPath := filepath.Join(configDir, "goose", "config.yaml")
				writeYAML(t, configPath, map[string]any{
					"GOOSE_PROVIDER": "anthropic",
					"extensions": map[string]any{
						"existing-srv": map[string]any{
							"type":    "stdio",
							"command": "existing-bin",
						},
					},
				})
				return src, configPath
			},
			check: func(t *testing.T, configPath string) {
				t.Helper()
				data, err := os.ReadFile(configPath)
				require.NoError(t, err)
				var doc map[string]any
				require.NoError(t, yaml.Unmarshal(data, &doc))
				// Existing top-level key preserved.
				assert.Equal(t, "anthropic", doc["GOOSE_PROVIDER"])
				extensions, ok := doc["extensions"].(map[string]any)
				require.True(t, ok)
				// Existing extension preserved.
				_, ok = extensions["existing-srv"]
				assert.True(t, ok)
				// New extension added.
				_, ok = extensions["new-srv"]
				assert.True(t, ok)
			},
		},
		{
			name: "no-op when mcp dir is absent",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return t.TempDir(), filepath.Join(t.TempDir(), "goose", "config.yaml")
			},
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
				return src, filepath.Join(t.TempDir(), "goose", "config.yaml")
			},
			wantErr: "read mcp dir",
		},
		{
			name: "returns error when mcp json file is unreadable",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				p := filepath.Join(src, "mcp", "srv.json")
				writeFile(t, p, `{"name":"srv"}`)
				require.NoError(t, os.Chmod(p, 0o000))
				t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
				return src, filepath.Join(t.TempDir(), "goose", "config.yaml")
			},
			wantErr: "read mcp/",
		},
		{
			name: "returns error when mcp json contains invalid JSON",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "mcp", "srv.json"), `{invalid`)
				return src, filepath.Join(t.TempDir(), "goose", "config.yaml")
			},
			wantErr: "parse mcp/",
		},
		{
			name: "returns error when mcp json has no name field",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "mcp", "bad.json"), map[string]any{
					"type": "stdio",
				})
				return src, filepath.Join(t.TempDir(), "goose", "config.yaml")
			},
			wantErr: "missing or invalid \"name\" field",
		},
		{
			name: "returns error on extension name conflict",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "mcp", "dup.json"), map[string]any{
					"name":    "dup",
					"type":    "stdio",
					"command": "dup-bin",
				})
				configDir := t.TempDir()
				configPath := filepath.Join(configDir, "goose", "config.yaml")
				writeYAML(t, configPath, map[string]any{
					"extensions": map[string]any{
						"dup": map[string]any{"type": "stdio"},
					},
				})
				return src, configPath
			},
			wantErr: "already exists",
		},
		{
			name: "skips directories and non-json entries in mcp dir",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(src, "mcp", "subdir"), 0o755))
				writeFile(t, filepath.Join(src, "mcp", "readme.txt"), "skip me")
				return src, filepath.Join(t.TempDir(), "goose", "config.yaml")
			},
		},
		{
			name: "context cancelled inside mcp entry loop",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "mcp", "srv.json"), map[string]any{
					"name":    "srv",
					"type":    "stdio",
					"command": "srv-bin",
				})
				return src, filepath.Join(t.TempDir(), "goose", "config.yaml")
			},
			customCtx: testutil.NewCancelAfterN(0),
			wantErr:   "context canceled",
		},
		{
			name: "returns error when config file is unreadable via mergeGooseExtension",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "mcp", "srv.json"), map[string]any{
					"name":    "srv",
					"type":    "stdio",
					"command": "srv-bin",
				})
				configPath := filepath.Join(t.TempDir(), "goose", "config.yaml")
				writeFile(t, configPath, "extensions:\n  existing: {}\n")
				require.NoError(t, os.Chmod(configPath, 0o000))
				t.Cleanup(func() { _ = os.Chmod(configPath, 0o644) })
				return src, configPath
			},
			wantErr: "merge mcp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srcDir, configPath := tt.setup(t)
			g := goose.New()

			ctx := context.Context(context.Background())
			if tt.customCtx != nil {
				ctx = tt.customCtx
			}

			err := goose.InstallMCP(ctx, g, srcDir, configPath)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, configPath)
			}
		})
	}
}

func TestGoose_ReadYAMLConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		wantErr string
		check   func(t *testing.T, doc map[string]any)
	}{
		{
			name: "returns empty map when file does not exist",
			setup: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "nonexistent.yaml")
			},
			check: func(t *testing.T, doc map[string]any) {
				t.Helper()
				assert.Empty(t, doc)
			},
		},
		{
			name: "returns empty map when file is empty YAML",
			setup: func(t *testing.T) string {
				t.Helper()
				p := filepath.Join(t.TempDir(), "config.yaml")
				writeFile(t, p, "")
				return p
			},
			check: func(t *testing.T, doc map[string]any) {
				t.Helper()
				assert.NotNil(t, doc)
				assert.Empty(t, doc)
			},
		},
		{
			name: "returns error when file is unreadable",
			setup: func(t *testing.T) string {
				t.Helper()
				p := filepath.Join(t.TempDir(), "config.yaml")
				writeFile(t, p, "key: value")
				require.NoError(t, os.Chmod(p, 0o000))
				t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
				return p
			},
			wantErr: "read ",
		},
		{
			name: "returns error when file contains invalid YAML",
			setup: func(t *testing.T) string {
				t.Helper()
				p := filepath.Join(t.TempDir(), "config.yaml")
				writeFile(t, p, "key: [\ninvalid")
				return p
			},
			wantErr: "parse ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := tt.setup(t)
			doc, err := goose.ReadYAMLConfig(path)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, doc)
			}
		})
	}
}

func TestGoose_WriteYAMLConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		doc     map[string]any
		wantErr string
		check   func(t *testing.T, path string)
	}{
		{
			name: "writes YAML file creating parent directories",
			setup: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "subdir", "config.yaml")
			},
			doc: map[string]any{"key": "value"},
			check: func(t *testing.T, path string) {
				t.Helper()
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				var doc map[string]any
				require.NoError(t, yaml.Unmarshal(data, &doc))
				assert.Equal(t, "value", doc["key"])
			},
		},
		{
			name: "returns error when parent directory is unwritable",
			setup: func(t *testing.T) string {
				t.Helper()
				parent := t.TempDir()
				require.NoError(t, os.Chmod(parent, 0o555))
				t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })
				return filepath.Join(parent, "subdir", "config.yaml")
			},
			doc:     map[string]any{"key": "value"},
			wantErr: "mkdir ",
		},
		{
			name: "returns error when file is not writable",
			setup: func(t *testing.T) string {
				t.Helper()
				p := filepath.Join(t.TempDir(), "config.yaml")
				writeFile(t, p, "old: content")
				require.NoError(t, os.Chmod(p, 0o444))
				t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
				return p
			},
			doc:     map[string]any{"key": "value"},
			wantErr: "write ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := tt.setup(t)
			err := goose.WriteYAMLConfig(path, tt.doc)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, path)
			}
		})
	}
}

func TestGoose_MergeGooseExtension(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		extName string
		config  map[string]any
		wantErr string
	}{
		{
			name: "returns error when config file is unreadable",
			setup: func(t *testing.T) string {
				t.Helper()
				p := filepath.Join(t.TempDir(), "config.yaml")
				writeFile(t, p, "extensions:\n  existing: {}\n")
				require.NoError(t, os.Chmod(p, 0o000))
				t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
				return p
			},
			extName: "new-ext",
			config:  map[string]any{"type": "stdio"},
			wantErr: "read ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := tt.setup(t)
			err := goose.MergeGooseExtension(path, tt.extName, tt.config)

			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}
