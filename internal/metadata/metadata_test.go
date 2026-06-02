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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/retr0h/agentpack/internal/metadata"
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
		_, err := cmd.CombinedOutput()
		require.NoError(t, err)
	}

	run("init")
	run("checkout", "-b", branch)

	err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644)
	require.NoError(t, err)

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
	require.NoError(t, err)
	sha := strings.TrimSpace(string(shaBytes))

	detach := exec.Command("git", "checkout", "--detach", sha)
	detach.Dir = dir
	detach.Env = append(os.Environ(), gitEnv...)
	_, err = detach.CombinedOutput()
	require.NoError(t, err)
}

// initEmptyGitRepo creates a git repo with no commits (so rev-parse HEAD
// fails) but the directory is still a valid git repo.
func initEmptyGitRepo(t *testing.T, dir string) {
	t.Helper()

	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), gitEnv...)
	_, err := cmd.CombinedOutput()
	require.NoError(t, err)
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
	err := os.WriteFile(fakeGit, []byte(script), 0o755)
	require.NoError(t, err)
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

				assert.Equal(t, "my-plugin", m.Name)
				assert.Equal(t, "1.0.0", m.Version)
				assert.Len(t, m.GitCommitSHA, 40)
				assert.Equal(t, "main", m.GitBranch)
				assert.NotEmpty(t, m.BuildTimestamp)
				assert.Equal(t, "dev", m.BuilderVersion)

				want := runtime.GOOS + "-" + runtime.GOARCH
				assert.Equal(t, want, m.Platform)
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
				assert.Equal(t, "HEAD", m.GitBranch)
				assert.Len(t, m.GitCommitSHA, 40)
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupEnv != nil {
				tt.setupEnv(t)
			}

			dir := t.TempDir()
			if tt.setupDir != nil {
				tt.setupDir(t, dir)
			}

			m, err := metadata.Capture(ctx, dir, "my-plugin", "1.0.0")

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.checkResult != nil {
				tt.checkResult(t, m)
			}
		})
	}
}

// --------------------------------------------------------------------------
// ContentEntry / Entries
// --------------------------------------------------------------------------

func TestMetadataEntriesYAMLRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		entries []metadata.ContentEntry
		wantKey bool // whether "entries:" should appear in the YAML output
	}{
		{
			name:    "nil entries omitted from YAML",
			entries: nil,
			wantKey: false,
		},
		{
			name: "single entry round-trips",
			entries: []metadata.ContentEntry{
				{Name: "my-command", Type: "command"},
			},
			wantKey: true,
		},
		{
			name: "multiple entries round-trip",
			entries: []metadata.ContentEntry{
				{Name: "agent-one", Type: "agent"},
				{Name: "hook-init", Type: "hook"},
			},
			wantKey: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			original := metadata.Metadata{
				Name:    "test-plugin",
				Version: "0.1.0",
				Entries: tt.entries,
			}

			data, err := yaml.Marshal(original)
			require.NoError(t, err)

			if tt.wantKey {
				assert.Contains(t, string(data), "entries:")
			} else {
				assert.NotContains(t, string(data), "entries:")
			}

			var got metadata.Metadata
			require.NoError(t, yaml.Unmarshal(data, &got))

			assert.Equal(t, original.Entries, got.Entries)
		})
	}
}
