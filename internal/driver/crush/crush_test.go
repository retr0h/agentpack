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

package crush_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/internal/driver/crush"
	"github.com/retr0h/agentpack/pkg/target"
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

func TestCrush_Name(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{name: "returns crush", want: "crush"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := crush.New()
			assert.Equal(t, tt.want, c.Name())
		})
	}
}

func TestCrush_DisplayName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{name: "returns Crush", want: "Crush"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := crush.New()
			assert.Equal(t, tt.want, c.DisplayName())
		})
	}
}

func TestCrush_SupportedTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want []string
	}{
		{name: "returns skill and hook", want: []string{"skill", "hook"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := crush.New()
			assert.Equal(t, tt.want, c.SupportedTypes())
		})
	}
}

func TestCrush_Detect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		configDirFunc func() (string, error)
		wantDetected  bool
	}{
		{
			name: "detect returns true when ~/.config/crush exists",
			configDirFunc: func() (string, error) {
				configDir := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(configDir, "crush"), 0o755))
				return configDir, nil
			},
			wantDetected: true,
		},
		{
			name: "detect returns false when ~/.config/crush missing",
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

			c := crush.New()
			crush.SetUserConfigDir(c, tt.configDirFunc)
			assert.Equal(t, tt.wantDetected, c.Detect())
		})
	}
}

func TestCrush_Install(t *testing.T) {
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
			name: "copies skills to ~/.config/crush/skills/ globally",
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
					".config",
					"crush",
					"skills",
					"test-plugin",
					"my-skill.md",
				)
				_, err := os.Stat(p)
				assert.NoError(t, err)
			},
		},
		{
			name: "merges hooks into crush.json in project root",
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
				data, err := os.ReadFile(filepath.Join(dir, "crush.json"))
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
				_, err := os.Stat(filepath.Join(dir, ".agents", "skills", "k8s", "SKILL.md"))
				assert.NoError(t, err)
				hooksData, hooksErr := os.ReadFile(filepath.Join(dir, "crush.json"))
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
				hooksFile := filepath.Join(installDir, "crush.json")
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
				hooksFile := filepath.Join(installDir, "crush.json")
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srcDir, installDir := tt.setup(t)
			c := crush.New()

			var homeDir string
			if tt.homeFunc != nil {
				crush.SetUserHome(c, func() (string, error) {
					h, err := tt.homeFunc()
					homeDir = h
					return h, err
				})
			}
			if tt.cwdFunc != nil {
				crush.SetCwd(c, tt.cwdFunc)
			}
			if tt.mkdirFunc != nil {
				crush.SetOsMkdirAll(c, tt.mkdirFunc)
			}

			ctx := context.Background()
			if tt.cancelCtx {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
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

func TestCrush_List(t *testing.T) {
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

			c := crush.New()
			got, err := c.List()

			require.NoError(t, err)
			assert.Len(t, got, tt.wantLen)
		})
	}
}
