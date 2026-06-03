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

package cline_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/internal/driver/cline"
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

func TestCline_Name(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{name: "returns cline", want: "cline"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := cline.New()
			assert.Equal(t, tt.want, c.Name())
		})
	}
}

func TestCline_DisplayName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{name: "returns Cline", want: "Cline"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := cline.New()
			assert.Equal(t, tt.want, c.DisplayName())
		})
	}
}

func TestCline_SupportedTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want []string
	}{
		{name: "returns skill hook and mcp", want: []string{"skill", "hook", "mcp"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := cline.New()
			assert.Equal(t, tt.want, c.SupportedTypes())
		})
	}
}

func TestCline_Detect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		homeFunc     func() (string, error)
		wantDetected bool
	}{
		{
			name: "detect returns true when .cline exists",
			homeFunc: func() (string, error) {
				home := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(home, ".cline"), 0o755))
				return home, nil
			},
			wantDetected: true,
		},
		{
			name: "detect returns false when .cline missing",
			homeFunc: func() (string, error) {
				return t.TempDir(), nil
			},
			wantDetected: false,
		},
		{
			name: "home error returns false",
			homeFunc: func() (string, error) {
				return "", errors.New("no home")
			},
			wantDetected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := cline.New()
			cline.SetUserHome(c, tt.homeFunc)
			assert.Equal(t, tt.wantDetected, c.Detect())
		})
	}
}

func TestCline_Install(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		setup          func(t *testing.T) (srcDir string, installDir string)
		entries        []target.ContentEntry
		entriesFromSrc func(src string) []target.ContentEntry
		global         bool
		homeFunc       func() (string, error)
		cwdFunc        func() (string, error)
		mkdirFunc      func(string, os.FileMode) error
		cancelCtx      bool
		customCtx      context.Context
		wantErr        string
		check          func(t *testing.T, installDir string, homeDir string)
	}{
		{
			name: "copies skills to .agents/skills/ locally",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "skills", "review", "SKILL.md"), "# Review")
				return src, t.TempDir()
			},
			check: func(t *testing.T, dir string, _ string) {
				t.Helper()
				p := filepath.Join(dir, ".agents", "skills", "test-plugin", "review", "SKILL.md")
				_, err := os.Stat(p)
				assert.NoError(t, err)
			},
		},
		{
			name: "copies skills to ~/.agents/skills/ globally",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "skills", "my-skill.md"), "# My Skill")
				return src, ""
			},
			global: true,
			homeFunc: func() func() (string, error) {
				home := t.TempDir()
				return func() (string, error) { return home, nil }
			}(),
			check: func(t *testing.T, _ string, home string) {
				t.Helper()
				p := filepath.Join(
					home,
					".agents",
					"skills",
					"test-plugin",
					"my-skill.md",
				)
				_, err := os.Stat(p)
				assert.NoError(t, err)
			},
		},
		{
			name: "merges MCP config into ~/.cline/data/settings/cline_mcp_settings.json",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "mcp", "my-api.json"), map[string]any{
					"name": "my-api",
					"type": "remote",
					"url":  "https://mcp.example.com/v1",
				})
				return src, t.TempDir()
			},
			homeFunc: func() func() (string, error) {
				home := t.TempDir()
				return func() (string, error) { return home, nil }
			}(),
			check: func(t *testing.T, _ string, home string) {
				t.Helper()
				data, err := os.ReadFile(
					filepath.Join(home, ".cline", "data", "settings", "cline_mcp_settings.json"),
				)
				require.NoError(t, err)
				var doc map[string]any
				require.NoError(t, json.Unmarshal(data, &doc))
				servers, ok := doc["mcpServers"].(map[string]any)
				require.True(t, ok)
				srv, ok := servers["my-api"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "remote", srv["type"])
			},
		},
		{
			name: "copies hooks to .clinerules/hooks/ and makes executable",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "hooks", "pre-tool-use.sh"), "#!/bin/bash\necho ok")
				return src, t.TempDir()
			},
			check: func(t *testing.T, dir string, _ string) {
				t.Helper()
				p := filepath.Join(dir, ".clinerules", "hooks", "pre-tool-use.sh")
				info, err := os.Stat(p)
				require.NoError(t, err)
				assert.NotZero(t, info.Mode()&0o111)
			},
		},
		{
			name: "installs from entries with skill + mcp + hook",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "skills", "k8s", "SKILL.md"), "# K8s")
				writeJSON(t, filepath.Join(src, "mcp", "srv.json"), map[string]any{
					"name": "srv",
					"type": "stdio",
				})
				writeFile(t, filepath.Join(src, "hooks", "post-tool-use.sh"), "#!/bin/bash\nexit 0")
				return src, t.TempDir()
			},
			homeFunc: func() func() (string, error) {
				home := t.TempDir()
				return func() (string, error) { return home, nil }
			}(),
			entriesFromSrc: func(src string) []target.ContentEntry {
				return []target.ContentEntry{
					{Name: "k8s", Type: "skill", Root: filepath.Join(src, "skills", "k8s")},
					{Name: "srv", Type: "mcp", Root: filepath.Join(src, "mcp")},
					{Name: "hooks", Type: "hook", Root: filepath.Join(src, "hooks")},
				}
			},
			check: func(t *testing.T, dir string, home string) {
				t.Helper()
				// Skill must be installed to .agents/skills/k8s/.
				_, err := os.Stat(filepath.Join(dir, ".agents", "skills", "k8s", "SKILL.md"))
				assert.NoError(t, err)
				// MCP must be merged into ~/.cline/data/settings/cline_mcp_settings.json.
				data, readErr := os.ReadFile(
					filepath.Join(home, ".cline", "data", "settings", "cline_mcp_settings.json"),
				)
				require.NoError(t, readErr)
				var doc map[string]any
				require.NoError(t, json.Unmarshal(data, &doc))
				servers, ok := doc["mcpServers"].(map[string]any)
				require.True(t, ok)
				_, ok = servers["srv"]
				assert.True(t, ok)
				// Hook must be copied to .clinerules/hooks/.
				hookPath := filepath.Join(dir, ".clinerules", "hooks", "post-tool-use.sh")
				info, hookErr := os.Stat(hookPath)
				require.NoError(t, hookErr)
				assert.NotZero(t, info.Mode()&0o111)
			},
		},
		{
			name: "skips missing content dirs without error",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return t.TempDir(), t.TempDir()
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
			name: "returns error when mcp json has no name field",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "mcp", "bad.json"), map[string]any{
					"type": "remote",
				})
				return src, t.TempDir()
			},
			homeFunc: func() func() (string, error) {
				home := t.TempDir()
				return func() (string, error) { return home, nil }
			}(),
			wantErr: `missing or invalid "name" field`,
		},
		{
			name: "returns error on mcp name conflict",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "mcp", "dup.json"), map[string]any{
					"name": "dup-srv",
					"type": "stdio",
				})
				return src, t.TempDir()
			},
			homeFunc: func() func() (string, error) {
				home := t.TempDir()
				writeJSON(
					t,
					filepath.Join(home, ".cline", "data", "settings", "cline_mcp_settings.json"),
					map[string]any{
						"mcpServers": map[string]any{
							"dup-srv": map[string]any{"type": "stdio"},
						},
					},
				)
				return func() (string, error) { return home, nil }
			}(),
			wantErr: "already exists",
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
			name: "mcpConfigPath home error propagates in installFromDirs",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "skills", "x.md"), "x")
				return src, t.TempDir()
			},
			homeFunc: func() (string, error) {
				return "", errors.New("no home for mcp")
			},
			wantErr: "home dir",
		},
		{
			name: "hooksDir cwdFunc error propagates in installFromDirs",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "skills", "x.md"), "x")
				return src, ""
			},
			cwdFunc: func() func() (string, error) {
				calls := 0
				return func() (string, error) {
					calls++
					if calls == 1 {
						return t.TempDir(), nil
					}
					return "", errors.New("getwd second call failed")
				}
			}(),
			homeFunc: func() (string, error) {
				return t.TempDir(), nil
			},
			wantErr: "getwd",
		},
		{
			name: "mcpConfigPath home error propagates in installFromEntries mcp case",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return t.TempDir(), t.TempDir()
			},
			homeFunc: func() (string, error) {
				return "", errors.New("no home for mcp entry")
			},
			entries: []target.ContentEntry{
				{Name: "srv", Type: "mcp"},
			},
			wantErr: "home dir",
		},
		{
			name: "hooksDir cwdFunc error propagates in installFromEntries hook case",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return t.TempDir(), ""
			},
			cwdFunc: func() (string, error) {
				return "", errors.New("getwd failed for hooks entry")
			},
			entries: []target.ContentEntry{
				{Name: "hooks", Type: "hook"},
			},
			wantErr: "getwd",
		},
		{
			name: "installSkillEntry mkdirAll failure propagates error via entries",
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
			mkdirFunc: func(string, os.FileMode) error {
				return errors.New("disk full on skill entry")
			},
			wantErr: "mkdir skills dir",
		},
		{
			name: "installSkillEntry resolveDirs error propagates via skill entry",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "skills", "k8s", "SKILL.md"), "# K8s")
				return src, ""
			},
			global: true,
			homeFunc: func() (string, error) {
				return "", errors.New("no home for skill entry resolveDirs")
			},
			entriesFromSrc: func(src string) []target.ContentEntry {
				return []target.ContentEntry{
					{Name: "k8s", Type: "skill", Root: filepath.Join(src, "skills", "k8s")},
				}
			},
			wantErr: "home dir",
		},
		{
			name: "installMCP error propagates in installFromEntries mcp case",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "mcp", "dup-e.json"), map[string]any{
					"name": "dup-e",
					"type": "stdio",
				})
				return src, t.TempDir()
			},
			homeFunc: func() func() (string, error) {
				home := t.TempDir()
				writeJSON(
					t,
					filepath.Join(home, ".cline", "data", "settings", "cline_mcp_settings.json"),
					map[string]any{
						"mcpServers": map[string]any{
							"dup-e": map[string]any{"type": "stdio"},
						},
					},
				)
				return func() (string, error) { return home, nil }
			}(),
			entries: []target.ContentEntry{
				{Name: "dup-e", Type: "mcp"},
			},
			wantErr: "already exists",
		},
		{
			name: "installHooks mkdirAll failure propagates in installFromEntries hook case",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "hooks", "pre.sh"), "#!/bin/bash")
				return src, t.TempDir()
			},
			entriesFromSrc: func(src string) []target.ContentEntry {
				return []target.ContentEntry{
					{Name: "hooks", Type: "hook", Root: filepath.Join(src, "hooks")},
				}
			},
			mkdirFunc: func(string, os.FileMode) error {
				return errors.New("disk full on hooks dir")
			},
			wantErr: "mkdir hooks dir",
		},
		{
			name: "copyTreeIfExists walkErr propagates when source subdir is unreadable",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				subdir := filepath.Join(src, "subdir")
				require.NoError(t, os.MkdirAll(subdir, 0o755))
				require.NoError(t, os.Chmod(subdir, 0o000))
				t.Cleanup(func() { _ = os.Chmod(subdir, 0o755) })
				return src, t.TempDir()
			},
			entriesFromSrc: func(src string) []target.ContentEntry {
				return []target.ContentEntry{
					{Name: "unreadable-skill", Type: "skill", Root: src},
				}
			},
			wantErr: "copy skills",
		},
		{
			name: "copyFile write error propagates when skills destDir is unwritable",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "skills", "SKILL.md"), "# Skill")
				return src, t.TempDir()
			},
			mkdirFunc: func(path string, mode os.FileMode) error {
				if err := os.MkdirAll(path, mode); err != nil {
					return err
				}
				return os.Chmod(path, 0o555)
			},
			wantErr: "copy skills",
		},
		{
			name: "enumerateFiles walkErr propagates when destDir has unreadable subdir",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				installDir := t.TempDir()
				destDir := filepath.Join(installDir, ".agents", "skills", "test-plugin")
				unreadable := filepath.Join(destDir, "unreadable")
				require.NoError(t, os.MkdirAll(unreadable, 0o755))
				require.NoError(t, os.Chmod(unreadable, 0o000))
				t.Cleanup(func() { _ = os.Chmod(unreadable, 0o755) })
				return src, installDir
			},
			homeFunc: func() func() (string, error) {
				home := t.TempDir()
				return func() (string, error) { return home, nil }
			}(),
			wantErr: "enumerate installed files",
		},
		{
			name: "installFromDirs context cancelled after top-level check",
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
			name: "installHooks error propagates in installFromDirs when hooks dir unreadable",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				hooksDir := filepath.Join(src, "hooks")
				require.NoError(t, os.MkdirAll(hooksDir, 0o755))
				require.NoError(t, os.Chmod(hooksDir, 0o000))
				t.Cleanup(func() { _ = os.Chmod(hooksDir, 0o755) })
				return src, t.TempDir()
			},
			homeFunc: func() func() (string, error) {
				home := t.TempDir()
				return func() (string, error) { return home, nil }
			}(),
			wantErr: "read hooks dir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srcDir, installDir := tt.setup(t)
			c := cline.New()

			// Track the home directory for check functions.
			var homeDir string
			if tt.homeFunc != nil {
				cline.SetUserHome(c, func() (string, error) {
					h, err := tt.homeFunc()
					homeDir = h
					return h, err
				})
			}
			if tt.cwdFunc != nil {
				cline.SetCwd(c, tt.cwdFunc)
			}
			if tt.mkdirFunc != nil {
				cline.SetOsMkdirAll(c, tt.mkdirFunc)
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

			files, err := c.Install(ctx, target.InstallOpts{
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
				tt.check(t, installDir, homeDir)
			}
		})
	}
}

func TestCline_List(t *testing.T) {
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

			c := cline.New()
			got, err := c.List()

			require.NoError(t, err)
			assert.Len(t, got, tt.wantLen)
		})
	}
}

func TestCline_InstallHooks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (srcDir, hooksDir string)
		wantErr string
		check   func(t *testing.T, hooksDir string)
	}{
		{
			name: "no-op when hooks dir is absent",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return filepath.Join(t.TempDir(), "nonexistent"), t.TempDir()
			},
		},
		{
			name: "copies hook files and makes executable",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "pre-tool-use.sh"), "#!/bin/bash\necho pre")
				writeFile(t, filepath.Join(src, "post-tool-use.sh"), "#!/bin/bash\necho post")
				return src, filepath.Join(t.TempDir(), "hooks")
			},
			check: func(t *testing.T, hooksDir string) {
				t.Helper()
				for _, name := range []string{"pre-tool-use.sh", "post-tool-use.sh"} {
					info, err := os.Stat(filepath.Join(hooksDir, name))
					require.NoError(t, err)
					assert.NotZero(t, info.Mode()&0o111)
				}
			},
		},
		{
			name: "skips subdirectories in hooks source",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(src, "subdir"), 0o755))
				writeFile(t, filepath.Join(src, "hook.sh"), "#!/bin/bash")
				return src, filepath.Join(t.TempDir(), "hooks")
			},
			check: func(t *testing.T, hooksDir string) {
				t.Helper()
				_, err := os.Stat(filepath.Join(hooksDir, "hook.sh"))
				assert.NoError(t, err)
				_, err = os.Stat(filepath.Join(hooksDir, "subdir"))
				assert.True(t, os.IsNotExist(err))
			},
		},
		{
			name: "returns error when hooks source dir is unreadable",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "hook.sh"), "#!/bin/bash")
				require.NoError(t, os.Chmod(src, 0o000))
				t.Cleanup(func() { _ = os.Chmod(src, 0o755) })
				return src, filepath.Join(t.TempDir(), "hooks")
			},
			wantErr: "read hooks dir",
		},
		{
			name: "returns error when hook file is unreadable",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				p := filepath.Join(src, "bad.sh")
				require.NoError(t, os.WriteFile(p, []byte("#!/bin/bash"), 0o000))
				t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
				return src, filepath.Join(t.TempDir(), "hooks")
			},
			wantErr: "read hook",
		},
		{
			name: "returns error when hooks dest dir write fails",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "hook.sh"), "#!/bin/bash")
				dest := filepath.Join(t.TempDir(), "hooks")
				require.NoError(t, os.MkdirAll(dest, 0o555))
				return src, dest
			},
			wantErr: "write hook",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srcDir, hooksDir := tt.setup(t)
			c := cline.New()
			err := cline.InstallHooks(context.Background(), c, srcDir, hooksDir)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, hooksDir)
			}
		})
	}
}
