package claudecode_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/internal/driver/claudecode"
	"github.com/retr0h/agentpack/internal/driver/fs"
	"github.com/retr0h/agentpack/internal/metadata"
	"github.com/retr0h/agentpack/internal/target"
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

func TestClaudeCode_Name(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{name: "returns claude-code", want: "claude-code"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cc := claudecode.New()
			assert.Equal(t, tt.want, cc.Name())
		})
	}
}

func TestClaudeCode_DisplayName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{name: "returns Claude Code", want: "Claude Code"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cc := claudecode.New()
			assert.Equal(t, tt.want, cc.DisplayName())
		})
	}
}

func TestClaudeCode_SupportedTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		wantTypes []string
	}{
		{
			name:      "returns all supported content types",
			wantTypes: []string{"skill", "command", "hook", "agent", "mcp", "config"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cc := claudecode.New()
			assert.Equal(t, tt.wantTypes, cc.SupportedTypes())
		})
	}
}

func TestClaudeCode_Detect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		homeFunc     func() (string, error)
		wantDetected bool
	}{
		{
			name: "home error returns false",
			homeFunc: func() (string, error) {
				return "", errors.New("no home")
			},
			wantDetected: false,
		},
		{
			name: "detects when ~/.claude exists",
			homeFunc: func() (string, error) {
				home := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(home, ".claude"), 0o755))
				return home, nil
			},
			wantDetected: true,
		},
		{
			name: "not detected when ~/.claude absent",
			homeFunc: func() (string, error) {
				return t.TempDir(), nil
			},
			wantDetected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cc := claudecode.New()
			claudecode.SetUserHome(cc, tt.homeFunc)
			assert.Equal(t, tt.wantDetected, cc.Detect())
		})
	}
}

func TestInstall(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		setup          func(t *testing.T) (srcDir string, installDir string)
		entries        []target.ContentEntry
		entriesFromSrc func(src string) []target.ContentEntry
		homeFunc       func() (string, error)
		mkdirFunc      func(string, os.FileMode) error
		wantErr        string
		check          func(t *testing.T, installDir string)
	}{
		{
			name: "copies skills recursively to .claude/skills/",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "skills", "review", "SKILL.md"), "# Review")
				return src, t.TempDir()
			},
			check: func(t *testing.T, dir string) {
				t.Helper()
				p := filepath.Join(dir, ".claude", "skills", "review", "SKILL.md")
				_, err := os.Stat(p)
				assert.NoError(t, err)
			},
		},
		{
			name: "copies commands to .claude/commands/",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "commands", "scan.md"), "# Scan")
				return src, t.TempDir()
			},
			check: func(t *testing.T, dir string) {
				t.Helper()
				_, err := os.Stat(filepath.Join(dir, ".claude", "commands", "scan.md"))
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
			wantErr: "context canceled",
		},
		{
			name: "uses home dir when opts.Dir is empty",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "skills", "x.md"), "x")
				return src, ""
			},
			homeFunc: func() (string, error) {
				return t.TempDir(), nil
			},
			check: func(_ *testing.T, _ string) {},
		},
		{
			name: "home dir error when opts.Dir is empty",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return t.TempDir(), ""
			},
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
			wantErr: "install skills",
		},
		{
			name: "merges mcp/*.json into .claude/settings.json",
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
			check: func(t *testing.T, dir string) {
				t.Helper()
				data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
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
			name: "merges hooks/hooks.json into .claude/settings.json",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "hooks", "hooks.json"), map[string]any{
					"PreToolUse": []any{
						map[string]any{"matcher": "Bash"},
					},
				})
				return src, t.TempDir()
			},
			check: func(t *testing.T, dir string) {
				t.Helper()
				data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
				require.NoError(t, err)
				var doc map[string]any
				require.NoError(t, json.Unmarshal(data, &doc))
				hooks, ok := doc["hooks"].(map[string]any)
				require.True(t, ok)
				entries, ok := hooks["PreToolUse"].([]any)
				require.True(t, ok)
				assert.Len(t, entries, 1)
			},
		},
		{
			name: "merges settings/*.json into .claude/settings.json",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "settings", "prefs.json"), map[string]any{
					"theme": "dark",
				})
				return src, t.TempDir()
			},
			check: func(t *testing.T, dir string) {
				t.Helper()
				data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
				require.NoError(t, err)
				var doc map[string]any
				require.NoError(t, json.Unmarshal(data, &doc))
				assert.Equal(t, "dark", doc["theme"])
			},
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
			wantErr: `missing or invalid "name" field`,
		},
		{
			name: "installs from entries when provided",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "skills", "k8s", "SKILL.md"), "# K8s")
				writeFile(t, filepath.Join(src, "commands", "scan.md"), "# Scan")
				return src, t.TempDir()
			},
			entriesFromSrc: func(src string) []target.ContentEntry {
				return []target.ContentEntry{
					{Name: "k8s", Type: "skill", Root: filepath.Join(src, "skills", "k8s")},
				}
			},
			check: func(t *testing.T, dir string) {
				t.Helper()
				// The k8s skill entry was listed — must be installed.
				_, err := os.Stat(filepath.Join(dir, ".claude", "skills", "k8s", "SKILL.md"))
				assert.NoError(t, err)

				// The commands dir was NOT listed in entries — must be absent.
				_, err = os.Stat(filepath.Join(dir, ".claude", "commands", "scan.md"))
				assert.True(t, os.IsNotExist(err))
			},
		},
		{
			name: "installs command entry via entries list",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "commands", "deploy", "deploy.md"), "# Deploy")
				return src, t.TempDir()
			},
			entriesFromSrc: func(src string) []target.ContentEntry {
				return []target.ContentEntry{
					{
						Name: "deploy",
						Type: "command",
						Root: filepath.Join(src, "commands", "deploy"),
					},
				}
			},
			check: func(t *testing.T, dir string) {
				t.Helper()
				_, err := os.Stat(filepath.Join(dir, ".claude", "commands", "deploy", "deploy.md"))
				assert.NoError(t, err)
			},
		},
		{
			name: "installs agent entry via entries list",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "agents", "my-agent", "AGENT.md"), "# Agent")
				return src, t.TempDir()
			},
			entriesFromSrc: func(src string) []target.ContentEntry {
				return []target.ContentEntry{
					{
						Name: "my-agent",
						Type: "agent",
						Root: filepath.Join(src, "agents", "my-agent"),
					},
				}
			},
			check: func(t *testing.T, dir string) {
				t.Helper()
				_, err := os.Stat(filepath.Join(dir, ".claude", "agents", "my-agent", "AGENT.md"))
				assert.NoError(t, err)
			},
		},
		{
			name: "installs mcp entry via entries list",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "mcp", "srv.json"), map[string]any{
					"name": "srv",
					"type": "stdio",
				})
				return src, t.TempDir()
			},
			entriesFromSrc: func(_ string) []target.ContentEntry {
				return []target.ContentEntry{
					{Name: "srv", Type: "mcp"},
				}
			},
			check: func(t *testing.T, dir string) {
				t.Helper()
				_, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
				assert.NoError(t, err)
			},
		},
		{
			name: "installs hook entry via entries list",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "hooks", "hooks.json"), map[string]any{
					"PreToolUse": []any{map[string]any{"matcher": "Bash"}},
				})
				return src, t.TempDir()
			},
			entriesFromSrc: func(_ string) []target.ContentEntry {
				return []target.ContentEntry{
					{Name: "hooks", Type: "hook"},
				}
			},
			check: func(t *testing.T, dir string) {
				t.Helper()
				_, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
				assert.NoError(t, err)
			},
		},
		{
			name: "installs config entry via entries list",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "settings", "prefs.json"), map[string]any{
					"theme": "dark",
				})
				return src, t.TempDir()
			},
			entriesFromSrc: func(_ string) []target.ContentEntry {
				return []target.ContentEntry{
					{Name: "prefs", Type: "config"},
				}
			},
			check: func(t *testing.T, dir string) {
				t.Helper()
				data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
				require.NoError(t, err)
				var doc map[string]any
				require.NoError(t, json.Unmarshal(data, &doc))
				assert.Equal(t, "dark", doc["theme"])
			},
		},
		{
			name: "entries mcp install error is propagated",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				dst := t.TempDir()
				// Write a duplicate mcp entry that conflicts with existing settings.
				writeJSON(t, filepath.Join(src, "mcp", "conflict.json"), map[string]any{
					"name": "conflict-srv",
					"type": "stdio",
				})
				writeJSON(t, filepath.Join(dst, ".claude", "settings.json"), map[string]any{
					"mcpServers": map[string]any{
						"conflict-srv": map[string]any{"type": "stdio"},
					},
				})
				return src, dst
			},
			entriesFromSrc: func(_ string) []target.ContentEntry {
				return []target.ContentEntry{
					{Name: "conflict-srv", Type: "mcp"},
				}
			},
			wantErr: "already exists",
		},
		{
			name: "entries hook install error is propagated",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				dst := t.TempDir()
				// Write an unreadable hooks file.
				hooksPath := filepath.Join(src, "hooks", "hooks.json")
				require.NoError(t, os.MkdirAll(filepath.Dir(hooksPath), 0o755))
				require.NoError(t, os.WriteFile(hooksPath, []byte(`{}`), 0o000))
				t.Cleanup(func() { _ = os.Chmod(hooksPath, 0o644) })
				return src, dst
			},
			entriesFromSrc: func(_ string) []target.ContentEntry {
				return []target.ContentEntry{
					{Name: "hooks", Type: "hook"},
				}
			},
			wantErr: "read hooks/hooks.json",
		},
		{
			name: "entries config install error is propagated",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				dst := t.TempDir()
				// Write an unreadable settings file.
				prefsPath := filepath.Join(src, "settings", "prefs.json")
				require.NoError(t, os.MkdirAll(filepath.Dir(prefsPath), 0o755))
				require.NoError(t, os.WriteFile(prefsPath, []byte(`{}`), 0o000))
				t.Cleanup(func() { _ = os.Chmod(prefsPath, 0o644) })
				return src, dst
			},
			entriesFromSrc: func(_ string) []target.ContentEntry {
				return []target.ContentEntry{
					{Name: "prefs", Type: "config"},
				}
			},
			wantErr: "read settings/",
		},
		{
			name: "entries skill copyTreeTracked error is propagated",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "skills", "fail-skill", "SKILL.md"), "# Fail")
				return src, t.TempDir()
			},
			mkdirFunc: func(string, os.FileMode) error {
				return errors.New("disk full")
			},
			entriesFromSrc: func(src string) []target.ContentEntry {
				return []target.ContentEntry{
					{
						Name: "fail-skill",
						Type: "skill",
						Root: filepath.Join(src, "skills", "fail-skill"),
					},
				}
			},
			wantErr: `install skill "fail-skill"`,
		},
		{
			name: "entries skill walkdir error on unreadable source dir",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				if os.Getuid() == 0 {
					t.Skip("root bypasses permission checks")
				}
				src := t.TempDir()
				skillDir := filepath.Join(src, "skills", "locked-skill")
				require.NoError(t, os.MkdirAll(skillDir, 0o755))
				require.NoError(
					t,
					os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# x"), 0o644),
				)
				sub := filepath.Join(skillDir, "sub")
				require.NoError(t, os.MkdirAll(sub, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(sub, "f.md"), []byte("x"), 0o644))
				require.NoError(t, os.Chmod(sub, 0o000))
				t.Cleanup(func() { _ = os.Chmod(sub, 0o755) })
				return src, t.TempDir()
			},
			entriesFromSrc: func(src string) []target.ContentEntry {
				return []target.ContentEntry{
					{
						Name: "locked-skill",
						Type: "skill",
						Root: filepath.Join(src, "skills", "locked-skill"),
					},
				}
			},
			wantErr: `install skill "locked-skill"`,
		},
		{
			name: "returns error on mcp name conflict",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				dst := t.TempDir()
				writeJSON(t, filepath.Join(src, "mcp", "dup.json"), map[string]any{
					"name": "dup-srv",
					"type": "stdio",
				})
				writeJSON(t, filepath.Join(dst, ".claude", "settings.json"), map[string]any{
					"mcpServers": map[string]any{
						"dup-srv": map[string]any{"type": "stdio"},
					},
				})
				return src, dst
			},
			wantErr: "already exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srcDir, installDir := tt.setup(t)
			cc := claudecode.New()
			if tt.homeFunc != nil {
				claudecode.SetUserHome(cc, tt.homeFunc)
			}
			if tt.mkdirFunc != nil {
				claudecode.SetOsMkdirAll(cc, tt.mkdirFunc)
			}
			ctx := context.Background()
			if tt.wantErr == "context canceled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			entries := tt.entries
			if tt.entriesFromSrc != nil {
				entries = tt.entriesFromSrc(srcDir)
			}
			files, err := cc.Install(ctx, target.InstallOpts{
				Name: "test", SourceDir: srcDir, Dir: installDir,
				Entries: entries,
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

func TestClaudeCode_List(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		homeFunc  func(t *testing.T) func() (string, error)
		wantErr   string
		wantNames []string
	}{
		{
			name: "home error propagates",
			homeFunc: func(t *testing.T) func() (string, error) {
				t.Helper()
				return func() (string, error) { return "", errors.New("no home") }
			},
			wantErr: "home dir",
		},
		{
			name: "marketplaces dir absent returns nil",
			homeFunc: func(t *testing.T) func() (string, error) {
				t.Helper()
				home := t.TempDir()
				return func() (string, error) { return home, nil }
			},
			wantNames: nil,
		},
		{
			name: "returns plugins with valid metadata",
			homeFunc: func(t *testing.T) func() (string, error) {
				t.Helper()
				home := t.TempDir()
				mdir := filepath.Join(
					home,
					".claude",
					"plugins",
					"marketplaces",
					"my-plugin",
					".agentpack",
				)
				writeJSON(t, filepath.Join(mdir, "metadata.json"), metadata.Metadata{
					Name:           "my-plugin",
					Version:        "v1.0.0",
					GitCommitSHA:   "abc1234567890",
					BuildTimestamp: "2026-05-24T12:00:00Z",
				})
				return func() (string, error) { return home, nil }
			},
			wantNames: []string{"my-plugin"},
		},
		{
			name: "skips non-directory entries in marketplaces dir",
			homeFunc: func(t *testing.T) func() (string, error) {
				t.Helper()
				home := t.TempDir()
				mdir := filepath.Join(home, ".claude", "plugins", "marketplaces")
				writeFile(t, filepath.Join(mdir, "not-a-dir.txt"), "file")
				return func() (string, error) { return home, nil }
			},
			wantNames: nil,
		},
		{
			name: "skips plugin dirs with invalid JSON metadata",
			homeFunc: func(t *testing.T) func() (string, error) {
				t.Helper()
				home := t.TempDir()
				mdir := filepath.Join(
					home,
					".claude",
					"plugins",
					"marketplaces",
					"bad-plugin",
					".agentpack",
				)
				writeFile(t, filepath.Join(mdir, "metadata.json"), "not-json{{{")
				return func() (string, error) { return home, nil }
			},
			wantNames: nil,
		},
		{
			name: "skips plugin dirs without metadata file",
			homeFunc: func(t *testing.T) func() (string, error) {
				t.Helper()
				home := t.TempDir()
				mdir := filepath.Join(home, ".claude", "plugins", "marketplaces", "no-meta")
				require.NoError(t, os.MkdirAll(mdir, 0o755))
				return func() (string, error) { return home, nil }
			},
			wantNames: nil,
		},
		{
			name: "readdir error on unreadable marketplaces dir",
			homeFunc: func(t *testing.T) func() (string, error) {
				t.Helper()
				home := t.TempDir()
				mdir := filepath.Join(home, ".claude", "plugins", "marketplaces")
				require.NoError(t, os.MkdirAll(mdir, 0o755))
				require.NoError(t, os.Chmod(mdir, 0o000))
				t.Cleanup(func() { _ = os.Chmod(mdir, 0o755) })
				return func() (string, error) { return home, nil }
			},
			wantErr: "read marketplaces dir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cc := claudecode.New()
			claudecode.SetUserHome(cc, tt.homeFunc(t))

			plugins, err := cc.List()

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.Len(t, plugins, len(tt.wantNames))

			for i, name := range tt.wantNames {
				assert.Equal(t, name, plugins[i].Name)
			}
		})
	}
}

func TestCopyFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (src, dst string)
		wantErr string
	}{
		{
			name: "copies file successfully",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := filepath.Join(t.TempDir(), "src.txt")
				require.NoError(t, os.WriteFile(src, []byte("hello"), 0o644))
				dst := filepath.Join(t.TempDir(), "dst.txt")
				return src, dst
			},
		},
		{
			name: "read error on missing src",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return filepath.Join(
						t.TempDir(),
						"nonexistent.txt",
					), filepath.Join(
						t.TempDir(),
						"dst.txt",
					)
			},
			wantErr: "read",
		},
		{
			name: "write error when dst is an existing directory",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := filepath.Join(t.TempDir(), "src.txt")
				require.NoError(t, os.WriteFile(src, []byte("data"), 0o644))
				// dst points at an existing directory — os.WriteFile fails with EISDIR.
				dstDir := t.TempDir()
				existingDir := filepath.Join(dstDir, "subdir")
				require.NoError(t, os.MkdirAll(existingDir, 0o755))
				return src, existingDir
			},
			wantErr: "is a directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src, dst := tt.setup(t)
			err := fs.CopyFile(src, dst)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestInstallMCP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (srcDir, settingsPath string)
		ctx     func() context.Context
		wantErr string
	}{
		{
			name: "no-op when mcp dir is absent",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return t.TempDir(), filepath.Join(t.TempDir(), "settings.json")
			},
		},
		{
			name: "context cancelled inside entry loop",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "mcp", "srv.json"), map[string]any{
					"name": "srv",
					"type": "stdio",
				})
				return src, filepath.Join(t.TempDir(), "settings.json")
			},
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			wantErr: "context canceled",
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
				return src, filepath.Join(t.TempDir(), "settings.json")
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
				return src, filepath.Join(t.TempDir(), "settings.json")
			},
			wantErr: "read mcp/",
		},
		{
			name: "returns error when mcp json contains invalid JSON",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "mcp", "srv.json"), `{invalid`)
				return src, filepath.Join(t.TempDir(), "settings.json")
			},
			wantErr: "parse mcp/",
		},
		{
			name: "skips directories and non-json entries in mcp dir",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				// Create a subdirectory and a non-.json file inside mcp/.
				require.NoError(t, os.MkdirAll(filepath.Join(src, "mcp", "subdir"), 0o755))
				writeFile(t, filepath.Join(src, "mcp", "readme.txt"), "skip me")
				return src, filepath.Join(t.TempDir(), "settings.json")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srcDir, settingsPath := tt.setup(t)
			ctx := context.Background()
			if tt.ctx != nil {
				ctx = tt.ctx()
			}
			cc := claudecode.New()
			err := claudecode.InstallMCP(ctx, cc, srcDir, settingsPath)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestInstallHooks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (srcDir, settingsPath string)
		wantErr string
	}{
		{
			name: "no-op when hooks dir is absent",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return t.TempDir(), filepath.Join(t.TempDir(), "settings.json")
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
				return src, filepath.Join(t.TempDir(), "settings.json")
			},
			wantErr: "read hooks/hooks.json",
		},
		{
			name: "returns error when hooks.json contains invalid JSON",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "hooks", "hooks.json"), `{invalid`)
				return src, filepath.Join(t.TempDir(), "settings.json")
			},
			wantErr: "parse hooks/hooks.json",
		},
		{
			name: "returns error when MergeHooks fails due to unreadable settings",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "hooks", "hooks.json"), map[string]any{
					"PreToolUse": []any{map[string]any{"matcher": "Bash"}},
				})
				sp := filepath.Join(t.TempDir(), "settings.json")
				require.NoError(t, os.WriteFile(sp, []byte(`{}`), 0o000))
				t.Cleanup(func() { _ = os.Chmod(sp, 0o644) })
				return src, sp
			},
			wantErr: "merge hooks",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srcDir, settingsPath := tt.setup(t)
			cc := claudecode.New()
			err := claudecode.InstallHooks(
				context.Background(),
				cc,
				srcDir,
				settingsPath,
				"test-plugin",
			)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestInstallSettings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (srcDir, settingsPath string)
		ctx     func() context.Context
		wantErr string
	}{
		{
			name: "no-op when settings dir is absent",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return t.TempDir(), filepath.Join(t.TempDir(), "settings.json")
			},
		},
		{
			name: "returns error when settings dir is unreadable",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				d := filepath.Join(src, "settings")
				require.NoError(t, os.MkdirAll(d, 0o755))
				require.NoError(t, os.Chmod(d, 0o000))
				t.Cleanup(func() { _ = os.Chmod(d, 0o755) })
				return src, filepath.Join(t.TempDir(), "settings.json")
			},
			wantErr: "read settings dir",
		},
		{
			name: "returns error when a settings json file is unreadable",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				p := filepath.Join(src, "settings", "prefs.json")
				require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
				require.NoError(t, os.WriteFile(p, []byte(`{}`), 0o000))
				t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
				return src, filepath.Join(t.TempDir(), "settings.json")
			},
			wantErr: "read settings/",
		},
		{
			name: "returns error when a settings json file contains invalid JSON",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "settings", "prefs.json"), `{invalid`)
				return src, filepath.Join(t.TempDir(), "settings.json")
			},
			wantErr: "parse settings/",
		},
		{
			name: "context cancelled inside entry loop",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "settings", "prefs.json"), map[string]any{
					"theme": "dark",
				})
				return src, filepath.Join(t.TempDir(), "settings.json")
			},
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			wantErr: "context canceled",
		},
		{
			name: "skips directories and non-json entries in settings dir",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				// Create a subdirectory and a non-.json file inside settings/.
				require.NoError(t, os.MkdirAll(filepath.Join(src, "settings", "subdir"), 0o755))
				writeFile(t, filepath.Join(src, "settings", "readme.txt"), "skip me")
				return src, filepath.Join(t.TempDir(), "settings.json")
			},
		},
		{
			name: "returns error when MergeSettings fails due to unreadable settings file",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeJSON(t, filepath.Join(src, "settings", "prefs.json"), map[string]any{
					"theme": "dark",
				})
				sp := filepath.Join(t.TempDir(), "settings.json")
				require.NoError(t, os.WriteFile(sp, []byte(`{}`), 0o000))
				t.Cleanup(func() { _ = os.Chmod(sp, 0o644) })
				return src, sp
			},
			wantErr: "merge settings/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srcDir, settingsPath := tt.setup(t)
			ctx := context.Background()
			if tt.ctx != nil {
				ctx = tt.ctx()
			}
			cc := claudecode.New()
			err := claudecode.InstallSettings(ctx, cc, srcDir, settingsPath)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestFormatDate(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, ts, want string }{
		{"trims time", "2026-05-24T14:00:00Z", "2026-05-24"},
		{"no T", "2026-05-24", "2026-05-24"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, claudecode.FormatDate(tt.ts))
		})
	}
}
