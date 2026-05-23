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

package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/avfs/avfs/vfs/osfs"

	"github.com/retr0h/claudia/internal/archive"
	"github.com/retr0h/claudia/internal/checksum"
)

var gitEnv = []string{
	"GIT_AUTHOR_NAME=Test Author",
	"GIT_AUTHOR_EMAIL=test@example.com",
	"GIT_COMMITTER_NAME=Test Committer",
	"GIT_COMMITTER_EMAIL=test@example.com",
}

func initTestRepo(t *testing.T, dir string) {
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
}

func commitAll(t *testing.T, dir string) {
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

	run("add", ".")
	run("commit", "-m", "test")
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunBuild(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		manifest   string
		setupFiles func(t *testing.T, dir string)
		names      []string
		noGit      bool // skip git repo init when true
		wantErr    string
		checkBuild func(t *testing.T, dir string)
	}{
		{
			name: "single plugin with skills and commands",
			manifest: `
name: test-plugin
version: 1.0.0
description: "A test plugin"
author:
  name: Test Author
  email: test@example.com
license: MIT
skills:
  - skills/*.md
commands:
  - commands/*.md
`,
			setupFiles: func(t *testing.T, dir string) {
				t.Helper()
				writeFile(t, filepath.Join(dir, "skills", "review.md"), "# Review Skill")
				writeFile(t, filepath.Join(dir, "commands", "init.md"), "# Init Command")
			},
			checkBuild: func(t *testing.T, dir string) {
				t.Helper()
				archivePath := filepath.Join(dir, "test-plugin-1.0.0.claudia")
				if _, err := os.Stat(archivePath); err != nil {
					t.Fatalf("archive not created: %v", err)
				}

				extractDir := t.TempDir()
				if err := archive.Extract(archivePath, extractDir); err != nil {
					t.Fatalf("extract: %v", err)
				}

				expected := []string{
					"marketplaces/test-plugin/.claude-plugin/marketplace.json",
					"marketplaces/test-plugin/.claude-plugin/plugin.json",
					"marketplaces/test-plugin/.claudia/metadata.json",
					"marketplaces/test-plugin/.claudia/checksums.txt",
					"marketplaces/test-plugin/.claudia/claudia.yaml",
					"marketplaces/test-plugin/skills/review.md",
					"marketplaces/test-plugin/commands/init.md",
				}
				for _, path := range expected {
					fullPath := filepath.Join(extractDir, path)
					if _, err := os.Stat(fullPath); err != nil {
						t.Errorf("missing in archive: %s", path)
					}
				}
			},
		},
		{
			name: "filter by plugin name",
			manifest: `
author:
  name: Test Author
  email: test@example.com
license: MIT
plugins:
  - name: plugin-a
    version: 1.0.0
    description: "Plugin A"
    skills:
      - skills/a.md
  - name: plugin-b
    version: 2.0.0
    description: "Plugin B"
    skills:
      - skills/b.md
`,
			setupFiles: func(t *testing.T, dir string) {
				t.Helper()
				writeFile(t, filepath.Join(dir, "skills", "a.md"), "# A")
				writeFile(t, filepath.Join(dir, "skills", "b.md"), "# B")
			},
			names: []string{"plugin-a"},
			checkBuild: func(t *testing.T, dir string) {
				t.Helper()
				if _, err := os.Stat(filepath.Join(dir, "plugin-a-1.0.0.claudia")); err != nil {
					t.Error("plugin-a archive not created")
				}
				if _, err := os.Stat(filepath.Join(dir, "plugin-b-2.0.0.claudia")); err == nil {
					t.Error("plugin-b archive should not have been created")
				}
			},
		},
		{
			name: "multiple plugins builds summary line",
			manifest: `
author:
  name: Test Author
  email: test@example.com
license: MIT
plugins:
  - name: plugin-a
    version: 1.0.0
    description: "Plugin A"
    skills:
      - skills/a.md
  - name: plugin-b
    version: 2.0.0
    description: "Plugin B"
    skills:
      - skills/b.md
`,
			setupFiles: func(t *testing.T, dir string) {
				t.Helper()
				writeFile(t, filepath.Join(dir, "skills", "a.md"), "# A")
				writeFile(t, filepath.Join(dir, "skills", "b.md"), "# B")
			},
			checkBuild: func(t *testing.T, dir string) {
				t.Helper()
				if _, err := os.Stat(filepath.Join(dir, "plugin-a-1.0.0.claudia")); err != nil {
					t.Error("plugin-a archive not created")
				}
				if _, err := os.Stat(filepath.Join(dir, "plugin-b-2.0.0.claudia")); err != nil {
					t.Error("plugin-b archive not created")
				}
			},
		},
		{
			name: "plugin with mcp binary entry",
			manifest: `
name: mcp-plugin
version: 1.0.0
description: "Plugin with MCP binary"
mcp:
  - type: binary
    name: my-server
    src: bin/my-server
`,
			setupFiles: func(t *testing.T, dir string) {
				t.Helper()
				writeFile(t, filepath.Join(dir, "bin", "my-server"), "#!/bin/sh\necho hi")
			},
			checkBuild: func(t *testing.T, dir string) {
				t.Helper()
				if _, err := os.Stat(filepath.Join(dir, "mcp-plugin-1.0.0.claudia")); err != nil {
					t.Error("mcp-plugin archive not created")
				}
			},
		},
		{
			name: "plugin with mcp remote entry",
			manifest: `
name: remote-plugin
version: 1.0.0
description: "Plugin with remote MCP"
mcp:
  - type: remote
    name: my-remote
    url: https://mcp.example.com/v1
`,
			checkBuild: func(t *testing.T, dir string) {
				t.Helper()
				if _, err := os.Stat(filepath.Join(dir, "remote-plugin-1.0.0.claudia")); err != nil {
					t.Error("remote-plugin archive not created")
				}
			},
		},
		{
			name: "plugin with mcp ux entry",
			manifest: `
name: ux-plugin
version: 1.0.0
description: "Plugin with UX MCP"
mcp:
  - type: ux
    name: my-ux
    package: "@mycompany/my-server"
`,
			checkBuild: func(t *testing.T, dir string) {
				t.Helper()
				if _, err := os.Stat(filepath.Join(dir, "ux-plugin-1.0.0.claudia")); err != nil {
					t.Error("ux-plugin archive not created")
				}
			},
		},
		{
			name: "plugin with mcp config entry",
			manifest: `
name: config-plugin
version: 1.0.0
description: "Plugin with config MCP"
mcp:
  - type: remote
    name: my-config-server
    config: .mcp.json
`,
			setupFiles: func(t *testing.T, dir string) {
				t.Helper()
				writeFile(t, filepath.Join(dir, ".mcp.json"), `{"mcpServers":{}}`)
			},
			checkBuild: func(t *testing.T, dir string) {
				t.Helper()
				if _, err := os.Stat(filepath.Join(dir, "config-plugin-1.0.0.claudia")); err != nil {
					t.Error("config-plugin archive not created")
				}
			},
		},
		{
			name: "mcp binary src not found returns error",
			manifest: `
name: bad-mcp
version: 1.0.0
description: "Plugin with missing MCP binary"
mcp:
  - type: binary
    name: my-server
    src: bin/nonexistent
`,
			wantErr: "mcp binary not found",
		},
		{
			name: "mcp config not found returns error",
			manifest: `
name: bad-config-mcp
version: 1.0.0
description: "Plugin with missing MCP config"
mcp:
  - type: remote
    name: my-server
    config: nonexistent.mcp.json
`,
			wantErr: "mcp config not found",
		},
		{
			name: "unknown plugin name returns error",
			manifest: `
name: test-plugin
version: 1.0.0
description: "A test plugin"
`,
			names:   []string{"nonexistent"},
			wantErr: "not found in claudia.yaml",
		},
		{
			name: "missing manifest returns error",
			setupFiles: func(t *testing.T, dir string) {
				t.Helper()
				writeFile(t, filepath.Join(dir, "README.md"), "placeholder")
			},
			wantErr: "claudia.yaml not found",
		},
		{
			name: "end-to-end build and verify round-trip",
			manifest: `
name: integration-test
version: 0.1.0
description: "End-to-end integration test plugin"
author:
  name: Test Author
  email: test@example.com
license: MIT
skills:
  - skills/*.md
commands:
  - commands/*.md
settings:
  - settings/settings.json
`,
			setupFiles: func(t *testing.T, dir string) {
				t.Helper()
				writeFile(t, filepath.Join(dir, "skills", "analyze.md"), "# Analyze\nA skill.")
				writeFile(t, filepath.Join(dir, "skills", "review.md"), "# Review\nAnother skill.")
				writeFile(t, filepath.Join(dir, "commands", "init.md"), "# Init\nA command.")
				writeFile(t, filepath.Join(dir, "settings", "settings.json"), `{"key":"value"}`)
			},
			checkBuild: func(t *testing.T, dir string) {
				t.Helper()
				archivePath := filepath.Join(dir, "integration-test-0.1.0.claudia")
				if _, err := os.Stat(archivePath); err != nil {
					t.Fatalf("archive not created: %v", err)
				}
				if err := runVerify(archivePath); err != nil {
					t.Fatalf("verify: %v", err)
				}
			},
		},
		{
			name: "metadata capture error when not a git repo",
			manifest: `
name: test-plugin
version: 1.0.0
description: "A test plugin"
`,
			noGit:   true,
			wantErr: "not a git repository",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			vfs := osfs.NewWithNoIdm()

			dir := t.TempDir()

			if !tt.noGit {
				initTestRepo(t, dir)
			}

			if tt.manifest != "" {
				writeFile(t, filepath.Join(dir, "claudia.yaml"), tt.manifest)
			}

			if tt.setupFiles != nil {
				tt.setupFiles(t, dir)
			}

			if !tt.noGit {
				commitAll(t, dir)
			}

			err := runBuild(ctx, vfs, dir, tt.names)

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

			if tt.checkBuild != nil {
				tt.checkBuild(t, dir)
			}
		})
	}
}

func TestShortSHA(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		sha  string
		want string
	}{
		{
			name: "full 40-char SHA returns first 7 chars",
			sha:  "abc1234def5678901234567890123456789abcde",
			want: "abc1234",
		},
		{
			name: "exactly 7 chars returns as-is",
			sha:  "abc1234",
			want: "abc1234",
		},
		{
			name: "fewer than 7 chars returns as-is",
			sha:  "abc",
			want: "abc",
		},
		{
			name: "empty string returns empty string",
			sha:  "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := shortSHA(tt.sha)
			if got != tt.want {
				t.Errorf("shortSHA(%q) = %q, want %q", tt.sha, got, tt.want)
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
			name:  "1023 bytes stays in B",
			bytes: 1023,
			want:  "1023 B",
		},
		{
			name:  "1024 bytes is 1 KB",
			bytes: 1024,
			want:  "1 KB",
		},
		{
			name:  "2048 bytes is 2 KB",
			bytes: 2048,
			want:  "2 KB",
		},
		{
			name:  "1536 bytes is 1 KB (truncated)",
			bytes: 1536,
			want:  "1 KB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := humanSize(tt.bytes)
			if got != tt.want {
				t.Errorf("humanSize(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestComputeArchiveChecksums(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	vfs := osfs.NewWithNoIdm()

	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "hello.txt")
	if err := os.WriteFile(srcFile, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		files       func() []archive.FileEntry
		wantErr     bool
		wantCount   int
		checkResult func(t *testing.T, entries []checksum.Entry)
	}{
		{
			name: "src file and virtual content produce correct hashes",
			files: func() []archive.FileEntry {
				return []archive.FileEntry{
					{Src: srcFile, ArchivePath: "a/hello.txt"},
					{ArchivePath: "b/virtual.txt", Content: []byte("virtual")},
				}
			},
			wantCount: 2,
			checkResult: func(t *testing.T, entries []checksum.Entry) {
				t.Helper()
				wantHash := checksum.ComputeBytes([]byte("hello"))
				if entries[0].Hash != wantHash {
					t.Errorf("entry[0].Hash = %q, want %q", entries[0].Hash, wantHash)
				}
				if entries[0].Path != "a/hello.txt" {
					t.Errorf("entry[0].Path = %q, want %q", entries[0].Path, "a/hello.txt")
				}
				wantVirtHash := checksum.ComputeBytes([]byte("virtual"))
				if entries[1].Hash != wantVirtHash {
					t.Errorf("entry[1].Hash = %q, want %q", entries[1].Hash, wantVirtHash)
				}
			},
		},
		{
			name: "missing src file returns error",
			files: func() []archive.FileEntry {
				return []archive.FileEntry{
					{Src: filepath.Join(srcDir, "missing.txt"), ArchivePath: "c/missing.txt"},
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			entries, err := computeArchiveChecksums(ctx, vfs, tt.files())

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(entries) != tt.wantCount {
				t.Fatalf("entries count = %d, want %d", len(entries), tt.wantCount)
			}

			if tt.checkResult != nil {
				tt.checkResult(t, entries)
			}
		})
	}
}
