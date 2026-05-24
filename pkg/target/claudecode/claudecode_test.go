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

package claudecode_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/retr0h/agentpack/pkg/metadata"
	"github.com/retr0h/agentpack/pkg/target"
	"github.com/retr0h/agentpack/pkg/target/claudecode"
)

// --------------------------------------------------------------------------
// helpers
// --------------------------------------------------------------------------

// writeMeta creates .agentpack/metadata.json inside dir/marketplaces/<name>/.
func writeMeta(t *testing.T, home string, meta metadata.Metadata) string {
	t.Helper()

	dir := filepath.Join(home, ".claude", "plugins", "marketplaces", meta.Name)
	agentpackDir := filepath.Join(dir, ".agentpack")

	if err := os.MkdirAll(agentpackDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if err := os.WriteFile(filepath.Join(agentpackDir, "metadata.json"), data, 0o644); err != nil {
		t.Fatalf("write metadata.json: %v", err)
	}

	return dir
}

// makeSourceDir builds a minimal extracted-archive directory tree in a temp
// location and returns its path. If meta is non-nil an .agentpack/metadata.json
// is written; if addYAML is true an agentpack.yaml is also added.
func makeSourceDir(t *testing.T, name, version string, meta *metadata.Metadata, addYAML bool) string {
	t.Helper()

	dir := t.TempDir()
	agentpackDir := filepath.Join(dir, ".agentpack")

	if err := os.MkdirAll(agentpackDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if meta != nil {
		data, err := json.Marshal(meta)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		if err := os.WriteFile(filepath.Join(agentpackDir, "metadata.json"), data, 0o644); err != nil {
			t.Fatalf("write metadata.json: %v", err)
		}
	}

	if addYAML {
		yamlContent := "name: " + name + "\nversion: " + version + "\ndescription: Test plugin\n"
		if err := os.WriteFile(filepath.Join(agentpackDir, "agentpack.yaml"), []byte(yamlContent), 0o644); err != nil {
			t.Fatalf("write agentpack.yaml: %v", err)
		}
	}

	// Add a skill file so the archive is non-empty.
	skillsDir := filepath.Join(dir, "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}

	if err := os.WriteFile(filepath.Join(skillsDir, "intro.md"), []byte("# Intro"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	return dir
}

// --------------------------------------------------------------------------
// TestName
// --------------------------------------------------------------------------

func TestName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{
			name: "returns claude-code",
			want: "claude-code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cc := &claudecode.ClaudeCode{}
			if got := cc.Name(); got != tt.want {
				t.Errorf("Name() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestDisplayName
// --------------------------------------------------------------------------

func TestDisplayName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{
			name: "returns Claude Code",
			want: "Claude Code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cc := &claudecode.ClaudeCode{}
			if got := cc.DisplayName(); got != tt.want {
				t.Errorf("DisplayName() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestDetect
// --------------------------------------------------------------------------

func TestDetect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setupHome func(t *testing.T) string
		want      bool
	}{
		{
			name: "returns true when ~/.claude exists",
			setupHome: func(t *testing.T) string {
				t.Helper()
				home := t.TempDir()
				if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				return home
			},
			want: true,
		},
		{
			name: "returns false when ~/.claude does not exist",
			setupHome: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			want: false,
		},
		{
			name: "returns false when home dir lookup fails",
			setupHome: func(t *testing.T) string {
				t.Helper()
				return "" // signal to use error-returning stub
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			homeDir := tt.setupHome(t)

			cc := claudecode.New()

			if homeDir == "" {
				claudecode.SetUserHome(cc, func() (string, error) {
					return "", errors.New("no home")
				})
			} else {
				claudecode.SetUserHome(cc, func() (string, error) { return homeDir, nil })
			}

			if got := cc.Detect(); got != tt.want {
				t.Errorf("Detect() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestInstall
// --------------------------------------------------------------------------

func TestInstall(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setup       func(t *testing.T) (home string, opts target.InstallOpts)
		cancelCtx   bool
		injectFuncs func(t *testing.T, cc *claudecode.ClaudeCode)
		wantErr     string
		check       func(t *testing.T, home string, opts target.InstallOpts)
	}{
		{
			name: "installs plugin with agentpack.yaml",
			setup: func(t *testing.T) (string, target.InstallOpts) {
				t.Helper()
				home := t.TempDir()
				meta := &metadata.Metadata{
					Name:    "my-plugin",
					Version: "1.0.0",
				}
				src := makeSourceDir(t, "my-plugin", "1.0.0", meta, true)
				return home, target.InstallOpts{
					Name:      "my-plugin",
					Version:   "1.0.0",
					SourceDir: src,
					Meta:      meta,
				}
			},
			check: func(t *testing.T, home string, opts target.InstallOpts) {
				t.Helper()
				destDir := filepath.Join(home, ".claude", "plugins", "marketplaces", opts.Name)
				if _, err := os.Stat(destDir); err != nil {
					t.Errorf("destDir not found: %v", err)
				}
				// Descriptors should exist.
				if _, err := os.Stat(filepath.Join(destDir, ".claude-plugin", "marketplace.json")); err != nil {
					t.Errorf("marketplace.json not found: %v", err)
				}
				if _, err := os.Stat(filepath.Join(destDir, ".claude-plugin", "plugin.json")); err != nil {
					t.Errorf("plugin.json not found: %v", err)
				}
			},
		},
		{
			name: "installs plugin without agentpack.yaml (synth fallback)",
			setup: func(t *testing.T) (string, target.InstallOpts) {
				t.Helper()
				home := t.TempDir()
				meta := &metadata.Metadata{
					Name:    "synth-plugin",
					Version: "2.0.0",
				}
				src := makeSourceDir(t, "synth-plugin", "2.0.0", meta, false)
				return home, target.InstallOpts{
					Name:      "synth-plugin",
					Version:   "2.0.0",
					SourceDir: src,
					Meta:      meta,
				}
			},
			check: func(t *testing.T, home string, opts target.InstallOpts) {
				t.Helper()
				destDir := filepath.Join(home, ".claude", "plugins", "marketplaces", opts.Name)
				if _, err := os.Stat(destDir); err != nil {
					t.Errorf("destDir not found: %v", err)
				}
				if _, err := os.Stat(filepath.Join(destDir, ".claude-plugin", "marketplace.json")); err != nil {
					t.Errorf("marketplace.json not found: %v", err)
				}
			},
		},
		{
			name: "reinstall replaces existing installation",
			setup: func(t *testing.T) (string, target.InstallOpts) {
				t.Helper()
				home := t.TempDir()
				// Pre-create a destination to simulate existing install.
				existing := filepath.Join(home, ".claude", "plugins", "marketplaces", "update-plugin")
				if err := os.MkdirAll(existing, 0o755); err != nil {
					t.Fatalf("mkdir existing: %v", err)
				}
				meta := &metadata.Metadata{Name: "update-plugin", Version: "3.0.0"}
				src := makeSourceDir(t, "update-plugin", "3.0.0", meta, true)
				return home, target.InstallOpts{
					Name:      "update-plugin",
					Version:   "3.0.0",
					SourceDir: src,
					Meta:      meta,
				}
			},
			check: func(t *testing.T, home string, opts target.InstallOpts) {
				t.Helper()
				destDir := filepath.Join(home, ".claude", "plugins", "marketplaces", opts.Name)
				if _, err := os.Stat(destDir); err != nil {
					t.Errorf("destDir not found: %v", err)
				}
			},
		},
		{
			name: "returns error when context is already cancelled",
			setup: func(t *testing.T) (string, target.InstallOpts) {
				t.Helper()
				home := t.TempDir()
				return home, target.InstallOpts{Name: "ctx-plugin", SourceDir: t.TempDir()}
			},
			cancelCtx: true,
			wantErr:   "context canceled",
		},
		{
			name: "returns error when home dir lookup fails",
			setup: func(t *testing.T) (string, target.InstallOpts) {
				t.Helper()
				return "", target.InstallOpts{Name: "home-err-plugin", SourceDir: t.TempDir()}
			},
			injectFuncs: func(t *testing.T, cc *claudecode.ClaudeCode) {
				t.Helper()
				claudecode.SetUserHome(cc, func() (string, error) {
					return "", errors.New("no home")
				})
			},
			wantErr: "home dir",
		},
		{
			name: "returns error when osRemoveAll fails",
			setup: func(t *testing.T) (string, target.InstallOpts) {
				t.Helper()
				home := t.TempDir()
				src := makeSourceDir(t, "rmfail-plugin", "1.0.0", nil, false)
				return home, target.InstallOpts{
					Name:      "rmfail-plugin",
					SourceDir: src,
				}
			},
			injectFuncs: func(t *testing.T, cc *claudecode.ClaudeCode) {
				t.Helper()
				claudecode.SetOsRemoveAll(cc, func(_ string) error {
					return errors.New("simulated remove all failure")
				})
			},
			wantErr: "remove existing",
		},
		{
			name: "falls back to copyDir when rename fails",
			setup: func(t *testing.T) (string, target.InstallOpts) {
				t.Helper()
				home := t.TempDir()
				meta := &metadata.Metadata{Name: "copy-plugin", Version: "1.0.0"}
				src := makeSourceDir(t, "copy-plugin", "1.0.0", meta, true)
				return home, target.InstallOpts{
					Name:      "copy-plugin",
					Version:   "1.0.0",
					SourceDir: src,
					Meta:      meta,
				}
			},
			injectFuncs: func(t *testing.T, cc *claudecode.ClaudeCode) {
				t.Helper()
				claudecode.SetRenameFunc(cc, func(_, _ string) error {
					return errors.New("cross-device rename")
				})
			},
			check: func(t *testing.T, home string, opts target.InstallOpts) {
				t.Helper()
				destDir := filepath.Join(home, ".claude", "plugins", "marketplaces", opts.Name)
				if _, err := os.Stat(destDir); err != nil {
					t.Errorf("destDir not found after copy fallback: %v", err)
				}
			},
		},
		{
			name: "returns error when rename and copyDir both fail",
			setup: func(t *testing.T) (string, target.InstallOpts) {
				t.Helper()
				home := t.TempDir()
				src := makeSourceDir(t, "both-fail", "1.0.0", nil, false)
				// Create parent dirs with normal permissions first, then make
				// marketplaces/ read-only so copyDir cannot create a subdir.
				pluginsDir := filepath.Join(home, ".claude", "plugins")
				if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
					t.Fatalf("mkdir pluginsDir: %v", err)
				}
				destParent := filepath.Join(pluginsDir, "marketplaces")
				if err := os.MkdirAll(destParent, 0o755); err != nil {
					t.Fatalf("mkdir destParent: %v", err)
				}
				if err := os.Chmod(destParent, 0o555); err != nil {
					t.Fatalf("chmod: %v", err)
				}
				t.Cleanup(func() { _ = os.Chmod(destParent, 0o755) })
				return home, target.InstallOpts{
					Name:      "both-fail",
					SourceDir: src,
				}
			},
			injectFuncs: func(t *testing.T, cc *claudecode.ClaudeCode) {
				t.Helper()
				claudecode.SetRenameFunc(cc, func(_, _ string) error {
					return errors.New("cross-device rename")
				})
			},
			wantErr: "install",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			homeDir, opts := tt.setup(t)

			cc := claudecode.New()

			if homeDir != "" {
				claudecode.SetUserHome(cc, func() (string, error) { return homeDir, nil })
			}

			if tt.injectFuncs != nil {
				tt.injectFuncs(t, cc)
			}

			var ctx context.Context
			var cancel context.CancelFunc

			if tt.cancelCtx {
				ctx, cancel = context.WithCancel(context.Background())
				cancel()
			} else {
				ctx = context.Background()
				cancel = func() {}
			}
			defer cancel()

			err := cc.Install(ctx, opts)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}

				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.check != nil {
				tt.check(t, homeDir, opts)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestList
// --------------------------------------------------------------------------

func TestList(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setup       func(t *testing.T) string // returns home dir
		wantErr     string
		checkResult func(t *testing.T, plugins []target.InstalledPlugin)
	}{
		{
			name: "returns nil when marketplaces dir does not exist",
			setup: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			checkResult: func(t *testing.T, plugins []target.InstalledPlugin) {
				t.Helper()
				if len(plugins) != 0 {
					t.Errorf("len = %d, want 0", len(plugins))
				}
			},
		},
		{
			name: "returns one entry for a single installed plugin",
			setup: func(t *testing.T) string {
				t.Helper()
				home := t.TempDir()
				writeMeta(t, home, metadata.Metadata{
					Name:           "acme-toolkit",
					Version:        "1.0.0",
					GitCommitSHA:   "a1b2c3d4e5f6789",
					BuildTimestamp: "2026-05-23T10:00:00Z",
				})
				return home
			},
			checkResult: func(t *testing.T, plugins []target.InstalledPlugin) {
				t.Helper()

				if len(plugins) != 1 {
					t.Fatalf("len = %d, want 1", len(plugins))
				}

				p := plugins[0]

				if p.Name != "acme-toolkit" {
					t.Errorf("Name = %q, want %q", p.Name, "acme-toolkit")
				}

				if p.Version != "1.0.0" {
					t.Errorf("Version = %q, want %q", p.Version, "1.0.0")
				}

				if p.SHA != "a1b2c3d" {
					t.Errorf("SHA = %q, want %q", p.SHA, "a1b2c3d")
				}

				if p.Installed != "2026-05-23" {
					t.Errorf("Installed = %q, want %q", p.Installed, "2026-05-23")
				}

				if p.Target != "Claude Code" {
					t.Errorf("Target = %q, want %q", p.Target, "Claude Code")
				}

				if p.Dir == "" {
					t.Error("Dir is empty")
				}
			},
		},
		{
			name: "returns multiple entries sorted by name",
			setup: func(t *testing.T) string {
				t.Helper()
				home := t.TempDir()
				writeMeta(t, home, metadata.Metadata{
					Name: "z-plugin", Version: "1.0.0", GitCommitSHA: "zzzzzzzz",
					BuildTimestamp: "2026-05-23T00:00:00Z",
				})
				writeMeta(t, home, metadata.Metadata{
					Name: "a-plugin", Version: "2.0.0", GitCommitSHA: "aaaaaaa",
					BuildTimestamp: "2026-05-23T00:00:00Z",
				})
				return home
			},
			checkResult: func(t *testing.T, plugins []target.InstalledPlugin) {
				t.Helper()

				if len(plugins) != 2 {
					t.Fatalf("len = %d, want 2", len(plugins))
				}

				if plugins[0].Name != "a-plugin" {
					t.Errorf("plugins[0].Name = %q, want %q", plugins[0].Name, "a-plugin")
				}

				if plugins[1].Name != "z-plugin" {
					t.Errorf("plugins[1].Name = %q, want %q", plugins[1].Name, "z-plugin")
				}
			},
		},
		{
			name: "skips directories without metadata.json",
			setup: func(t *testing.T) string {
				t.Helper()
				home := t.TempDir()
				// A directory without metadata.json.
				nonPlugin := filepath.Join(home, ".claude", "plugins", "marketplaces", "git-plugin")
				if err := os.MkdirAll(nonPlugin, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				// A proper agentpack plugin.
				writeMeta(t, home, metadata.Metadata{
					Name: "real-plugin", Version: "1.0.0", GitCommitSHA: "abc1234",
					BuildTimestamp: "2026-05-23T00:00:00Z",
				})
				return home
			},
			checkResult: func(t *testing.T, plugins []target.InstalledPlugin) {
				t.Helper()

				if len(plugins) != 1 {
					t.Fatalf("len = %d, want 1", len(plugins))
				}

				if plugins[0].Name != "real-plugin" {
					t.Errorf("Name = %q, want %q", plugins[0].Name, "real-plugin")
				}
			},
		},
		{
			name: "returns error for invalid metadata.json",
			setup: func(t *testing.T) string {
				t.Helper()
				home := t.TempDir()
				agentpackDir := filepath.Join(
					home, ".claude", "plugins", "marketplaces", "bad-plugin", ".agentpack",
				)
				if err := os.MkdirAll(agentpackDir, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.WriteFile(
					filepath.Join(agentpackDir, "metadata.json"), []byte("not json"), 0o644,
				); err != nil {
					t.Fatalf("write: %v", err)
				}
				return home
			},
			wantErr: "parse metadata.json",
		},
		{
			name: "returns error when home dir lookup fails during List",
			setup: func(t *testing.T) string {
				t.Helper()
				return "" // signals use of error-returning stub
			},
			wantErr: "home dir",
		},
		{
			name: "returns error when marketplaces dir is unreadable",
			setup: func(t *testing.T) string {
				t.Helper()
				home := t.TempDir()
				pluginsDir := filepath.Join(home, ".claude", "plugins")
				if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
					t.Fatalf("mkdir pluginsDir: %v", err)
				}
				mpDir := filepath.Join(pluginsDir, "marketplaces")
				if err := os.MkdirAll(mpDir, 0o755); err != nil {
					t.Fatalf("mkdir mpDir: %v", err)
				}
				if err := os.Chmod(mpDir, 0o000); err != nil {
					t.Fatalf("chmod: %v", err)
				}
				t.Cleanup(func() { _ = os.Chmod(mpDir, 0o755) })
				return home
			},
			wantErr: "read marketplaces dir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			homeDir := tt.setup(t)
			cc := claudecode.New()

			if homeDir == "" {
				claudecode.SetUserHome(cc, func() (string, error) {
					return "", errors.New("no home")
				})
			} else {
				claudecode.SetUserHome(cc, func() (string, error) { return homeDir, nil })
			}

			plugins, err := cc.List()

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}

				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.checkResult != nil {
				tt.checkResult(t, plugins)
			}
		})
	}
}
