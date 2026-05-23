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

package install_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/avfs/avfs/vfs/osfs"

	"github.com/retr0h/claudia/pkg/build"
	"github.com/retr0h/claudia/pkg/install"
)

// --------------------------------------------------------------------------
// Helpers shared with build_test.go pattern
// --------------------------------------------------------------------------

var gitEnv = []string{
	"GIT_AUTHOR_NAME=Test Author",
	"GIT_AUTHOR_EMAIL=test@example.com",
	"GIT_COMMITTER_NAME=Test Committer",
	"GIT_COMMITTER_EMAIL=test@example.com",
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), gitEnv...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init")
	run("checkout", "-b", "main")

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	run("add", ".")
	run("commit", "-m", "init")
}

// buildTestArchive creates a .claudia archive in dir using the build pipeline
// and returns the path to the archive.
func buildTestArchive(t *testing.T, dir string, manifest string) string {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, "claudia.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write claudia.yaml: %v", err)
	}

	vfs := osfs.NewWithNoIdm()
	results, err := build.Run(context.Background(), vfs, build.Options{Dir: dir})
	if err != nil {
		t.Fatalf("build.Run: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("build.Run returned no results")
	}

	return results[0].ArchivePath
}

// --------------------------------------------------------------------------
// Run
// --------------------------------------------------------------------------

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setup       func(t *testing.T) (archivePath string, pluginDir string)
		cancelCtx   bool
		wantErr     string
		checkResult func(t *testing.T, r *install.Result, pluginDir string)
	}{
		{
			name: "installs archive to plugin dir",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: test-plugin
version: "1.0.0"
description: A test plugin
`)
				pluginDir := t.TempDir()
				return archivePath, pluginDir
			},
			checkResult: func(t *testing.T, r *install.Result, _ string) {
				t.Helper()
				if r.Name != "test-plugin" {
					t.Errorf("Name = %q, want %q", r.Name, "test-plugin")
				}
				if r.Version != "1.0.0" {
					t.Errorf("Version = %q, want %q", r.Version, "1.0.0")
				}
				if r.SHA == "" {
					t.Error("SHA is empty")
				}
				if r.Dir == "" {
					t.Error("Dir is empty")
				}
				// Check that the plugin directory was actually created.
				if _, err := os.Stat(r.Dir); err != nil {
					t.Errorf("plugin dir not found: %v", err)
				}
				// Verify metadata.json exists in the installed dir.
				metaPath := filepath.Join(r.Dir, ".claudia", "metadata.json")
				if _, err := os.Stat(metaPath); err != nil {
					t.Errorf("metadata.json not found: %v", err)
				}
			},
		},
		{
			name: "reinstalling replaces existing plugin",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: my-plugin
version: "2.0.0"
description: Updated plugin
`)
				pluginDir := t.TempDir()
				// Pre-create the destination to simulate an existing install.
				existing := filepath.Join(pluginDir, "marketplaces", "my-plugin")
				if err := os.MkdirAll(existing, 0o755); err != nil {
					t.Fatalf("mkdir existing: %v", err)
				}
				return archivePath, pluginDir
			},
			checkResult: func(t *testing.T, r *install.Result, _ string) {
				t.Helper()
				if r.Name != "my-plugin" {
					t.Errorf("Name = %q, want %q", r.Name, "my-plugin")
				}
				if r.Version != "2.0.0" {
					t.Errorf("Version = %q, want %q", r.Version, "2.0.0")
				}
			},
		},
		{
			name: "returns error when archive does not exist",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return "/nonexistent/path.claudia", t.TempDir()
			},
			wantErr: "fetch",
		},
		{
			name: "returns error when context is cancelled",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: cancel-plugin
version: "1.0.0"
description: Plugin for cancel test
`)
				return archivePath, t.TempDir()
			},
			cancelCtx: true,
			wantErr:   "context canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			archivePath, pluginDir := tt.setup(t)

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

			r, err := install.Run(ctx, install.Options{
				Source:    archivePath,
				PluginDir: pluginDir,
			})

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
				tt.checkResult(t, r, pluginDir)
			}
		})
	}
}
