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

package hermes_test

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

	"github.com/retr0h/agentpack/internal/driver/hermes"
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

func TestHermes_Name(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{name: "returns hermes-agent", want: "hermes-agent"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := hermes.New()
			assert.Equal(t, tt.want, h.Name())
		})
	}
}

func TestHermes_DisplayName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{name: "returns Hermes Agent", want: "Hermes Agent"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := hermes.New()
			assert.Equal(t, tt.want, h.DisplayName())
		})
	}
}

func TestHermes_SupportedTypes(t *testing.T) {
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

			h := hermes.New()
			assert.Equal(t, tt.want, h.SupportedTypes())
		})
	}
}

func TestHermes_Detect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		homeFunc     func() (string, error)
		wantDetected bool
	}{
		{
			name: "detect returns true when ~/.hermes exists",
			homeFunc: func() (string, error) {
				home := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(home, ".hermes"), 0o755))
				return home, nil
			},
			wantDetected: true,
		},
		{
			name: "detect returns false when ~/.hermes missing",
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

			h := hermes.New()
			hermes.SetUserHome(h, tt.homeFunc)
			assert.Equal(t, tt.wantDetected, h.Detect())
		})
	}
}

func TestHermes_Install(t *testing.T) {
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
			homeFunc: func() (string, error) {
				return t.TempDir(), nil
			},
			check: func(t *testing.T, dir string, _ string) {
				t.Helper()
				p := filepath.Join(dir, ".agents", "skills", "test-plugin", "review", "SKILL.md")
				_, err := os.Stat(p)
				assert.NoError(t, err)
			},
		},
		{
			name: "copies skills to ~/.hermes/skills/ globally",
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
				p := filepath.Join(home, ".hermes", "skills", "test-plugin", "my-skill.md")
				_, err := os.Stat(p)
				assert.NoError(t, err)
			},
		},
		{
			name: "merges hooks into ~/.hermes/config.yaml as YAML",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "hooks", "hooks.json"), map[string]any{
					"on_save": []any{
						map[string]any{
							"command":    "echo saved",
							"showOutput": true,
						},
					},
				})
				return src, t.TempDir()
			},
			homeFunc: func() func() (string, error) {
				home := t.TempDir()
				return func() (string, error) { return home, nil }
			}(),
			check: func(t *testing.T, _ string, home string) {
				t.Helper()
				data, err := os.ReadFile(filepath.Join(home, ".hermes", "config.yaml"))
				require.NoError(t, err)
				var doc map[string]any
				require.NoError(t, yaml.Unmarshal(data, &doc))
				hooks, ok := doc["hooks"].(map[string]any)
				require.True(t, ok)
				entries, ok := hooks["on_save"].([]any)
				require.True(t, ok)
				require.Len(t, entries, 1)
				entry, ok := entries[0].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "echo saved", entry["command"])
			},
		},
		{
			name: "installs from entries with skill + hook",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "skills", "k8s", "SKILL.md"), "# K8s")
				writeJSON(t, filepath.Join(src, "hooks", "hooks.json"), map[string]any{
					"on_open": []any{
						map[string]any{
							"command":    "echo opened",
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
			check: func(t *testing.T, dir string, home string) {
				t.Helper()
				_, err := os.Stat(filepath.Join(dir, ".agents", "skills", "k8s", "SKILL.md"))
				assert.NoError(t, err)
				data, readErr := os.ReadFile(filepath.Join(home, ".hermes", "config.yaml"))
				require.NoError(t, readErr)
				var doc map[string]any
				require.NoError(t, yaml.Unmarshal(data, &doc))
				hooks, ok := doc["hooks"].(map[string]any)
				require.True(t, ok)
				_, ok = hooks["on_open"]
				assert.True(t, ok)
			},
		},
		{
			name: "skips missing content dirs without error",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return t.TempDir(), t.TempDir()
			},
			homeFunc: func() (string, error) {
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
			homeFunc: func() (string, error) {
				return t.TempDir(), nil
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
			name: "hermesConfigPath home error propagates in installFromDirs",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "skills", "x.md"), "x")
				return src, t.TempDir()
			},
			homeFunc: func() (string, error) {
				return "", errors.New("no home for hooks")
			},
			wantErr: "home dir",
		},
		{
			name: "hermesConfigPath home error propagates in installFromEntries hook case",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return t.TempDir(), t.TempDir()
			},
			homeFunc: func() (string, error) {
				return "", errors.New("no home for hooks entry")
			},
			entries: []target.ContentEntry{
				{Name: "hooks", Type: "hook"},
			},
			wantErr: "home dir",
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
			name: "installSkillEntry mkdirAll failure propagates error via entries",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "skills", "k8s", "SKILL.md"), "# K8s")
				return src, t.TempDir()
			},
			mkdirFunc: func(string, os.FileMode) error {
				return errors.New("disk full on skill entry")
			},
			entriesFromSrc: func(src string) []target.ContentEntry {
				return []target.ContentEntry{
					{Name: "k8s", Type: "skill", Root: filepath.Join(src, "skills", "k8s")},
				}
			},
			wantErr: "mkdir skills dir",
		},
		{
			name: "installHooks error propagates in installFromDirs",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "hooks", "hooks.json"), map[string]any{
					"on_save": []any{map[string]any{"command": "echo lint", "showOutput": true}},
				})
				return src, t.TempDir()
			},
			homeFunc: func() func() (string, error) {
				home := t.TempDir()
				configPath := filepath.Join(home, ".hermes", "config.yaml")
				require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o755))
				require.NoError(t, os.WriteFile(configPath, []byte(`{}`), 0o000))
				t.Cleanup(func() { _ = os.Chmod(configPath, 0o644) })
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
					"on_save": []any{map[string]any{"command": "echo lint", "showOutput": true}},
				})
				return src, t.TempDir()
			},
			homeFunc: func() func() (string, error) {
				home := t.TempDir()
				configPath := filepath.Join(home, ".hermes", "config.yaml")
				require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o755))
				require.NoError(t, os.WriteFile(configPath, []byte(`{}`), 0o000))
				t.Cleanup(func() { _ = os.Chmod(configPath, 0o644) })
				return func() (string, error) { return home, nil }
			}(),
			entries: []target.ContentEntry{
				{Name: "hooks", Type: "hook"},
			},
			wantErr: "merge hooks",
		},
		{
			name: "installFromDirs context cancelled inside dir loop",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return t.TempDir(), t.TempDir()
			},
			homeFunc: func() (string, error) {
				return t.TempDir(), nil
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
			homeFunc: func() (string, error) {
				return t.TempDir(), nil
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
			homeFunc: func() (string, error) {
				return t.TempDir(), nil
			},
			wantErr: "enumerate installed files",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srcDir, installDir := tt.setup(t)
			h := hermes.New()

			var homeDir string
			if tt.homeFunc != nil {
				hermes.SetUserHome(h, func() (string, error) {
					hd, err := tt.homeFunc()
					homeDir = hd
					return hd, err
				})
			}
			if tt.cwdFunc != nil {
				hermes.SetCwd(h, tt.cwdFunc)
			}
			if tt.mkdirFunc != nil {
				hermes.SetOsMkdirAll(h, tt.mkdirFunc)
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

			files, err := h.Install(ctx, target.InstallOpts{
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

func TestHermes_List(t *testing.T) {
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

			h := hermes.New()
			got, err := h.List()

			require.NoError(t, err)
			assert.Len(t, got, tt.wantLen)
		})
	}
}

func TestHermes_InstallHooks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (srcDir, configPath string)
		wantErr string
		check   func(t *testing.T, configPath string)
	}{
		{
			name: "merges hooks into config.yaml as YAML",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "hooks", "hooks.json"), map[string]any{
					"on_save": []any{
						map[string]any{
							"command":    "echo lint",
							"showOutput": true,
						},
					},
				})
				return src, filepath.Join(t.TempDir(), ".hermes", "config.yaml")
			},
			check: func(t *testing.T, configPath string) {
				t.Helper()
				data, err := os.ReadFile(configPath)
				require.NoError(t, err)
				var doc map[string]any
				require.NoError(t, yaml.Unmarshal(data, &doc))
				hooks, ok := doc["hooks"].(map[string]any)
				require.True(t, ok)
				entries, ok := hooks["on_save"].([]any)
				require.True(t, ok)
				require.Len(t, entries, 1)
				entry, ok := entries[0].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "echo lint", entry["command"])
				assert.Equal(t, "test-plugin", entry["_plugin"])
			},
		},
		{
			name: "creates config.yaml if absent",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "hooks", "hooks.json"), map[string]any{
					"on_open": []any{
						map[string]any{
							"command":    "echo opened",
							"showOutput": false,
						},
					},
				})
				return src, filepath.Join(t.TempDir(), ".hermes", "config.yaml")
			},
			check: func(t *testing.T, configPath string) {
				t.Helper()
				_, err := os.Stat(configPath)
				require.NoError(t, err)
				data, err := os.ReadFile(configPath)
				require.NoError(t, err)
				var doc map[string]any
				require.NoError(t, yaml.Unmarshal(data, &doc))
				hooks, ok := doc["hooks"].(map[string]any)
				require.True(t, ok)
				_, ok = hooks["on_open"]
				assert.True(t, ok)
			},
		},
		{
			name: "preserves existing YAML keys when merging hooks",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "hooks", "hooks.json"), map[string]any{
					"on_save": []any{
						map[string]any{
							"command":    "echo new-hook",
							"showOutput": true,
						},
					},
				})
				configDir := t.TempDir()
				configPath := filepath.Join(configDir, ".hermes", "config.yaml")
				writeYAML(t, configPath, map[string]any{
					"model":   "claude-3-opus",
					"api_key": "sk-test",
					"hooks": map[string]any{
						"on_open": []any{
							map[string]any{
								"command":    "echo existing",
								"showOutput": false,
								"_plugin":    "other-plugin",
							},
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
				assert.Equal(t, "claude-3-opus", doc["model"])
				assert.Equal(t, "sk-test", doc["api_key"])
				hooks, ok := doc["hooks"].(map[string]any)
				require.True(t, ok)
				_, ok = hooks["on_open"]
				assert.True(t, ok)
				_, ok = hooks["on_save"]
				assert.True(t, ok)
			},
		},
		{
			name: "no-op when hooks dir is absent",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return t.TempDir(), filepath.Join(t.TempDir(), ".hermes", "config.yaml")
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
				return src, filepath.Join(t.TempDir(), ".hermes", "config.yaml")
			},
			wantErr: "read hooks/hooks.json",
		},
		{
			name: "returns error when hooks.json contains invalid JSON",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "hooks", "hooks.json"), `{invalid`)
				return src, filepath.Join(t.TempDir(), ".hermes", "config.yaml")
			},
			wantErr: "parse hooks/hooks.json",
		},
		{
			name: "returns error when existing config.yaml is unreadable",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "hooks", "hooks.json"), map[string]any{
					"on_save": []any{
						map[string]any{"command": "echo lint", "showOutput": true},
					},
				})
				destDir := t.TempDir()
				configPath := filepath.Join(destDir, ".hermes", "config.yaml")
				require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o755))
				require.NoError(t, os.WriteFile(configPath, []byte(`{}`), 0o000))
				t.Cleanup(func() { _ = os.Chmod(configPath, 0o644) })
				return src, configPath
			},
			wantErr: "merge hooks",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srcDir, configPath := tt.setup(t)
			h := hermes.New()

			err := hermes.InstallHooks(context.Background(), h, srcDir, configPath, "test-plugin")

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

func TestHermes_ReadYAMLConfig(t *testing.T) {
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
			doc, err := hermes.ReadYAMLConfig(path)

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

func TestHermes_WriteYAMLConfig(t *testing.T) {
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
			err := hermes.WriteYAMLConfig(path, tt.doc)

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

func TestHermes_MergeHooksYAML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setup      func(t *testing.T) string
		hooks      map[string]any
		pluginName string
		wantErr    string
		check      func(t *testing.T, path string)
	}{
		{
			name: "skips non-slice hook entries",
			setup: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "config.yaml")
			},
			hooks: map[string]any{
				"on_save": "not-a-slice",
			},
			pluginName: "test-plugin",
			check: func(t *testing.T, path string) {
				t.Helper()
				_, err := os.Stat(path)
				require.NoError(t, err)
			},
		},
		{
			name: "skips non-map hook entries within slice",
			setup: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "config.yaml")
			},
			hooks: map[string]any{
				"on_save": []any{"string-not-map", 42},
			},
			pluginName: "test-plugin",
			check: func(t *testing.T, path string) {
				t.Helper()
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				var doc map[string]any
				require.NoError(t, yaml.Unmarshal(data, &doc))
				hooks, ok := doc["hooks"].(map[string]any)
				require.True(t, ok)
				entries, ok := hooks["on_save"].([]any)
				require.True(t, ok)
				assert.Empty(t, entries)
			},
		},
		{
			name: "returns error when config file is unreadable",
			setup: func(t *testing.T) string {
				t.Helper()
				p := filepath.Join(t.TempDir(), "config.yaml")
				writeFile(t, p, "hooks:\n  on_save: []\n")
				require.NoError(t, os.Chmod(p, 0o000))
				t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
				return p
			},
			hooks: map[string]any{
				"on_save": []any{map[string]any{"command": "echo lint"}},
			},
			pluginName: "test-plugin",
			wantErr:    "read ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := tt.setup(t)
			err := hermes.MergeHooksYAML(path, tt.pluginName, tt.hooks)

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

func TestHermes_GetOrCreateYAMLSlice(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		m    map[string]any
		key  string
		want []any
	}{
		{
			name: "returns existing slice",
			m:    map[string]any{"events": []any{"a", "b"}},
			key:  "events",
			want: []any{"a", "b"},
		},
		{
			name: "returns empty slice when key absent",
			m:    map[string]any{},
			key:  "events",
			want: []any{},
		},
		{
			name: "returns empty slice when value is not a slice",
			m:    map[string]any{"events": "not-a-slice"},
			key:  "events",
			want: []any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := hermes.GetOrCreateYAMLSlice(tt.m, tt.key)
			assert.Equal(t, tt.want, got)
		})
	}
}
