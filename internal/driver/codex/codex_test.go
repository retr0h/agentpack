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

package codex_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/internal/driver/codex"
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

func TestCodex_Name(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{name: "returns codex", want: "codex"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := codex.New()
			assert.Equal(t, tt.want, c.Name())
		})
	}
}

func TestCodex_DisplayName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{name: "returns Codex", want: "Codex"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := codex.New()
			assert.Equal(t, tt.want, c.DisplayName())
		})
	}
}

func TestCodex_SupportedTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want []string
	}{
		{name: "returns skill hook and config", want: []string{"skill", "hook", "config"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := codex.New()
			assert.Equal(t, tt.want, c.SupportedTypes())
		})
	}
}

func TestCodex_Detect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		homeFunc     func() (string, error)
		getenvFunc   func(string) string
		wantDetected bool
	}{
		{
			name: "detect returns true when ~/.codex exists",
			homeFunc: func() (string, error) {
				home := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(home, ".codex"), 0o755))
				return home, nil
			},
			wantDetected: true,
		},
		{
			name: "detect returns false when ~/.codex missing",
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
		{
			name: "CODEX_HOME env override to existing dir returns true",
			homeFunc: func() (string, error) {
				return t.TempDir(), nil
			},
			getenvFunc: func(key string) string {
				if key == "CODEX_HOME" {
					return t.TempDir()
				}
				return ""
			},
			wantDetected: true,
		},
		{
			name: "CODEX_HOME env override to missing dir returns false",
			homeFunc: func() (string, error) {
				return t.TempDir(), nil
			},
			getenvFunc: func(key string) string {
				if key == "CODEX_HOME" {
					return "/nonexistent/path/for/agentpack/test"
				}
				return ""
			},
			wantDetected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := codex.New()
			codex.SetUserHome(c, tt.homeFunc)
			if tt.getenvFunc != nil {
				codex.SetGetenv(c, tt.getenvFunc)
			}
			assert.Equal(t, tt.wantDetected, c.Detect())
		})
	}
}

func TestCodex_Install(t *testing.T) {
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
			name: "copies skills to ~/.codex/skills/ globally",
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
					".codex",
					"skills",
					"test-plugin",
					"my-skill.md",
				)
				_, err := os.Stat(p)
				assert.NoError(t, err)
			},
		},
		{
			name: "merges hooks into .codex/hooks/hooks.json locally",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "hooks", "hooks.json"), map[string]any{
					"PreToolUse": []any{
						map[string]any{
							"command":    "echo pre-tool",
							"showOutput": true,
						},
					},
				})
				return src, t.TempDir()
			},
			check: func(t *testing.T, dir string, _ string) {
				t.Helper()
				data, err := os.ReadFile(filepath.Join(dir, ".codex", "hooks", "hooks.json"))
				require.NoError(t, err)
				var doc map[string]any
				require.NoError(t, json.Unmarshal(data, &doc))
				hooks, ok := doc["hooks"].(map[string]any)
				require.True(t, ok)
				entries, ok := hooks["PreToolUse"].([]any)
				require.True(t, ok)
				require.Len(t, entries, 1)
				entry, ok := entries[0].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "echo pre-tool", entry["command"])
			},
		},
		{
			name: "installs from entries with skill + hook",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "skills", "k8s", "SKILL.md"), "# K8s")
				writeJSON(t, filepath.Join(src, "hooks", "hooks.json"), map[string]any{
					"PostToolUse": []any{
						map[string]any{
							"command":    "echo post-tool",
							"showOutput": false,
						},
					},
				})
				return src, t.TempDir()
			},
			homeFunc: func() func() (string, error) {
				home := t.TempDir()
				return func() (string, error) { return home, nil }
			}(),
			entriesFromSrc: func(src string) []target.ContentEntry {
				return []target.ContentEntry{
					{Name: "k8s", Type: "skill", Root: filepath.Join(src, "skills", "k8s")},
					{Name: "hooks", Type: "hook", Root: filepath.Join(src, "hooks")},
				}
			},
			check: func(t *testing.T, dir string, _ string) {
				t.Helper()
				// Skill must be installed to .agents/skills/k8s/.
				_, err := os.Stat(filepath.Join(dir, ".agents", "skills", "k8s", "SKILL.md"))
				assert.NoError(t, err)
				// Hooks must be merged into .codex/hooks/hooks.json.
				hooksData, hooksErr := os.ReadFile(
					filepath.Join(dir, ".codex", "hooks", "hooks.json"),
				)
				require.NoError(t, hooksErr)
				var hooksDoc map[string]any
				require.NoError(t, json.Unmarshal(hooksData, &hooksDoc))
				hooks, ok := hooksDoc["hooks"].(map[string]any)
				require.True(t, ok)
				_, ok = hooks["PostToolUse"]
				assert.True(t, ok)
			},
		},
		{
			name: "installs config into .codex/config.toml via entries",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "settings", "model.json"), map[string]any{
					"model": "o3-pro",
				})
				return src, t.TempDir()
			},
			entries: []target.ContentEntry{
				{Name: "config", Type: "config"},
			},
			check: func(t *testing.T, dir string, _ string) {
				t.Helper()
				data, err := os.ReadFile(filepath.Join(dir, ".codex", "config.toml"))
				require.NoError(t, err)
				var cfg map[string]any
				require.NoError(t, toml.Unmarshal(data, &cfg))
				assert.Equal(t, "o3-pro", cfg["model"])
			},
		},
		{
			name: "installs config into .codex/config.toml via directory walk",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "settings", "approval.json"), map[string]any{
					"approval_mode": "full-auto",
				})
				return src, t.TempDir()
			},
			check: func(t *testing.T, dir string, _ string) {
				t.Helper()
				data, err := os.ReadFile(filepath.Join(dir, ".codex", "config.toml"))
				require.NoError(t, err)
				var cfg map[string]any
				require.NoError(t, toml.Unmarshal(data, &cfg))
				assert.Equal(t, "full-auto", cfg["approval_mode"])
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
			name: "hooksPath cwdFunc error propagates in installFromDirs",
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
			name: "hooksPath home error propagates in installFromEntries hook case when global",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return t.TempDir(), ""
			},
			global: true,
			homeFunc: func() (string, error) {
				return "", errors.New("no home for hooks entry")
			},
			entries: []target.ContentEntry{
				{Name: "hooks", Type: "hook"},
			},
			wantErr: "home dir",
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
			name: "cwdFunc success resolves dir for local install with empty Dir",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return t.TempDir(), ""
			},
			cwdFunc: func() func() (string, error) {
				home := t.TempDir()
				return func() (string, error) { return home, nil }
			}(),
			homeFunc: func() (string, error) {
				return t.TempDir(), nil
			},
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
			name: "installHooks error propagates in installFromDirs when hooksPath unreadable",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "hooks", "hooks.json"), map[string]any{
					"PreToolUse": []any{
						map[string]any{"command": "echo lint", "showOutput": true},
					},
				})
				installDir := t.TempDir()
				hooksFile := filepath.Join(installDir, ".codex", "hooks", "hooks.json")
				require.NoError(t, os.MkdirAll(filepath.Dir(hooksFile), 0o755))
				require.NoError(t, os.WriteFile(hooksFile, []byte(`{}`), 0o000))
				t.Cleanup(func() { _ = os.Chmod(hooksFile, 0o644) })
				return src, installDir
			},
			homeFunc: func() func() (string, error) {
				home := t.TempDir()
				return func() (string, error) { return home, nil }
			}(),
			wantErr: "merge hooks",
		},
		{
			name: "installHooks error propagates in installFromEntries hook case",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "hooks", "hooks.json"), map[string]any{
					"PreToolUse": []any{
						map[string]any{"command": "echo lint", "showOutput": true},
					},
				})
				installDir := t.TempDir()
				hooksFile := filepath.Join(installDir, ".codex", "hooks", "hooks.json")
				require.NoError(t, os.MkdirAll(filepath.Dir(hooksFile), 0o755))
				require.NoError(t, os.WriteFile(hooksFile, []byte(`{}`), 0o000))
				t.Cleanup(func() { _ = os.Chmod(hooksFile, 0o644) })
				return src, installDir
			},
			homeFunc: func() func() (string, error) {
				home := t.TempDir()
				return func() (string, error) { return home, nil }
			}(),
			entries: []target.ContentEntry{
				{Name: "hooks", Type: "hook"},
			},
			wantErr: "merge hooks",
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
			name: "codexConfigPath global home error in installFromEntries config case",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "settings", "x.json"), `{"key":"val"}`)
				return src, ""
			},
			global: true,
			homeFunc: func() (string, error) {
				return "", errors.New("no home for config entry")
			},
			entries: []target.ContentEntry{
				{Name: "config", Type: "config"},
			},
			wantErr: "home dir",
		},
		{
			name: "codexConfigPath local cwdFunc error in installFromEntries config case",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "settings", "x.json"), `{"key":"val"}`)
				return src, ""
			},
			cwdFunc: func() (string, error) {
				return "", errors.New("getwd failed for config entry")
			},
			entries: []target.ContentEntry{
				{Name: "config", Type: "config"},
			},
			wantErr: "getwd",
		},
		{
			name: "codexConfigPath cwdFunc error propagates in installFromDirs",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "skills", "x.md"), "x")
				writeFile(t, filepath.Join(src, "hooks", "hooks.json"), `{}`)
				return src, ""
			},
			cwdFunc: func() func() (string, error) {
				calls := 0
				return func() (string, error) {
					calls++
					if calls <= 2 {
						return t.TempDir(), nil
					}
					return "", errors.New("getwd third call failed")
				}
			}(),
			homeFunc: func() (string, error) {
				return t.TempDir(), nil
			},
			wantErr: "getwd",
		},
		{
			name: "installConfig error propagates in installFromDirs when config.toml unreadable",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "settings", "x.json"), `{"key":"val"}`)
				installDir := t.TempDir()
				cfgFile := filepath.Join(installDir, ".codex", "config.toml")
				require.NoError(t, os.MkdirAll(filepath.Dir(cfgFile), 0o755))
				require.NoError(t, os.WriteFile(cfgFile, []byte("key = \"val\"\n"), 0o000))
				t.Cleanup(func() { _ = os.Chmod(cfgFile, 0o644) })
				return src, installDir
			},
			homeFunc: func() func() (string, error) {
				home := t.TempDir()
				return func() (string, error) { return home, nil }
			}(),
			wantErr: "read",
		},
		{
			name: "installConfig error propagates in installFromEntries config case",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "settings", "x.json"), `{"key":"val"}`)
				installDir := t.TempDir()
				cfgFile := filepath.Join(installDir, ".codex", "config.toml")
				require.NoError(t, os.MkdirAll(filepath.Dir(cfgFile), 0o755))
				require.NoError(t, os.WriteFile(cfgFile, []byte("key = \"val\"\n"), 0o000))
				t.Cleanup(func() { _ = os.Chmod(cfgFile, 0o644) })
				return src, installDir
			},
			entries: []target.ContentEntry{
				{Name: "config", Type: "config"},
			},
			wantErr: "read",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srcDir, installDir := tt.setup(t)
			c := codex.New()

			// Track the home directory for check functions.
			var homeDir string
			if tt.homeFunc != nil {
				codex.SetUserHome(c, func() (string, error) {
					h, err := tt.homeFunc()
					homeDir = h
					return h, err
				})
			}
			if tt.cwdFunc != nil {
				codex.SetCwd(c, tt.cwdFunc)
			}
			if tt.mkdirFunc != nil {
				codex.SetOsMkdirAll(c, tt.mkdirFunc)
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

func TestCodex_List(t *testing.T) {
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

			c := codex.New()
			got, err := c.List()

			require.NoError(t, err)
			assert.Len(t, got, tt.wantLen)
		})
	}
}

func TestCodex_InstallConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(t *testing.T) (srcDir, cfgPath string)
		customCtx context.Context
		wantErr   string
		check     func(t *testing.T, cfgPath string)
	}{
		{
			name: "merges config into .codex/config.toml",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "settings", "model.json"), map[string]any{
					"model": "o3-pro",
				})
				return src, filepath.Join(t.TempDir(), ".codex", "config.toml")
			},
			check: func(t *testing.T, cfgPath string) {
				t.Helper()
				data, err := os.ReadFile(cfgPath)
				require.NoError(t, err)
				var cfg map[string]any
				require.NoError(t, toml.Unmarshal(data, &cfg))
				assert.Equal(t, "o3-pro", cfg["model"])
			},
		},
		{
			name: "creates config.toml if absent",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "settings", "notify.json"), map[string]any{
					"notify": true,
				})
				dir := t.TempDir()
				cfgPath := filepath.Join(dir, ".codex", "config.toml")
				// Ensure parent dir does not exist yet.
				_, err := os.Stat(filepath.Dir(cfgPath))
				require.True(t, os.IsNotExist(err))
				return src, cfgPath
			},
			check: func(t *testing.T, cfgPath string) {
				t.Helper()
				data, err := os.ReadFile(cfgPath)
				require.NoError(t, err)
				var cfg map[string]any
				require.NoError(t, toml.Unmarshal(data, &cfg))
				assert.Equal(t, true, cfg["notify"])
			},
		},
		{
			name: "preserves existing TOML keys when merging",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "settings", "model.json"), map[string]any{
					"model": "o3-pro",
				})
				dir := t.TempDir()
				cfgPath := filepath.Join(dir, ".codex", "config.toml")
				existing := map[string]any{
					"approval_mode": "suggest",
					"notify":        true,
				}
				existingData, err := toml.Marshal(existing)
				require.NoError(t, err)
				writeFile(t, cfgPath, string(existingData))
				return src, cfgPath
			},
			check: func(t *testing.T, cfgPath string) {
				t.Helper()
				data, err := os.ReadFile(cfgPath)
				require.NoError(t, err)
				var cfg map[string]any
				require.NoError(t, toml.Unmarshal(data, &cfg))
				assert.Equal(t, "o3-pro", cfg["model"])
				assert.Equal(t, "suggest", cfg["approval_mode"])
				assert.Equal(t, true, cfg["notify"])
			},
		},
		{
			name: "no-op when settings dir is absent",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return t.TempDir(), filepath.Join(t.TempDir(), "config.toml")
			},
		},
		{
			name: "returns error when settings dir is unreadable",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				settingsDir := filepath.Join(src, "settings")
				require.NoError(t, os.MkdirAll(settingsDir, 0o755))
				require.NoError(t, os.Chmod(settingsDir, 0o000))
				t.Cleanup(func() { _ = os.Chmod(settingsDir, 0o755) })
				return src, filepath.Join(t.TempDir(), "config.toml")
			},
			wantErr: "read settings dir",
		},
		{
			name: "returns error when settings JSON file is unreadable",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				p := filepath.Join(src, "settings", "bad.json")
				require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
				require.NoError(t, os.WriteFile(p, []byte(`{}`), 0o000))
				t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
				return src, filepath.Join(t.TempDir(), "config.toml")
			},
			wantErr: "read settings/bad.json",
		},
		{
			name: "returns error when settings JSON is invalid",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "settings", "broken.json"), `{invalid`)
				return src, filepath.Join(t.TempDir(), "config.toml")
			},
			wantErr: "parse settings/broken.json",
		},
		{
			name: "returns error when existing config.toml is unreadable",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "settings", "x.json"), map[string]any{
					"key": "val",
				})
				dir := t.TempDir()
				cfgPath := filepath.Join(dir, "config.toml")
				require.NoError(t, os.WriteFile(cfgPath, []byte("key = \"val\"\n"), 0o000))
				t.Cleanup(func() { _ = os.Chmod(cfgPath, 0o644) })
				return src, cfgPath
			},
			wantErr: "read",
		},
		{
			name: "returns error when existing config.toml contains invalid TOML",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "settings", "x.json"), map[string]any{
					"key": "val",
				})
				dir := t.TempDir()
				cfgPath := filepath.Join(dir, "config.toml")
				writeFile(t, cfgPath, "= invalid toml [[[")
				return src, cfgPath
			},
			wantErr: "parse",
		},
		{
			name: "context cancelled inside installConfig loop",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "settings", "x.json"), map[string]any{
					"key": "val",
				})
				return src, filepath.Join(t.TempDir(), ".codex", "config.toml")
			},
			customCtx: testutil.NewCancelAfterN(0),
			wantErr:   "context canceled",
		},
		{
			name: "de.IsDir skips directory entry in settings dir",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				settingsDir := filepath.Join(src, "settings")
				require.NoError(t, os.MkdirAll(filepath.Join(settingsDir, "subdir"), 0o755))
				writeJSON(t, filepath.Join(settingsDir, "x.json"), map[string]any{
					"key": "val",
				})
				return src, filepath.Join(t.TempDir(), ".codex", "config.toml")
			},
			check: func(t *testing.T, cfgPath string) {
				t.Helper()
				data, err := os.ReadFile(cfgPath)
				require.NoError(t, err)
				var cfg map[string]any
				require.NoError(t, toml.Unmarshal(data, &cfg))
				assert.Equal(t, "val", cfg["key"])
			},
		},
		{
			name: "non-json file is skipped in settings dir",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				settingsDir := filepath.Join(src, "settings")
				writeFile(t, filepath.Join(settingsDir, "ignore.txt"), "not json")
				writeJSON(t, filepath.Join(settingsDir, "x.json"), map[string]any{
					"key": "val",
				})
				return src, filepath.Join(t.TempDir(), ".codex", "config.toml")
			},
			check: func(t *testing.T, cfgPath string) {
				t.Helper()
				data, err := os.ReadFile(cfgPath)
				require.NoError(t, err)
				var cfg map[string]any
				require.NoError(t, toml.Unmarshal(data, &cfg))
				assert.Equal(t, "val", cfg["key"])
			},
		},
		{
			name: "empty TOML file produces non-nil map",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "settings", "x.json"), map[string]any{
					"key": "val",
				})
				dir := t.TempDir()
				cfgPath := filepath.Join(dir, "config.toml")
				writeFile(t, cfgPath, "")
				return src, cfgPath
			},
			check: func(t *testing.T, cfgPath string) {
				t.Helper()
				data, err := os.ReadFile(cfgPath)
				require.NoError(t, err)
				var cfg map[string]any
				require.NoError(t, toml.Unmarshal(data, &cfg))
				assert.Equal(t, "val", cfg["key"])
			},
		},
		{
			name: "writeTOML MkdirAll error propagates",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "settings", "x.json"), map[string]any{
					"key": "val",
				})
				parent := t.TempDir()
				require.NoError(t, os.Chmod(parent, 0o555))
				t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })
				return src, filepath.Join(parent, "subdir", "config.toml")
			},
			wantErr: "mkdir",
		},
		{
			name: "writeTOML WriteFile error propagates",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "settings", "x.json"), map[string]any{
					"key": "val",
				})
				parent := t.TempDir()
				require.NoError(t, os.Chmod(parent, 0o555))
				t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })
				return src, filepath.Join(parent, "config.toml")
			},
			wantErr: "write",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srcDir, cfgPath := tt.setup(t)
			c := codex.New()
			ctx := context.Background()
			if tt.customCtx != nil {
				ctx = tt.customCtx
			}
			err := codex.InstallConfig(ctx, c, srcDir, cfgPath)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, cfgPath)
			}
		})
	}
}
