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

package roo_test

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

	"github.com/retr0h/agentpack/internal/driver/roo"
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

func TestRoo_Name(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{name: "returns roo", want: "roo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := roo.New()
			assert.Equal(t, tt.want, r.Name())
		})
	}
}

func TestRoo_DisplayName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{name: "returns Roo Code", want: "Roo Code"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := roo.New()
			assert.Equal(t, tt.want, r.DisplayName())
		})
	}
}

func TestRoo_SupportedTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want []string
	}{
		{name: "returns skill and config", want: []string{"skill", "config"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := roo.New()
			assert.Equal(t, tt.want, r.SupportedTypes())
		})
	}
}

func TestRoo_Detect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		homeFunc     func() (string, error)
		wantDetected bool
	}{
		{
			name: "detect returns true when ~/.roo exists",
			homeFunc: func() (string, error) {
				home := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(home, ".roo"), 0o755))
				return home, nil
			},
			wantDetected: true,
		},
		{
			name: "detect returns false when ~/.roo missing",
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

			r := roo.New()
			roo.SetUserHome(r, tt.homeFunc)
			assert.Equal(t, tt.wantDetected, r.Detect())
		})
	}
}

func TestRoo_Install(t *testing.T) {
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
			name: "global installs skills into ~/.roo/skills/",
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
		{
			name: "merges settings into .roomodes from dirs path",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "settings", "modes.json"), map[string]any{
					"customModes": []any{map[string]any{"slug": "reviewer", "name": "Reviewer"}},
				})
				return src, t.TempDir()
			},
			check: func(t *testing.T, dir string) {
				t.Helper()
				data, err := os.ReadFile(filepath.Join(dir, ".roomodes"))
				require.NoError(t, err)
				var doc map[string]any
				require.NoError(t, yaml.Unmarshal(data, &doc))
				assert.Contains(t, doc, "customModes")
			},
		},
		{
			name: "merges settings into .roomodes from entries path",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "settings", "modes.json"), map[string]any{
					"customModes": []any{map[string]any{"slug": "writer", "name": "Writer"}},
				})
				return src, t.TempDir()
			},
			entriesFromSrc: func(src string) []target.ContentEntry {
				return []target.ContentEntry{
					{Name: "modes", Type: "config", Root: filepath.Join(src, "settings")},
				}
			},
			check: func(t *testing.T, dir string) {
				t.Helper()
				data, err := os.ReadFile(filepath.Join(dir, ".roomodes"))
				require.NoError(t, err)
				var doc map[string]any
				require.NoError(t, yaml.Unmarshal(data, &doc))
				assert.Contains(t, doc, "customModes")
			},
		},
		{
			name: "preserves existing keys in .roomodes when merging",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				dst := t.TempDir()
				writeYAML(t, filepath.Join(dst, ".roomodes"), map[string]any{
					"existingKey": "existingValue",
				})
				writeJSON(t, filepath.Join(src, "settings", "new.json"), map[string]any{
					"newKey": "newValue",
				})
				return src, dst
			},
			check: func(t *testing.T, dir string) {
				t.Helper()
				data, err := os.ReadFile(filepath.Join(dir, ".roomodes"))
				require.NoError(t, err)
				var doc map[string]any
				require.NoError(t, yaml.Unmarshal(data, &doc))
				assert.Equal(t, "existingValue", doc["existingKey"])
				assert.Equal(t, "newValue", doc["newKey"])
			},
		},
		{
			name: "config entry getwd error propagates",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "settings", "modes.json"), map[string]any{
					"customModes": []any{},
				})
				return src, ""
			},
			cwdFunc: func() (string, error) {
				return "", errors.New("getwd failed")
			},
			entriesFromSrc: func(src string) []target.ContentEntry {
				return []target.ContentEntry{
					{Name: "modes", Type: "config", Root: filepath.Join(src, "settings")},
				}
			},
			wantErr: "getwd",
		},
		{
			name: "installFromEntries ctx cancelled mid-loop returns error",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "skills", "a", "SKILL.md"), "# A")
				writeFile(t, filepath.Join(src, "skills", "b", "SKILL.md"), "# B")
				return src, t.TempDir()
			},
			entries: []target.ContentEntry{
				{Name: "a", Type: "skill"},
				{Name: "b", Type: "skill"},
			},
			customCtx: testutil.NewCancelAfterN(1),
			wantErr:   "context canceled",
		},
		{
			name: "installFromDirs roomodesPath cwdFunc error propagates on second call",
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
			wantErr: "getwd",
		},
		{
			name: "installFromDirs ctx cancelled after top-level Install check",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return t.TempDir(), t.TempDir()
			},
			customCtx: testutil.NewCancelAfterN(1),
			wantErr:   "context canceled",
		},
		{
			name: "config entry installConfig error propagates in installFromEntries",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "settings", "e.json"), map[string]any{"k": "v"})
				dst := t.TempDir()
				require.NoError(
					t,
					os.MkdirAll(filepath.Join(dst, ".agents", "skills", "test-plugin"), 0o755),
				)
				p := filepath.Join(dst, ".roomodes")
				writeFile(t, p, "")
				require.NoError(t, os.Chmod(p, 0o000))
				t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
				return src, dst
			},
			entriesFromSrc: func(src string) []target.ContentEntry {
				return []target.ContentEntry{
					{Name: "cfg", Type: "config", Root: filepath.Join(src, "settings")},
				}
			},
			wantErr: "merge settings/e.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srcDir, installDir := tt.setup(t)
			r := roo.New()

			if tt.homeFunc != nil {
				roo.SetUserHome(r, tt.homeFunc)
			}
			if tt.cwdFunc != nil {
				roo.SetCwd(r, tt.cwdFunc)
			}
			if tt.mkdirFunc != nil {
				roo.SetOsMkdirAll(r, tt.mkdirFunc)
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

			files, err := r.Install(ctx, target.InstallOpts{
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

func TestRoo_InstallConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (srcDir string, installDir string)
		cwdFunc func() (string, error)
		wantErr string
		check   func(t *testing.T, installDir string)
	}{
		{
			name: "creates .roomodes from settings json when absent",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "settings", "modes.json"), map[string]any{
					"customModes": []any{map[string]any{"slug": "coder", "name": "Coder"}},
				})
				return src, t.TempDir()
			},
			check: func(t *testing.T, dir string) {
				t.Helper()
				data, err := os.ReadFile(filepath.Join(dir, ".roomodes"))
				require.NoError(t, err)
				var doc map[string]any
				require.NoError(t, yaml.Unmarshal(data, &doc))
				assert.Contains(t, doc, "customModes")
			},
		},
		{
			name: "merges multiple json files into .roomodes",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(
					t,
					filepath.Join(src, "settings", "a.json"),
					map[string]any{"keyA": "valA"},
				)
				writeJSON(
					t,
					filepath.Join(src, "settings", "b.json"),
					map[string]any{"keyB": "valB"},
				)
				return src, t.TempDir()
			},
			check: func(t *testing.T, dir string) {
				t.Helper()
				data, err := os.ReadFile(filepath.Join(dir, ".roomodes"))
				require.NoError(t, err)
				var doc map[string]any
				require.NoError(t, yaml.Unmarshal(data, &doc))
				assert.Equal(t, "valA", doc["keyA"])
				assert.Equal(t, "valB", doc["keyB"])
			},
		},
		{
			name: "skips non-json files in settings dir",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "settings", "notes.txt"), "ignore me")
				writeJSON(
					t,
					filepath.Join(src, "settings", "real.json"),
					map[string]any{"key": "val"},
				)
				return src, t.TempDir()
			},
			check: func(t *testing.T, dir string) {
				t.Helper()
				data, err := os.ReadFile(filepath.Join(dir, ".roomodes"))
				require.NoError(t, err)
				var doc map[string]any
				require.NoError(t, yaml.Unmarshal(data, &doc))
				assert.Equal(t, "val", doc["key"])
			},
		},
		{
			name: "no-op when settings dir is absent",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return t.TempDir(), t.TempDir()
			},
			check: func(t *testing.T, dir string) {
				t.Helper()
				_, err := os.Stat(filepath.Join(dir, ".roomodes"))
				assert.True(t, os.IsNotExist(err))
			},
		},
		{
			name: "invalid json returns parse error",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "settings", "bad.json"), "{not json}")
				return src, t.TempDir()
			},
			wantErr: "parse settings/bad.json",
		},
		{
			name: "unreadable settings dir returns read dir error",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				settingsDir := filepath.Join(src, "settings")
				require.NoError(t, os.MkdirAll(settingsDir, 0o755))
				require.NoError(t, os.Chmod(settingsDir, 0o000))
				t.Cleanup(func() { _ = os.Chmod(settingsDir, 0o755) })
				return src, t.TempDir()
			},
			wantErr: "read settings dir",
		},
		{
			name: "unreadable settings json file returns read error",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				p := filepath.Join(src, "settings", "locked.json")
				writeFile(t, p, `{"key":"val"}`)
				require.NoError(t, os.Chmod(p, 0o000))
				t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
				return src, t.TempDir()
			},
			wantErr: "read settings/locked.json",
		},
		{
			name: "merge error when .roomodes is unwritable",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "settings", "m.json"), map[string]any{"k": "v"})
				dst := t.TempDir()
				require.NoError(
					t,
					os.MkdirAll(filepath.Join(dst, ".agents", "skills", "test-plugin"), 0o755),
				)
				p := filepath.Join(dst, ".roomodes")
				writeFile(t, p, "")
				require.NoError(t, os.Chmod(p, 0o000))
				t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
				return src, dst
			},
			wantErr: "merge settings/m.json",
		},
		{
			name: "readYAMLConfig returns empty map for empty yaml file",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "settings", "new.json"), map[string]any{"k": "v"})
				dst := t.TempDir()
				writeFile(t, filepath.Join(dst, ".roomodes"), "")
				return src, dst
			},
			check: func(t *testing.T, dir string) {
				t.Helper()
				data, err := os.ReadFile(filepath.Join(dir, ".roomodes"))
				require.NoError(t, err)
				var doc map[string]any
				require.NoError(t, yaml.Unmarshal(data, &doc))
				assert.Equal(t, "v", doc["k"])
			},
		},
		{
			name: "readYAMLConfig returns error for unreadable existing .roomodes",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "settings", "x.json"), map[string]any{"k": "v"})
				dst := t.TempDir()
				p := filepath.Join(dst, ".roomodes")
				writeFile(t, p, "k: v\n")
				require.NoError(t, os.Chmod(p, 0o000))
				t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
				return src, dst
			},
			wantErr: "merge settings/x.json",
		},
		{
			name: "readYAMLConfig returns parse error for invalid yaml in .roomodes",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "settings", "y.json"), map[string]any{"k": "v"})
				dst := t.TempDir()
				writeFile(t, filepath.Join(dst, ".roomodes"), "key: [unclosed")
				return src, dst
			},
			wantErr: "merge settings/y.json",
		},
		{
			name: "writeYAMLConfig WriteFile error when install dir is read-only",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "settings", "q.json"), map[string]any{"k": "v"})
				dst := t.TempDir()
				require.NoError(
					t,
					os.MkdirAll(filepath.Join(dst, ".agents", "skills", "test-plugin"), 0o755),
				)
				require.NoError(t, os.Chmod(dst, 0o555))
				t.Cleanup(func() { _ = os.Chmod(dst, 0o755) })
				return src, dst
			},
			wantErr: "merge settings/q.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srcDir, installDir := tt.setup(t)
			r := roo.New()

			if tt.cwdFunc != nil {
				roo.SetCwd(r, tt.cwdFunc)
			}

			ctx := context.Background()

			_, err := r.Install(ctx, target.InstallOpts{
				Name:      "test-plugin",
				SourceDir: srcDir,
				Dir:       installDir,
			})

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

func TestRoo_List(t *testing.T) {
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

			r := roo.New()
			got, err := r.List()

			require.NoError(t, err)
			assert.Len(t, got, tt.wantLen)
		})
	}
}
