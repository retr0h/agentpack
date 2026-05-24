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
		{
			name: "returns error when mkdirAll for plugin parent fails",
			setup: func(t *testing.T) (string, target.InstallOpts) {
				t.Helper()
				home := t.TempDir()
				src := makeSourceDir(t, "mkdir-fail", "1.0.0", nil, false)
				return home, target.InstallOpts{
					Name:      "mkdir-fail",
					SourceDir: src,
				}
			},
			injectFuncs: func(t *testing.T, cc *claudecode.ClaudeCode) {
				t.Helper()
				claudecode.SetOsMkdirAll(cc, func(_ string, _ os.FileMode) error {
					return errors.New("simulated mkdirAll failure")
				})
			},
			wantErr: "mkdir plugin dir",
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
			name: "skips non-directory entries in marketplaces dir",
			setup: func(t *testing.T) string {
				t.Helper()
				home := t.TempDir()
				mpDir := filepath.Join(home, ".claude", "plugins", "marketplaces")
				if err := os.MkdirAll(mpDir, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				// Place a regular file alongside subdirectory entries.
				if err := os.WriteFile(filepath.Join(mpDir, "stray-file.txt"), []byte("hello"), 0o644); err != nil {
					t.Fatalf("write: %v", err)
				}
				// Also add a proper plugin so we confirm only the dir is counted.
				writeMeta(t, home, metadata.Metadata{
					Name: "real-plugin", Version: "1.0.0", GitCommitSHA: "abc1234",
					BuildTimestamp: "2026-05-23T00:00:00Z",
				})
				return home
			},
			checkResult: func(t *testing.T, plugins []target.InstalledPlugin) {
				t.Helper()

				if len(plugins) != 1 {
					t.Fatalf("len = %d, want 1 (stray file must be skipped)", len(plugins))
				}

				if plugins[0].Name != "real-plugin" {
					t.Errorf("Name = %q, want %q", plugins[0].Name, "real-plugin")
				}
			},
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

// --------------------------------------------------------------------------
// TestWriteDescriptors
// --------------------------------------------------------------------------

func TestWriteDescriptors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (destDir string, opts target.InstallOpts)
		wantErr string
		check   func(t *testing.T, destDir string)
	}{
		{
			name: "writes marketplace.json and plugin.json",
			setup: func(t *testing.T) (string, target.InstallOpts) {
				t.Helper()
				destDir := makeSourceDir(t, "ok-plugin", "1.0.0", nil, true)
				return destDir, target.InstallOpts{Name: "ok-plugin", Version: "1.0.0"}
			},
			check: func(t *testing.T, destDir string) {
				t.Helper()
				if _, err := os.Stat(filepath.Join(destDir, ".claude-plugin", "marketplace.json")); err != nil {
					t.Errorf("marketplace.json missing: %v", err)
				}
				if _, err := os.Stat(filepath.Join(destDir, ".claude-plugin", "plugin.json")); err != nil {
					t.Errorf("plugin.json missing: %v", err)
				}
			},
		},
		{
			name: "returns error when mkdir .claude-plugin fails",
			setup: func(t *testing.T) (string, target.InstallOpts) {
				t.Helper()
				// Make destDir a read-only directory so os.MkdirAll(.claude-plugin) fails.
				destDir := t.TempDir()
				if err := os.Chmod(destDir, 0o555); err != nil {
					t.Fatalf("chmod: %v", err)
				}
				t.Cleanup(func() { _ = os.Chmod(destDir, 0o755) })
				return destDir, target.InstallOpts{Name: "mkdir-fail", Version: "1.0.0"}
			},
			wantErr: "mkdir .claude-plugin",
		},
		{
			name: "returns error when writing marketplace.json fails",
			setup: func(t *testing.T) (string, target.InstallOpts) {
				t.Helper()
				destDir := t.TempDir()
				// Create .claude-plugin as a read-only dir so WriteFile fails.
				descDir := filepath.Join(destDir, ".claude-plugin")
				if err := os.MkdirAll(descDir, 0o555); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				t.Cleanup(func() { _ = os.Chmod(descDir, 0o755) })
				return destDir, target.InstallOpts{Name: "mktplace-fail", Version: "1.0.0"}
			},
			wantErr: "write marketplace.json",
		},
		{
			name: "returns error when writing plugin.json fails",
			setup: func(t *testing.T) (string, target.InstallOpts) {
				t.Helper()
				destDir := t.TempDir()
				// Create .claude-plugin dir with write access so marketplace.json
				// can be written successfully. Pre-create plugin.json as a
				// read-only file so os.WriteFile for plugin.json fails.
				descDir := filepath.Join(destDir, ".claude-plugin")
				if err := os.MkdirAll(descDir, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				// Pre-create plugin.json as read-only; WriteFile will fail on it.
				pluginJSONPath := filepath.Join(descDir, "plugin.json")
				if err := os.WriteFile(pluginJSONPath, []byte("{}"), 0o444); err != nil {
					t.Fatalf("pre-write plugin.json: %v", err)
				}
				t.Cleanup(func() { _ = os.Chmod(pluginJSONPath, 0o644) })
				return destDir, target.InstallOpts{Name: "pluginjson-fail", Version: "1.0.0"}
			},
			wantErr: "write plugin.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			destDir, opts := tt.setup(t)
			cc := claudecode.New()

			err := claudecode.WriteDescriptors(cc, destDir, opts)

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
				tt.check(t, destDir)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestReadManifestPlugin
// --------------------------------------------------------------------------

func TestReadManifestPlugin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setup       func(t *testing.T) (dir string, opts target.InstallOpts)
		wantErr     string
		checkResult func(t *testing.T, p interface{})
	}{
		{
			name: "reads valid agentpack.yaml and returns plugin",
			setup: func(t *testing.T) (string, target.InstallOpts) {
				t.Helper()
				dir := makeSourceDir(t, "valid-plugin", "1.0.0", nil, true)
				return dir, target.InstallOpts{Name: "valid-plugin", Version: "1.0.0"}
			},
		},
		{
			name: "returns error when agentpack.yaml missing",
			setup: func(t *testing.T) (string, target.InstallOpts) {
				t.Helper()
				// makeSourceDir with addYAML=false writes no agentpack.yaml.
				dir := makeSourceDir(t, "no-yaml-plugin", "1.0.0", nil, false)
				return dir, target.InstallOpts{Name: "no-yaml-plugin", Version: "1.0.0"}
			},
			wantErr: "read agentpack.yaml",
		},
		{
			name: "returns error when agentpack.yaml contains invalid YAML",
			setup: func(t *testing.T) (string, target.InstallOpts) {
				t.Helper()
				dir := t.TempDir()
				agentpackDir := filepath.Join(dir, ".agentpack")
				if err := os.MkdirAll(agentpackDir, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				// Write malformed YAML (tabs are invalid in YAML keys).
				bad := "name:\t: invalid\n"
				if err := os.WriteFile(filepath.Join(agentpackDir, "agentpack.yaml"), []byte(bad), 0o644); err != nil {
					t.Fatalf("write: %v", err)
				}
				return dir, target.InstallOpts{Name: "bad-yaml", Version: "1.0.0"}
			},
			wantErr: "parse agentpack.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir, opts := tt.setup(t)

			_, err := claudecode.ReadManifestPlugin(dir, opts)

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
		})
	}
}

// --------------------------------------------------------------------------
// TestSynthPlugin
// --------------------------------------------------------------------------

func TestSynthPlugin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		opts        target.InstallOpts
		checkResult func(t *testing.T, p interface{ GetName() string })
	}{
		{
			name: "uses opts Name and Version when set",
			opts: target.InstallOpts{Name: "direct-name", Version: "2.0.0"},
		},
		{
			name: "falls back to Meta.Name when opts.Name is empty",
			opts: target.InstallOpts{
				Name:    "",
				Version: "3.0.0",
				Meta:    &metadata.Metadata{Name: "meta-name", Version: "9.0.0"},
			},
		},
		{
			name: "falls back to Meta.Version when opts.Version is empty",
			opts: target.InstallOpts{
				Name:    "some-name",
				Version: "",
				Meta:    &metadata.Metadata{Name: "meta-name", Version: "5.0.0"},
			},
		},
		{
			name: "Meta nil does not panic",
			opts: target.InstallOpts{Name: "no-meta", Version: "1.0.0", Meta: nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := claudecode.SynthPlugin(tt.opts)

			// Verify the returned plugin is consistent.
			expectedName := tt.opts.Name
			if expectedName == "" && tt.opts.Meta != nil {
				expectedName = tt.opts.Meta.Name
			}

			expectedVersion := tt.opts.Version
			if expectedVersion == "" && tt.opts.Meta != nil {
				expectedVersion = tt.opts.Meta.Version
			}

			if p.Name != expectedName {
				t.Errorf("Name = %q, want %q", p.Name, expectedName)
			}

			if p.Version != expectedVersion {
				t.Errorf("Version = %q, want %q", p.Version, expectedVersion)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestCollectCommandPaths
// --------------------------------------------------------------------------

func TestCollectCommandPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setup       func(t *testing.T) string // returns dir
		checkResult func(t *testing.T, paths []string)
	}{
		{
			name: "returns nil when commands dir does not exist",
			setup: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			checkResult: func(t *testing.T, paths []string) {
				t.Helper()
				if len(paths) != 0 {
					t.Errorf("len = %d, want 0", len(paths))
				}
			},
		},
		{
			name: "returns file paths skipping subdirectories",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				commandsDir := filepath.Join(dir, "commands")
				if err := os.MkdirAll(commandsDir, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				// Add a regular file.
				if err := os.WriteFile(filepath.Join(commandsDir, "cmd-one.md"), []byte("# cmd"), 0o644); err != nil {
					t.Fatalf("write file: %v", err)
				}
				// Add a subdirectory (should be skipped).
				if err := os.MkdirAll(filepath.Join(commandsDir, "subdir"), 0o755); err != nil {
					t.Fatalf("mkdir subdir: %v", err)
				}
				return dir
			},
			checkResult: func(t *testing.T, paths []string) {
				t.Helper()
				if len(paths) != 1 {
					t.Fatalf("len = %d, want 1", len(paths))
				}
				if !strings.HasSuffix(paths[0], "cmd-one.md") {
					t.Errorf("path %q does not end with cmd-one.md", paths[0])
				}
			},
		},
		{
			name: "returns empty slice when commands dir is empty",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				commandsDir := filepath.Join(dir, "commands")
				if err := os.MkdirAll(commandsDir, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				return dir
			},
			checkResult: func(t *testing.T, paths []string) {
				t.Helper()
				if len(paths) != 0 {
					t.Errorf("len = %d, want 0", len(paths))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := tt.setup(t)
			paths := claudecode.CollectCommandPaths(dir)

			if tt.checkResult != nil {
				tt.checkResult(t, paths)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestCopyDir
// --------------------------------------------------------------------------

func TestCopyDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (src, dst string)
		ctx     func() context.Context
		wantErr string
		check   func(t *testing.T, dst string)
	}{
		{
			name: "copies directory tree successfully",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				subdir := filepath.Join(src, "sub")
				if err := os.MkdirAll(subdir, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.WriteFile(filepath.Join(src, "file.txt"), []byte("hello"), 0o644); err != nil {
					t.Fatalf("write: %v", err)
				}
				if err := os.WriteFile(filepath.Join(subdir, "nested.txt"), []byte("world"), 0o644); err != nil {
					t.Fatalf("write nested: %v", err)
				}
				return src, t.TempDir()
			},
			ctx: func() context.Context { return context.Background() },
			check: func(t *testing.T, dst string) {
				t.Helper()
				if _, err := os.Stat(filepath.Join(dst, "file.txt")); err != nil {
					t.Errorf("file.txt missing: %v", err)
				}
				if _, err := os.Stat(filepath.Join(dst, "sub", "nested.txt")); err != nil {
					t.Errorf("nested.txt missing: %v", err)
				}
			},
		},
		{
			name: "returns error when src does not exist",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return filepath.Join(t.TempDir(), "nonexistent"), t.TempDir()
			},
			ctx:     func() context.Context { return context.Background() },
			wantErr: "nonexistent",
		},
		{
			name: "returns error when context is cancelled",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				if err := os.WriteFile(filepath.Join(src, "file.txt"), []byte("data"), 0o644); err != nil {
					t.Fatalf("write: %v", err)
				}
				return src, t.TempDir()
			},
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			wantErr: "context canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src, dst := tt.setup(t)
			ctx := tt.ctx()

			err := claudecode.CopyDir(ctx, src, dst)

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
				tt.check(t, dst)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestCopyFile
// --------------------------------------------------------------------------

func TestCopyFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (src, dst string)
		wantErr string
		check   func(t *testing.T, dst string)
	}{
		{
			name: "copies file successfully",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := filepath.Join(t.TempDir(), "src.txt")
				if err := os.WriteFile(src, []byte("content"), 0o644); err != nil {
					t.Fatalf("write: %v", err)
				}
				return src, filepath.Join(t.TempDir(), "dst.txt")
			},
			check: func(t *testing.T, dst string) {
				t.Helper()
				data, err := os.ReadFile(dst)
				if err != nil {
					t.Fatalf("read dst: %v", err)
				}
				if string(data) != "content" {
					t.Errorf("content = %q, want %q", string(data), "content")
				}
			},
		},
		{
			name: "returns error when src does not exist",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return filepath.Join(t.TempDir(), "missing.txt"), filepath.Join(t.TempDir(), "dst.txt")
			},
			wantErr: "read",
		},
		{
			name: "returns error when dst directory is not writable",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				srcDir := t.TempDir()
				src := filepath.Join(srcDir, "src.txt")
				if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
					t.Fatalf("write src: %v", err)
				}
				// Make a read-only destination directory.
				dstDir := t.TempDir()
				if err := os.Chmod(dstDir, 0o555); err != nil {
					t.Fatalf("chmod: %v", err)
				}
				t.Cleanup(func() { _ = os.Chmod(dstDir, 0o755) })
				return src, filepath.Join(dstDir, "dst.txt")
			},
			wantErr: "write",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src, dst := tt.setup(t)

			err := claudecode.CopyFile(src, dst)

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
				tt.check(t, dst)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestShortSHA
// --------------------------------------------------------------------------

func TestShortSHA(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		sha  string
		want string
	}{
		{
			name: "truncates SHA longer than 7 characters",
			sha:  "a1b2c3d4e5f6789",
			want: "a1b2c3d",
		},
		{
			name: "returns SHA as-is when shorter than 7 characters",
			sha:  "abc",
			want: "abc",
		},
		{
			name: "returns SHA as-is when exactly 7 characters",
			sha:  "a1b2c3d",
			want: "a1b2c3d",
		},
		{
			name: "returns empty string for empty SHA",
			sha:  "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := claudecode.ShortSHA(tt.sha)
			if got != tt.want {
				t.Errorf("ShortSHA(%q) = %q, want %q", tt.sha, got, tt.want)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestFormatDate
// --------------------------------------------------------------------------

func TestFormatDate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ts   string
		want string
	}{
		{
			name: "extracts date from RFC3339 timestamp",
			ts:   "2026-05-23T10:00:00Z",
			want: "2026-05-23",
		},
		{
			name: "returns string as-is when no T separator",
			ts:   "2026-05-23",
			want: "2026-05-23",
		},
		{
			name: "returns empty string for empty input",
			ts:   "",
			want: "",
		},
		{
			name: "returns string as-is when T is at index 0",
			ts:   "Tinvalid",
			want: "Tinvalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := claudecode.FormatDate(tt.ts)
			if got != tt.want {
				t.Errorf("FormatDate(%q) = %q, want %q", tt.ts, got, tt.want)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestInstallCtxCancelledAfterMove
// --------------------------------------------------------------------------

// TestInstallCtxCancelledAfterMove covers the ctx.Err() guard that fires
// between the rename/copyDir step and writeDescriptors in Install. The rename
// function cancels the context and then performs the real rename so the move
// succeeds; the subsequent ctx.Err() check returns the cancellation error.
func TestInstallCtxCancelledAfterMove(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	meta := &metadata.Metadata{Name: "ctx-after-move", Version: "1.0.0"}
	src := makeSourceDir(t, "ctx-after-move", "1.0.0", meta, true)

	ctx, cancel := context.WithCancel(context.Background())

	cc := claudecode.New()
	claudecode.SetUserHome(cc, func() (string, error) { return home, nil })
	claudecode.SetRenameFunc(cc, func(oldpath, newpath string) error {
		cancel() // cancel the context so the post-move ctx.Err() fires
		return os.Rename(oldpath, newpath)
	})

	opts := target.InstallOpts{
		Name:      "ctx-after-move",
		Version:   "1.0.0",
		SourceDir: src,
		Meta:      meta,
	}

	err := cc.Install(ctx, opts)
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}

	if !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("error %q does not contain %q", err.Error(), "context canceled")
	}
}

// --------------------------------------------------------------------------
// TestCopyDirWalkError
// --------------------------------------------------------------------------

// TestCopyDirWalkError covers the WalkDir error-callback path in copyDir,
// which fires when WalkDir passes a non-nil err to the callback for a path
// it could not stat (e.g., a directory whose permissions were removed after
// walking started).
func TestCopyDirWalkError(t *testing.T) {
	t.Parallel()

	src := t.TempDir()

	// Create a subdirectory with a file inside, then remove read permission
	// from the subdirectory so WalkDir cannot descend into it.
	unreadable := filepath.Join(src, "unreadable")
	if err := os.MkdirAll(unreadable, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(unreadable, "secret.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o755) })

	dst := t.TempDir()

	err := claudecode.CopyDir(context.Background(), src, dst)
	if err == nil {
		t.Fatal("expected error from unreadable subdir, got nil")
	}
}
