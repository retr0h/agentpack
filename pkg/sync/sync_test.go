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

package sync_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/avfs/avfs/vfs/osfs"

	"github.com/retr0h/claudia/pkg/build"
	pkgsync "github.com/retr0h/claudia/pkg/sync"
)

// --------------------------------------------------------------------------
// Custom context for triggering loop-level cancellation
// --------------------------------------------------------------------------

// cancelAfterFirstErrCtx is a context.Context whose Err() returns nil on the
// first call and context.Canceled on all subsequent calls. This lets us pass
// the function-entry check (line 59 in sync.go) but fail the loop-level check
// (line 75).
type cancelAfterFirstErrCtx struct {
	callCount int
}

func newCancelAfterFirstErrCtx() *cancelAfterFirstErrCtx {
	return &cancelAfterFirstErrCtx{}
}

func (c *cancelAfterFirstErrCtx) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *cancelAfterFirstErrCtx) Done() <-chan struct{}       { return nil }
func (c *cancelAfterFirstErrCtx) Value(_ any) any             { return nil }

func (c *cancelAfterFirstErrCtx) Err() error {
	c.callCount++
	if c.callCount == 1 {
		return nil
	}
	return errors.New("context canceled")
}

// --------------------------------------------------------------------------
// Helpers
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

func writePackagesFile(t *testing.T, dir string, content string) string {
	t.Helper()
	path := filepath.Join(dir, "claudia-packages.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write claudia-packages.yaml: %v", err)
	}
	return path
}

// --------------------------------------------------------------------------
// Run
// --------------------------------------------------------------------------

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setup       func(t *testing.T) (configPath string, pluginDir string)
		cancelCtx   bool
		customCtx   context.Context
		wantErr     string
		checkResult func(t *testing.T, results []pkgsync.Result)
	}{
		{
			name: "installs all packages from config",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				srcDir := t.TempDir()
				initGitRepo(t, srcDir)
				archivePath := buildTestArchive(t, srcDir, `
name: sync-plugin
version: "1.0.0"
description: Plugin for sync test
`)
				cfgDir := t.TempDir()
				pluginDir := t.TempDir()
				configPath := writePackagesFile(t, cfgDir, "packages:\n  - name: sync-plugin\n    source: "+archivePath+"\n")
				return configPath, pluginDir
			},
			checkResult: func(t *testing.T, results []pkgsync.Result) {
				t.Helper()
				if len(results) != 1 {
					t.Fatalf("result count = %d, want 1", len(results))
				}
				if results[0].Status != "installed" {
					t.Errorf("Status = %q, want %q", results[0].Status, "installed")
				}
				if results[0].Name != "sync-plugin" {
					t.Errorf("Name = %q, want %q", results[0].Name, "sync-plugin")
				}
			},
		},
		{
			name: "records failed status for bad source",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				cfgDir := t.TempDir()
				pluginDir := t.TempDir()
				configPath := writePackagesFile(t, cfgDir,
					"packages:\n  - name: bad-plugin\n    source: /nonexistent/bad.claudia\n")
				return configPath, pluginDir
			},
			checkResult: func(t *testing.T, results []pkgsync.Result) {
				t.Helper()
				if len(results) != 1 {
					t.Fatalf("result count = %d, want 1", len(results))
				}
				if results[0].Status != "failed" {
					t.Errorf("Status = %q, want %q", results[0].Status, "failed")
				}
				if results[0].Err == nil {
					t.Error("Err is nil, want non-nil")
				}
			},
		},
		{
			name: "returns error when config file missing",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return "/nonexistent/claudia-packages.yaml", t.TempDir()
			},
			wantErr: "read",
		},
		{
			name: "returns error when config file has invalid YAML",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				cfgDir := t.TempDir()
				// An unclosed bracket causes a real YAML parse error.
				configPath := writePackagesFile(t, cfgDir, "packages:\n  - name: [unclosed\n")
				return configPath, t.TempDir()
			},
			wantErr: "parse",
		},
		{
			name: "returns error when context is cancelled",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				cfgDir := t.TempDir()
				configPath := writePackagesFile(t, cfgDir, "packages:\n  - name: p\n    source: /tmp/x.claudia\n")
				return configPath, t.TempDir()
			},
			cancelCtx: true,
			wantErr:   "context canceled",
		},
		{
			name: "returns error when context cancelled inside loop",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				cfgDir := t.TempDir()
				// Two packages so the loop runs at least once before cancelling.
				configPath := writePackagesFile(t, cfgDir,
					"packages:\n  - name: a\n    source: /tmp/a.claudia\n  - name: b\n    source: /tmp/b.claudia\n",
				)
				return configPath, t.TempDir()
			},
			customCtx: newCancelAfterFirstErrCtx(),
			wantErr:   "context canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			configPath, pluginDir := tt.setup(t)

			var ctx context.Context
			var cancel context.CancelFunc

			switch {
			case tt.customCtx != nil:
				ctx = tt.customCtx
				cancel = func() {}
			case tt.cancelCtx:
				ctx, cancel = context.WithCancel(context.Background())
				cancel()
			default:
				ctx = context.Background()
				cancel = func() {}
			}
			defer cancel()

			results, err := pkgsync.Run(ctx, configPath, pluginDir)

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
				tt.checkResult(t, results)
			}
		})
	}
}
