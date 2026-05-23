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

package metadata_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/retr0h/claudia/pkg/metadata"
)

// gitEnv returns env vars that satisfy git's author/committer requirements.
var gitEnv = []string{
	"GIT_AUTHOR_NAME=Test Author",
	"GIT_AUTHOR_EMAIL=test@example.com",
	"GIT_COMMITTER_NAME=Test Committer",
	"GIT_COMMITTER_EMAIL=test@example.com",
}

// initGitRepo creates a git repo in dir on the given branch with one commit.
func initGitRepo(t *testing.T, dir, branch string) {
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
	run("checkout", "-b", branch)

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	run("add", ".")
	run("commit", "-m", "init")
}

// initDetachedRepo creates a git repo, commits, then checks out in detached
// HEAD state.
func initDetachedRepo(t *testing.T, dir string) {
	t.Helper()

	initGitRepo(t, dir, "main")

	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	shaBytes, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	sha := strings.TrimSpace(string(shaBytes))

	detach := exec.Command("git", "checkout", "--detach", sha)
	detach.Dir = dir
	detach.Env = append(os.Environ(), gitEnv...)
	if out, err := detach.CombinedOutput(); err != nil {
		t.Fatalf("git checkout --detach: %v\n%s", err, out)
	}
}

// initEmptyGitRepo creates a git repo with no commits (so rev-parse HEAD
// fails) but the directory is still a valid git repo.
func initEmptyGitRepo(t *testing.T, dir string) {
	t.Helper()

	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), gitEnv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
}

// makeFakeGitDir creates a temporary directory containing a fake "git" script
// that returns a commit SHA for "rev-parse HEAD" but fails for
// "rev-parse --abbrev-ref HEAD". Returns the directory path.
func makeFakeGitDir(t *testing.T) string {
	t.Helper()

	fakeGitDir := t.TempDir()
	fakeGit := filepath.Join(fakeGitDir, "git")

	script := `#!/bin/sh
for arg in "$@"; do
  case "$arg" in
    --abbrev-ref)
      echo "simulated branch error" >&2
      exit 1
      ;;
  esac
done
echo "aabbccddaabbccddaabbccddaabbccddaabbccdd"
exit 0
`
	if err := os.WriteFile(fakeGit, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake git: %v", err)
	}
	return fakeGitDir
}

// --------------------------------------------------------------------------
// Capture
// --------------------------------------------------------------------------

// TestCapture is not parallel because the "branch rev-parse fails" row uses
// t.Setenv to inject a fake git, which is incompatible with t.Parallel.
func TestCapture(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		setupDir    func(t *testing.T, dir string) // nil means no git repo
		setupEnv    func(t *testing.T)             // optional env manipulation
		wantErr     string                         // substring; empty = success
		checkResult func(t *testing.T, m *metadata.Metadata)
	}{
		{
			name: "captures from git repo",
			setupDir: func(t *testing.T, dir string) {
				t.Helper()
				initGitRepo(t, dir, "main")
			},
			checkResult: func(t *testing.T, m *metadata.Metadata) {
				t.Helper()

				if m.Name != "my-plugin" {
					t.Errorf("Name = %q, want %q", m.Name, "my-plugin")
				}
				if m.Version != "1.0.0" {
					t.Errorf("Version = %q, want %q", m.Version, "1.0.0")
				}
				if len(m.GitCommitSHA) != 40 {
					t.Errorf(
						"GitCommitSHA length = %d, want 40; got %q",
						len(m.GitCommitSHA),
						m.GitCommitSHA,
					)
				}
				if m.GitBranch != "main" {
					t.Errorf("GitBranch = %q, want %q", m.GitBranch, "main")
				}
				if m.BuildTimestamp == "" {
					t.Error("BuildTimestamp is empty")
				}
				if m.BuilderVersion != "dev" {
					t.Errorf("BuilderVersion = %q, want %q", m.BuilderVersion, "dev")
				}
				want := runtime.GOOS + "-" + runtime.GOARCH
				if m.Platform != want {
					t.Errorf("Platform = %q, want %q", m.Platform, want)
				}
			},
		},
		{
			name:     "fails outside git repo",
			setupDir: nil,
			wantErr:  "not a git repository",
		},
		{
			name: "detached HEAD branch is HEAD",
			setupDir: func(t *testing.T, dir string) {
				t.Helper()
				initDetachedRepo(t, dir)
			},
			checkResult: func(t *testing.T, m *metadata.Metadata) {
				t.Helper()
				if m.GitBranch != "HEAD" {
					t.Errorf("GitBranch = %q, want %q", m.GitBranch, "HEAD")
				}
				if len(m.GitCommitSHA) != 40 {
					t.Errorf("GitCommitSHA length = %d, want 40", len(m.GitCommitSHA))
				}
			},
		},
		{
			name: "empty git repo fails rev-parse HEAD",
			setupDir: func(t *testing.T, dir string) {
				t.Helper()
				initEmptyGitRepo(t, dir)
			},
			wantErr: "git rev-parse HEAD",
		},
		{
			name: "branch rev-parse fails",
			setupEnv: func(t *testing.T) {
				t.Helper()
				fakeGitDir := makeFakeGitDir(t)
				origPath := os.Getenv("PATH")
				t.Setenv("PATH", fakeGitDir+string(os.PathListSeparator)+origPath)
			},
			wantErr: "git rev-parse --abbrev-ref HEAD",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setupEnv != nil {
				tc.setupEnv(t)
			}

			dir := t.TempDir()
			if tc.setupDir != nil {
				tc.setupDir(t, dir)
			}

			m, err := metadata.Capture(ctx, dir, "my-plugin", "1.0.0")

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tc.checkResult != nil {
				tc.checkResult(t, m)
			}
		})
	}
}
