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

package build_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/avfs/avfs"
	"github.com/avfs/avfs/vfs/memfs"
	"github.com/avfs/avfs/vfs/osfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/internal/build"
	"github.com/retr0h/agentpack/internal/manifest"
	"github.com/retr0h/agentpack/internal/metadata"
)

// --------------------------------------------------------------------------
// Error-injecting VFS helpers
// --------------------------------------------------------------------------

// statErrorForPathVFS wraps a VFS and returns an error from Stat only when the
// path has the given suffix.
type statErrorForPathVFS struct {
	avfs.VFS
	suffix string
}

func (v statErrorForPathVFS) Stat(name string) (fs.FileInfo, error) {
	if strings.HasSuffix(name, v.suffix) {
		return nil, errors.New("simulated stat error")
	}
	return v.VFS.Stat(name)
}

// openErrorForPathVFS wraps a VFS and returns an error from Open only when the
// path has the given suffix.
type openErrorForPathVFS struct {
	avfs.VFS
	suffix string
}

func (v openErrorForPathVFS) Open(name string) (avfs.File, error) {
	if strings.HasSuffix(name, v.suffix) {
		return nil, errors.New("simulated open error")
	}
	return v.VFS.Open(name)
}

// createErrorForPathVFS wraps a VFS and returns an error from Create only when
// the path has the given suffix.
type createErrorForPathVFS struct {
	avfs.VFS
	suffix string
}

func (v createErrorForPathVFS) Create(name string) (avfs.File, error) {
	if strings.HasSuffix(name, v.suffix) {
		return nil, errors.New("simulated create error")
	}
	return v.VFS.Create(name)
}

// statAlwaysErrorVFS returns an error from every Stat call.
type statAlwaysErrorVFS struct {
	avfs.VFS
}

func (statAlwaysErrorVFS) Stat(string) (fs.FileInfo, error) {
	return nil, errors.New("simulated stat error")
}

// errOnlyContext is a context whose Done() is nil (so exec.CommandContext is
// unaffected) but whose Err() always returns an error. This lets tests cover
// ctx.Err() guard checks without interfering with subprocess execution.
type errOnlyContext struct{}

func (errOnlyContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (errOnlyContext) Done() <-chan struct{}       { return nil }
func (errOnlyContext) Err() error                  { return errors.New("context canceled") }
func (errOnlyContext) Value(_ any) any             { return nil }

// --------------------------------------------------------------------------
// Git repo helpers
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
			require.NoError(t, err, "git %v\n%s", args, out)
		}
	}

	run("init")
	run("checkout", "-b", "main")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644))

	run("add", ".")
	run("commit", "-m", "init")
}

// writeManifest writes content as agentpack.yaml in dir.
func writeManifest(t *testing.T, dir, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "agentpack.yaml"), []byte(content), 0o644))
}

// --------------------------------------------------------------------------
// Run
// --------------------------------------------------------------------------

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setup       func(t *testing.T) (dir string, cleanup func())
		opts        func(dir string) build.Options
		wantErr     string
		useErrOnly  bool // use errOnlyContext (Done=nil, Err=error) instead of cancelled ctx
		checkResult func(t *testing.T, results []build.Result)
	}{
		{
			name: "builds single plugin",
			setup: func(t *testing.T) (string, func()) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				writeManifest(t, dir, `
name: my-plugin
version: "1.0.0"
description: A test plugin
`)
				return dir, func() {}
			},
			opts: func(dir string) build.Options {
				return build.Options{Dir: dir}
			},
			checkResult: func(t *testing.T, results []build.Result) {
				t.Helper()
				require.Len(t, results, 1)
				r := results[0]
				assert.Equal(t, "my-plugin", r.Name)
				assert.Equal(t, "1.0.0", r.Version)
				assert.NotEmpty(t, r.SHA256)
				assert.Len(t, r.SHA256, 64)
				assert.NotZero(t, r.Size)
				assert.True(t, strings.HasSuffix(r.ArchivePath, "my-plugin-1.0.0.agentpack"))
			},
		},
		{
			name: "builds multi-plugin manifest",
			setup: func(t *testing.T) (string, func()) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				writeManifest(t, dir, `
plugins:
  - name: plugin-a
    version: "1.0.0"
    description: First plugin
  - name: plugin-b
    version: "2.0.0"
    description: Second plugin
`)
				return dir, func() {}
			},
			opts: func(dir string) build.Options {
				return build.Options{Dir: dir}
			},
			checkResult: func(t *testing.T, results []build.Result) {
				t.Helper()
				require.Len(t, results, 2)
				names := map[string]bool{}
				for _, r := range results {
					names[r.Name] = true
				}
				assert.True(t, names["plugin-a"] && names["plugin-b"])
			},
		},
		{
			name: "filters to named plugins",
			setup: func(t *testing.T) (string, func()) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				writeManifest(t, dir, `
plugins:
  - name: plugin-a
    version: "1.0.0"
    description: First plugin
  - name: plugin-b
    version: "2.0.0"
    description: Second plugin
`)
				return dir, func() {}
			},
			opts: func(dir string) build.Options {
				return build.Options{Dir: dir, Names: []string{"plugin-b"}}
			},
			checkResult: func(t *testing.T, results []build.Result) {
				t.Helper()
				require.Len(t, results, 1)
				assert.Equal(t, "plugin-b", results[0].Name)
			},
		},
		{
			name: "fails with unknown plugin name",
			setup: func(t *testing.T) (string, func()) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				writeManifest(t, dir, `
name: my-plugin
version: "1.0.0"
description: A test plugin
`)
				return dir, func() {}
			},
			opts: func(dir string) build.Options {
				return build.Options{Dir: dir, Names: []string{"nonexistent"}}
			},
			wantErr: "nonexistent",
		},
		{
			name: "succeeds without git repo using fallback metadata",
			setup: func(t *testing.T) (string, func()) {
				t.Helper()
				dir := t.TempDir()
				writeManifest(t, dir, `
name: my-plugin
version: "1.0.0"
description: A test plugin
`)
				return dir, func() {}
			},
			opts: func(dir string) build.Options {
				return build.Options{Dir: dir}
			},
		},
		{
			name: "fails when agentpack.yaml missing",
			setup: func(t *testing.T) (string, func()) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				return dir, func() {}
			},
			opts: func(dir string) build.Options {
				return build.Options{Dir: dir}
			},
			wantErr: "agentpack.yaml not found",
		},
		{
			name: "respects context cancellation",
			setup: func(t *testing.T) (string, func()) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				writeManifest(t, dir, `
plugins:
  - name: plugin-a
    version: "1.0.0"
    description: First plugin
  - name: plugin-b
    version: "2.0.0"
    description: Second plugin
`)
				return dir, func() {}
			},
			opts: func(dir string) build.Options {
				return build.Options{Dir: dir}
			},
			wantErr: "context canceled",
		},
		{
			name: "creates sha256 sidecar alongside archive",
			setup: func(t *testing.T) (string, func()) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				writeManifest(t, dir, `
name: sidecar-plugin
version: "1.0.0"
description: A sidecar test plugin
`)
				return dir, func() {}
			},
			opts: func(dir string) build.Options {
				return build.Options{Dir: dir}
			},
			checkResult: func(t *testing.T, results []build.Result) {
				t.Helper()
				require.Len(t, results, 1)
				r := results[0]
				sidecarPath := r.ArchivePath + ".sha256"
				data, err := os.ReadFile(sidecarPath)
				require.NoError(t, err)
				content := strings.TrimSpace(string(data))
				assert.Equal(t, r.SHA256, content)
				assert.Len(t, content, 64)
			},
		},
		{
			name: "builds plugin with skills",
			setup: func(t *testing.T) (string, func()) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)

				skillDir := filepath.Join(dir, "skills")
				require.NoError(t, os.MkdirAll(skillDir, 0o755))
				require.NoError(
					t,
					os.WriteFile(
						filepath.Join(skillDir, "intro.md"),
						[]byte("# Intro"),
						fs.FileMode(0o644),
					),
				)

				writeManifest(t, dir, `
name: skill-plugin
version: "1.0.0"
description: Plugin with skills
skills:
  - skills/*.md
`)
				return dir, func() {}
			},
			opts: func(dir string) build.Options {
				return build.Options{Dir: dir}
			},
			checkResult: func(t *testing.T, results []build.Result) {
				t.Helper()
				require.Len(t, results, 1)
				assert.NotZero(t, results[0].FileCount)
			},
		},
		{
			name: "builds plugin with remote mcp server",
			setup: func(t *testing.T) (string, func()) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)

				writeManifest(t, dir, `
name: mcp-remote-plugin
version: "1.0.0"
description: Plugin with remote MCP
mcp:
  - type: remote
    name: my-remote
    url: https://mcp.example.com/v1
`)
				return dir, func() {}
			},
			opts: func(dir string) build.Options {
				return build.Options{Dir: dir}
			},
			checkResult: func(t *testing.T, results []build.Result) {
				t.Helper()
				require.Len(t, results, 1)
				assert.Equal(t, "mcp-remote-plugin", results[0].Name)
			},
		},
		{
			name: "builds plugin with ux mcp server",
			setup: func(t *testing.T) (string, func()) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)

				writeManifest(t, dir, `
name: mcp-ux-plugin
version: "1.0.0"
description: Plugin with ux MCP
mcp:
  - type: ux
    name: my-ux
    package: "@mycompany/my-server"
`)
				return dir, func() {}
			},
			opts: func(dir string) build.Options {
				return build.Options{Dir: dir}
			},
			checkResult: func(t *testing.T, results []build.Result) {
				t.Helper()
				require.Len(t, results, 1)
				assert.Equal(t, "mcp-ux-plugin", results[0].Name)
			},
		},
		{
			name: "builds plugin with config-based mcp server",
			setup: func(t *testing.T) (string, func()) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)

				// Write a .mcp.json config file.
				mcpContent := `{"mcpServers":{"my-srv":{"url":"https://example.com"}}}`
				require.NoError(
					t,
					os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(mcpContent), 0o644),
				)

				writeManifest(t, dir, `
name: mcp-config-plugin
version: "1.0.0"
description: Plugin with config MCP
mcp:
  - type: remote
    name: my-srv
    config: .mcp.json
`)
				return dir, func() {}
			},
			opts: func(dir string) build.Options {
				return build.Options{Dir: dir}
			},
			checkResult: func(t *testing.T, results []build.Result) {
				t.Helper()
				require.Len(t, results, 1)
				assert.Equal(t, "mcp-config-plugin", results[0].Name)
			},
		},
		{
			name: "fails when mcp config file is missing",
			setup: func(t *testing.T) (string, func()) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)

				writeManifest(t, dir, `
name: mcp-missing-config
version: "1.0.0"
description: Plugin with missing config
mcp:
  - type: remote
    name: my-srv
    config: nonexistent/.mcp.json
`)
				return dir, func() {}
			},
			opts: func(dir string) build.Options {
				return build.Options{Dir: dir}
			},
			wantErr: "mcp config not found",
		},
		{
			name: "fails when ctx.Err fires inside plugins loop",
			setup: func(t *testing.T) (string, func()) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				writeManifest(t, dir, `
plugins:
  - name: loop-cancel-a
    version: "1.0.0"
    description: First plugin
  - name: loop-cancel-b
    version: "2.0.0"
    description: Second plugin
`)
				return dir, func() {}
			},
			opts: func(dir string) build.Options {
				return build.Options{Dir: dir}
			},
			useErrOnly: true,
			wantErr:    "context canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir, cleanup := tt.setup(t)
			defer cleanup()

			opts := tt.opts(dir)

			var ctx context.Context
			var cancel context.CancelFunc

			// Special case: errOnlyContext has nil Done() so exec.CommandContext
			// is unaffected, but Err() always returns an error so ctx.Err()
			// guards in the plugins loop fire immediately.
			if tt.useErrOnly {
				ctx = errOnlyContext{}
				cancel = func() {}
			} else if tt.wantErr == "context canceled" {
				// Pre-cancel so metadata.Capture (git) fails immediately.
				ctx, cancel = context.WithCancel(context.Background())
				cancel()
			} else {
				ctx = context.Background()
				cancel = func() {}
			}
			defer cancel()

			vfs := osfs.NewWithNoIdm()
			results, err := build.New().Run(ctx, vfs, opts)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.checkResult != nil {
				tt.checkResult(t, results)
			}
		})
	}
}

func TestComputeArchiveChecksums(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tests := []struct {
		name    string
		useOSFS bool // use real osfs instead of memfs
		files   func(t *testing.T) []build.FileEntry
		wantN   int
		wantErr string
	}{
		{
			name: "virtual file checksummed",
			files: func(_ *testing.T) []build.FileEntry {
				return []build.FileEntry{
					{ArchivePath: "a/b.json", Content: []byte(`{"k":"v"}`)},
				}
			},
			wantN: 1,
		},
		{
			name:    "disk file checksummed",
			useOSFS: true,
			files: func(t *testing.T) []build.FileEntry {
				t.Helper()
				p := filepath.Join(t.TempDir(), "data.txt")
				if err := os.WriteFile(p, []byte("hello"), 0o644); err != nil {
					require.NoError(t, err)
				}
				return []build.FileEntry{
					{Src: p, ArchivePath: "data.txt"},
				}
			},
			wantN: 1,
		},
		{
			name:    "missing disk file returns error",
			useOSFS: true,
			files: func(_ *testing.T) []build.FileEntry {
				return []build.FileEntry{
					{Src: "/nonexistent/file.txt", ArchivePath: "file.txt"},
				}
			},
			wantErr: "checksum",
		},
		{
			name:    "cancelled context returns error",
			useOSFS: true,
			files: func(t *testing.T) []build.FileEntry {
				t.Helper()
				p := filepath.Join(t.TempDir(), "data.txt")
				if err := os.WriteFile(p, []byte("hello"), 0o644); err != nil {
					require.NoError(t, err)
				}
				return []build.FileEntry{
					{Src: p, ArchivePath: "data.txt"},
				}
			},
			wantErr: "context canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testCtx := ctx
			if tt.wantErr == "context canceled" {
				cancelCtx, cancel := context.WithCancel(context.Background())
				cancel()
				testCtx = cancelCtx
			}

			var entries []build.ChecksumEntry
			var err error
			if tt.useOSFS {
				entries, err = build.ComputeArchiveChecksums(
					testCtx,
					osfs.NewWithNoIdm(),
					tt.files(t),
				)
			} else {
				entries, err = build.ComputeArchiveChecksums(testCtx, memfs.New(), tt.files(t))
			}

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.Len(t, entries, tt.wantN)

			for _, e := range entries {
				assert.Len(t, e.Hash, 64)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestBuildPlugin
// --------------------------------------------------------------------------

func TestBuildPlugin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (vfs avfs.VFS, dir string, p manifest.Plugin, meta *metadata.Metadata)
		wantErr string
	}{
		{
			name: "succeeds with minimal plugin",
			setup: func(t *testing.T) (avfs.VFS, string, manifest.Plugin, *metadata.Metadata) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				vfs := osfs.NewWithNoIdm()
				p := manifest.Plugin{Name: "test-plugin", Version: "1.0.0", Description: "desc"}
				meta := &metadata.Metadata{
					GitCommitSHA:   "abc1234",
					BuildTimestamp: "2026-01-01T00:00:00Z",
				}
				return vfs, dir, p, meta
			},
		},
		{
			name: "fails when ResolveEntries returns error for bad skills glob",
			setup: func(t *testing.T) (avfs.VFS, string, manifest.Plugin, *metadata.Metadata) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				vfs := osfs.NewWithNoIdm()
				// Use a src/dest entry pointing to a nonexistent file — ResolveEntries
				// returns an error when a Src path does not exist.
				p := manifest.Plugin{
					Name:        "skills-err-plugin",
					Version:     "1.0.0",
					Description: "desc",
					Skills: []manifest.Entry{
						{Src: "nonexistent/missing.md", Dest: "missing.md"},
					},
				}
				meta := &metadata.Metadata{}
				return vfs, dir, p, meta
			},
			wantErr: "skills",
		},
		{
			name: "fails when archive Create returns error",
			setup: func(t *testing.T) (avfs.VFS, string, manifest.Plugin, *metadata.Metadata) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				vfs := createErrorForPathVFS{
					VFS:    osfs.NewWithNoIdm(),
					suffix: ".agentpack",
				}
				p := manifest.Plugin{Name: "create-err", Version: "1.0.0", Description: "desc"}
				meta := &metadata.Metadata{}
				return vfs, dir, p, meta
			},
			wantErr: "creating archive",
		},
		{
			name: "fails when Stat of output archive returns error",
			setup: func(t *testing.T) (avfs.VFS, string, manifest.Plugin, *metadata.Metadata) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				vfs := statErrorForPathVFS{
					VFS:    osfs.NewWithNoIdm(),
					suffix: ".agentpack",
				}
				p := manifest.Plugin{Name: "stat-err", Version: "1.0.0", Description: "desc"}
				meta := &metadata.Metadata{}
				return vfs, dir, p, meta
			},
			wantErr: "stat archive",
		},
		{
			name: "fails when Open of output archive for hashing returns error",
			setup: func(t *testing.T) (avfs.VFS, string, manifest.Plugin, *metadata.Metadata) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				vfs := openErrorForPathVFS{
					VFS:    osfs.NewWithNoIdm(),
					suffix: ".agentpack",
				}
				p := manifest.Plugin{Name: "hash-err", Version: "1.0.0", Description: "desc"}
				meta := &metadata.Metadata{}
				return vfs, dir, p, meta
			},
			wantErr: "hashing archive",
		},
		{
			name: "fails when sidecar Create returns error",
			setup: func(t *testing.T) (avfs.VFS, string, manifest.Plugin, *metadata.Metadata) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				vfs := createErrorForPathVFS{
					VFS:    osfs.NewWithNoIdm(),
					suffix: ".sha256",
				}
				p := manifest.Plugin{Name: "sidecar-err", Version: "1.0.0", Description: "desc"}
				meta := &metadata.Metadata{}
				return vfs, dir, p, meta
			},
			wantErr: "creating sidecar",
		},
		{
			name: "fails when buildMCPEntries returns error",
			setup: func(t *testing.T) (avfs.VFS, string, manifest.Plugin, *metadata.Metadata) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				// statAlwaysErrorVFS causes a non-NotExist stat error for the mcp config.
				vfs := statAlwaysErrorVFS{VFS: osfs.NewWithNoIdm()}
				p := manifest.Plugin{
					Name:        "mcp-err-plugin",
					Version:     "1.0.0",
					Description: "desc",
					MCP: []manifest.MCPEntry{
						{Type: "remote", Name: "srv", Config: ".mcp.json"},
					},
				}
				meta := &metadata.Metadata{}
				return vfs, dir, p, meta
			},
			wantErr: "stat mcp config",
		},
		{
			name: "fails when computeArchiveChecksums returns ctx error",
			setup: func(t *testing.T) (avfs.VFS, string, manifest.Plugin, *metadata.Metadata) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				p := manifest.Plugin{Name: "cs-ctx-err", Version: "1.0.0", Description: "desc"}
				meta := &metadata.Metadata{}
				return osfs.NewWithNoIdm(), dir, p, meta
			},
			wantErr: "context canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			vfs, dir, p, meta := tt.setup(t)

			testCtx := context.Background()
			if tt.wantErr == "context canceled" {
				testCtx = errOnlyContext{}
			}

			_, err := build.BuildPlugin(testCtx, vfs, dir, p, meta)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
		})
	}
}

// --------------------------------------------------------------------------
// TestBuildMCPEntries
// --------------------------------------------------------------------------

func TestBuildMCPEntries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (vfs avfs.VFS, dir string, p manifest.Plugin)
		wantErr string
	}{
		{
			name: "returns nil for empty mcp list",
			setup: func(t *testing.T) (avfs.VFS, string, manifest.Plugin) {
				t.Helper()
				return osfs.NewWithNoIdm(), t.TempDir(), manifest.Plugin{}
			},
		},
		{
			name: "fails on non-NotExist stat error for config file",
			setup: func(t *testing.T) (avfs.VFS, string, manifest.Plugin) {
				t.Helper()
				dir := t.TempDir()
				vfs := statAlwaysErrorVFS{VFS: osfs.NewWithNoIdm()}
				p := manifest.Plugin{
					MCP: []manifest.MCPEntry{
						{Type: "remote", Name: "srv", Config: ".mcp.json"},
					},
				}
				return vfs, dir, p
			},
			wantErr: "stat mcp config",
		},
		{
			name: "returns error when context is cancelled",
			setup: func(t *testing.T) (avfs.VFS, string, manifest.Plugin) {
				t.Helper()
				dir := t.TempDir()
				// Write real config files so stat passes.
				cfg := `{"mcpServers":{}}`
				if err := os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(cfg), 0o644); err != nil {
					require.NoError(t, err)
				}
				p := manifest.Plugin{
					MCP: []manifest.MCPEntry{
						{Type: "remote", Name: "srv", Config: ".mcp.json"},
						{Type: "remote", Name: "srv2", Config: ".mcp.json"},
					},
				}
				return osfs.NewWithNoIdm(), dir, p
			},
			wantErr: "context canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			vfs, dir, p := tt.setup(t)

			testCtx := context.Background()
			if tt.wantErr == "context canceled" {
				cancelCtx, cancel := context.WithCancel(context.Background())
				cancel()
				testCtx = cancelCtx
			}

			_, err := build.BuildMCPEntries(testCtx, vfs, dir, p)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
		})
	}
}
