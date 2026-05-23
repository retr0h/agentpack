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
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/avfs/avfs/vfs/memfs"
	"github.com/avfs/avfs/vfs/osfs"

	"github.com/retr0h/claudia/pkg/build"
)

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

// writeManifest writes content as claudia.yaml in dir.
func writeManifest(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "claudia.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write claudia.yaml: %v", err)
	}
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
				if len(results) != 1 {
					t.Fatalf("result count = %d, want 1", len(results))
				}
				r := results[0]
				if r.Name != "my-plugin" {
					t.Errorf("Name = %q, want %q", r.Name, "my-plugin")
				}
				if r.Version != "1.0.0" {
					t.Errorf("Version = %q, want %q", r.Version, "1.0.0")
				}
				if r.SHA256 == "" {
					t.Error("SHA256 is empty")
				}
				if len(r.SHA256) != 64 {
					t.Errorf("SHA256 length = %d, want 64", len(r.SHA256))
				}
				if r.Size == 0 {
					t.Error("Size is 0")
				}
				if !strings.HasSuffix(r.ArchivePath, "my-plugin-1.0.0.claudia") {
					t.Errorf("ArchivePath = %q, expected suffix my-plugin-1.0.0.claudia", r.ArchivePath)
				}
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
				if len(results) != 2 {
					t.Fatalf("result count = %d, want 2", len(results))
				}
				names := map[string]bool{}
				for _, r := range results {
					names[r.Name] = true
				}
				if !names["plugin-a"] || !names["plugin-b"] {
					t.Errorf("results = %v, want plugin-a and plugin-b", names)
				}
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
				if len(results) != 1 {
					t.Fatalf("result count = %d, want 1", len(results))
				}
				if results[0].Name != "plugin-b" {
					t.Errorf("Name = %q, want plugin-b", results[0].Name)
				}
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
			name: "fails when not a git repo",
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
			wantErr: "not a git repository",
		},
		{
			name: "fails when claudia.yaml missing",
			setup: func(t *testing.T) (string, func()) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				return dir, func() {}
			},
			opts: func(dir string) build.Options {
				return build.Options{Dir: dir}
			},
			wantErr: "claudia.yaml not found",
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
			name: "builds plugin with skills",
			setup: func(t *testing.T) (string, func()) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)

				skillDir := filepath.Join(dir, "skills")
				if err := os.MkdirAll(skillDir, 0o755); err != nil {
					t.Fatalf("mkdir skills: %v", err)
				}
				if err := os.WriteFile(filepath.Join(skillDir, "intro.md"), []byte("# Intro"), fs.FileMode(0o644)); err != nil {
					t.Fatalf("write skill: %v", err)
				}

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
				if len(results) != 1 {
					t.Fatalf("result count = %d, want 1", len(results))
				}
				if results[0].FileCount == 0 {
					t.Error("FileCount is 0, expected > 0")
				}
			},
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

			// Special case: cancelled context test.
			if tt.wantErr == "context canceled" {
				ctx, cancel = context.WithCancel(context.Background())
				cancel() // cancel immediately
			} else {
				ctx = context.Background()
				cancel = func() {}
			}
			defer cancel()

			vfs := osfs.NewWithNoIdm()
			results, err := build.Run(ctx, vfs, opts)

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

// --------------------------------------------------------------------------
// shortSHA and humanSize are private — tested indirectly via Run results.
// computeArchiveChecksums is tested indirectly through Run.
// We expose thin wrappers via export_test.go for direct testing.
// --------------------------------------------------------------------------

func TestShortSHA(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		sha  string
		want string
	}{
		{
			name: "40-char sha returns first 7",
			sha:  "aabbccddaabbccddaabbccddaabbccddaabbccdd",
			want: "aabbccd",
		},
		{
			name: "7-char sha unchanged",
			sha:  "abcdefg",
			want: "abcdefg",
		},
		{
			name: "short sha returned as-is",
			sha:  "abc",
			want: "abc",
		},
		{
			name: "empty sha returned as-is",
			sha:  "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := build.ShortSHA(tt.sha)
			if got != tt.want {
				t.Errorf("ShortSHA(%q) = %q, want %q", tt.sha, got, tt.want)
			}
		})
	}
}

func TestHumanSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{
			name:  "zero bytes",
			bytes: 0,
			want:  "0 B",
		},
		{
			name:  "under 1 KB",
			bytes: 512,
			want:  "512 B",
		},
		{
			name:  "exactly 1 KB",
			bytes: 1024,
			want:  "1 KB",
		},
		{
			name:  "multiple KB",
			bytes: 5120,
			want:  "5 KB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := build.HumanSize(tt.bytes)
			if got != tt.want {
				t.Errorf("HumanSize(%d) = %q, want %q", tt.bytes, got, tt.want)
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
					t.Fatal(err)
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var entries []build.ChecksumEntry
			var err error
			if tt.useOSFS {
				entries, err = build.ComputeArchiveChecksums(ctx, osfs.NewWithNoIdm(), tt.files(t))
			} else {
				entries, err = build.ComputeArchiveChecksums(ctx, memfs.New(), tt.files(t))
			}

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

			if len(entries) != tt.wantN {
				t.Fatalf("entry count = %d, want %d", len(entries), tt.wantN)
			}

			for _, e := range entries {
				if len(e.Hash) != 64 {
					t.Errorf("hash length = %d, want 64 for %q", len(e.Hash), e.Path)
				}
			}
		})
	}
}
