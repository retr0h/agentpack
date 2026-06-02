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

package fetcher_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/internal/fetcher"
)

// --------------------------------------------------------------------------
// Git repo helpers
// --------------------------------------------------------------------------

var gitEnvVars = []string{
	"GIT_AUTHOR_NAME=Test Author",
	"GIT_AUTHOR_EMAIL=test@example.com",
	"GIT_COMMITTER_NAME=Test Committer",
	"GIT_COMMITTER_EMAIL=test@example.com",
}

// initBareRepo creates a bare git repo at dest that can be cloned locally.
// It first creates a working repo with a commit, then clones it bare.
func initBareRepo(t *testing.T) (bareDir string) {
	t.Helper()

	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), gitEnvVars...)
		_, err := cmd.CombinedOutput()
		require.NoError(t, err)
	}

	// Working repo.
	workDir := t.TempDir()
	run(workDir, "init")
	run(workDir, "checkout", "-b", "main")

	require.NoError(t, os.WriteFile(
		filepath.Join(workDir, "README.md"),
		[]byte("# test repo\n"),
		0o644,
	))

	run(workDir, "add", ".")
	run(workDir, "commit", "-m", "init")

	// Bare clone for use as a remote.
	bareDir = t.TempDir()
	run(workDir, "clone", "--bare", workDir, bareDir)

	return bareDir
}

// initBareRepoWithTag creates a bare repo and tags the HEAD commit.
func initBareRepoWithTag(t *testing.T, tag string) (bareDir string) {
	t.Helper()

	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), gitEnvVars...)
		_, err := cmd.CombinedOutput()
		require.NoError(t, err)
	}

	workDir := t.TempDir()
	run(workDir, "init")
	run(workDir, "checkout", "-b", "main")

	require.NoError(t, os.WriteFile(
		filepath.Join(workDir, "README.md"),
		[]byte("# test repo\n"),
		0o644,
	))

	run(workDir, "add", ".")
	run(workDir, "commit", "-m", "init")
	run(workDir, "tag", tag)

	bareDir = t.TempDir()
	run(workDir, "clone", "--bare", workDir, bareDir)

	return bareDir
}

// --------------------------------------------------------------------------
// TestGitFetcher_Fetch
// --------------------------------------------------------------------------

func TestGitFetcher_Fetch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(t *testing.T) (source string, dest string)
		cancelCtx bool
		wantErr   string
		check     func(t *testing.T, dest string)
	}{
		{
			name: "clones local bare repo",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				bareDir := initBareRepo(t)
				dest := t.TempDir()
				return bareDir, dest
			},
			check: func(t *testing.T, dest string) {
				t.Helper()
				// README.md should be present; .git/ should not.
				_, err := os.Stat(filepath.Join(dest, "README.md"))
				assert.NoError(t, err)
				_, err = os.Stat(filepath.Join(dest, ".git"))
				assert.Error(t, err)
			},
		},
		{
			name: "clones repo with tag ref",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				bareDir := initBareRepoWithTag(t, "v1.0.0")
				dest := t.TempDir()
				return bareDir + "#v1.0.0", dest
			},
			check: func(t *testing.T, dest string) {
				t.Helper()
				_, err := os.Stat(filepath.Join(dest, "README.md"))
				assert.NoError(t, err)
			},
		},
		{
			name: "missing repo returns error",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return "/nonexistent/path/to/repo", t.TempDir()
			},
			wantErr: "git clone",
		},
		{
			name: "context cancelled before fetch returns error",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				bareDir := initBareRepo(t)
				dest := t.TempDir()
				return bareDir, dest
			},
			cancelCtx: true,
			wantErr:   "context",
		},
		{
			name: "empty source returns error",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return "", t.TempDir()
			},
			wantErr: "git source must not be empty",
		},
		{
			name: "source with only fragment returns error",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return "#v1.0.0", t.TempDir()
			},
			wantErr: "empty URL before '#'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			source, dest := tt.setup(t)

			var ctx context.Context
			var cancel context.CancelFunc

			if tt.cancelCtx {
				ctx, cancel = context.WithCancel(context.Background())
				cancel() // cancel immediately
			} else {
				ctx, cancel = context.WithCancel(context.Background())
			}

			defer cancel()

			f := &fetcher.GitFetcher{}
			err := f.Fetch(ctx, source, dest)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, dest)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestCopyWorktree
// --------------------------------------------------------------------------

func TestCopyWorktree(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(t *testing.T) (src string, dst string)
		customCtx context.Context
		wantErr   string
		check     func(t *testing.T, dst string)
	}{
		{
			name: "copies files skipping .git directory",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				require.NoError(
					t,
					os.WriteFile(filepath.Join(src, "file.txt"), []byte("hello"), 0o644),
				)
				require.NoError(t, os.MkdirAll(filepath.Join(src, ".git"), 0o755))
				require.NoError(
					t,
					os.WriteFile(
						filepath.Join(src, ".git", "HEAD"),
						[]byte("ref: refs/heads/main"),
						0o644,
					),
				)
				return src, t.TempDir()
			},
			check: func(t *testing.T, dst string) {
				t.Helper()
				_, err := os.Stat(filepath.Join(dst, "file.txt"))
				assert.NoError(t, err)
				_, err = os.Stat(filepath.Join(dst, ".git"))
				assert.Error(t, err)
			},
		},
		{
			name: "copies files in subdirectories",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(src, "subdir"), 0o755))
				require.NoError(
					t,
					os.WriteFile(
						filepath.Join(src, "subdir", "nested.txt"),
						[]byte("nested"),
						0o644,
					),
				)
				return src, t.TempDir()
			},
			check: func(t *testing.T, dst string) {
				t.Helper()
				_, err := os.Stat(filepath.Join(dst, "subdir", "nested.txt"))
				assert.NoError(t, err)
			},
		},
		{
			name: "walk error when src does not exist",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return "/nonexistent/src/dir", t.TempDir()
			},
			wantErr: "lstat",
		},
		{
			name: "context cancelled returns error",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				require.NoError(
					t,
					os.WriteFile(filepath.Join(src, "file.txt"), []byte("data"), 0o644),
				)
				return src, t.TempDir()
			},
			customCtx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
			wantErr: "context canceled",
		},
		{
			name: "skips file named .git at root",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				require.NoError(
					t,
					os.WriteFile(filepath.Join(src, ".git"), []byte("worktree: something"), 0o644),
				)
				require.NoError(
					t,
					os.WriteFile(filepath.Join(src, "README.md"), []byte("readme"), 0o644),
				)
				return src, t.TempDir()
			},
			check: func(t *testing.T, dst string) {
				t.Helper()
				_, err := os.Stat(filepath.Join(dst, ".git"))
				assert.Error(t, err)
				_, err = os.Stat(filepath.Join(dst, "README.md"))
				assert.NoError(t, err)
			},
		},
		{
			name: "returns error when dst is read-only and src has subdirectory",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				if os.Getuid() == 0 {
					t.Skip("root bypasses permission checks")
				}
				src := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(src, "subdir"), 0o755))
				require.NoError(
					t,
					os.WriteFile(filepath.Join(src, "subdir", "f.txt"), []byte("x"), 0o644),
				)
				dst := t.TempDir()
				require.NoError(t, os.Chmod(dst, 0o555))
				t.Cleanup(func() { _ = os.Chmod(dst, 0o755) })
				return src, dst
			},
			wantErr: "mkdir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src, dst := tt.setup(t)

			var ctx context.Context
			if tt.customCtx != nil {
				ctx = tt.customCtx
			} else {
				ctx = context.Background()
			}

			err := fetcher.CopyWorktree(ctx, src, dst)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, dst)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestCopyFileGit
// --------------------------------------------------------------------------

func TestCopyFileGit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (src string, dst string)
		wantErr string
		check   func(t *testing.T, dst string)
	}{
		{
			name: "copies file preserving permissions",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				src := filepath.Join(dir, "source.txt")
				require.NoError(t, os.WriteFile(src, []byte("content"), 0o755))
				return src, filepath.Join(dir, "dest.txt")
			},
			check: func(t *testing.T, dst string) {
				t.Helper()
				data, err := os.ReadFile(dst)
				require.NoError(t, err)
				assert.Equal(t, "content", string(data))
			},
		},
		{
			name: "returns error when src does not exist",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return "/nonexistent/src.txt", filepath.Join(t.TempDir(), "dst.txt")
			},
			wantErr: "read",
		},
		{
			name: "returns error when dst dir does not exist",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				src := filepath.Join(dir, "source.txt")
				require.NoError(t, os.WriteFile(src, []byte("data"), 0o644))
				return src, filepath.Join(dir, "nonexistent", "dst.txt")
			},
			wantErr: "write",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src, dst := tt.setup(t)
			err := fetcher.CopyFileGit(src, dst)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, dst)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestParseGitSource
// --------------------------------------------------------------------------

func TestParseGitSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		source     string
		wantRawURL string
		wantRef    string
		wantErr    string
	}{
		{
			name:       "bare host path no ref",
			source:     "github.com/org/repo",
			wantRawURL: "github.com/org/repo",
			wantRef:    "",
		},
		{
			name:       "host path with tag ref",
			source:     "github.com/org/repo#v1.0.0",
			wantRawURL: "github.com/org/repo",
			wantRef:    "v1.0.0",
		},
		{
			name:       "https URL no ref",
			source:     "https://github.com/org/repo.git",
			wantRawURL: "https://github.com/org/repo.git",
			wantRef:    "",
		},
		{
			name:       "https URL with SHA ref",
			source:     "https://github.com/org/repo.git#abc1234",
			wantRawURL: "https://github.com/org/repo.git",
			wantRef:    "abc1234",
		},
		{
			name:    "empty source returns error",
			source:  "",
			wantErr: "must not be empty",
		},
		{
			name:    "source with only fragment",
			source:  "#v1.0.0",
			wantErr: "empty URL before '#'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rawURL, ref, err := fetcher.ParseGitSource(tt.source)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantRawURL, rawURL)
			assert.Equal(t, tt.wantRef, ref)
		})
	}
}

// --------------------------------------------------------------------------
// TestToGitURL
// --------------------------------------------------------------------------

func TestToGitURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		rawURL  string
		wantURL string
	}{
		{
			name:    "bare host path gets https and .git suffix",
			rawURL:  "github.com/org/repo",
			wantURL: "https://github.com/org/repo.git",
		},
		{
			name:    "bare host path with .git suffix unchanged except scheme",
			rawURL:  "github.com/org/repo.git",
			wantURL: "https://github.com/org/repo.git",
		},
		{
			name:    "https URL returned unchanged",
			rawURL:  "https://github.com/org/repo.git",
			wantURL: "https://github.com/org/repo.git",
		},
		{
			name:    "http URL returned unchanged",
			rawURL:  "http://internal.example.com/repo.git",
			wantURL: "http://internal.example.com/repo.git",
		},
		{
			name:    "absolute path returned unchanged",
			rawURL:  "/tmp/my-bare-repo",
			wantURL: "/tmp/my-bare-repo",
		},
		{
			name:    "relative path returned unchanged",
			rawURL:  "./my-bare-repo",
			wantURL: "./my-bare-repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := fetcher.ToGitURL(tt.rawURL)
			assert.Equal(t, tt.wantURL, got)
		})
	}
}

// --------------------------------------------------------------------------
// TestLsRemote
// --------------------------------------------------------------------------

func TestLsRemote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) string // returns rawURL
		wantErr string
		check   func(t *testing.T, refs map[string]string)
	}{
		{
			name: "lists refs from local bare repo",
			setup: func(t *testing.T) string {
				t.Helper()
				return initBareRepo(t)
			},
			check: func(t *testing.T, refs map[string]string) {
				t.Helper()
				assert.NotEmpty(t, refs)
			},
		},
		{
			name: "nonexistent repo returns error",
			setup: func(t *testing.T) string {
				t.Helper()
				return "/nonexistent/path/to/repo"
			},
			wantErr: "ls-remote",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rawURL := tt.setup(t)
			refs, err := fetcher.LsRemote(context.Background(), rawURL)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, refs)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestGitFetcher_FetchWithResult
// --------------------------------------------------------------------------

// getSHA returns the HEAD SHA of a bare repo by running git rev-parse.
func getSHA(t *testing.T, bareDir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = bareDir
	cmd.Env = append(os.Environ(), gitEnvVars...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}

func TestGitFetcher_FetchWithResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		setup    func(t *testing.T) (source string, dest string)
		wantErr  string
		checkSHA func(t *testing.T, sha string, bareDir string)
	}{
		{
			name: "returns resolved SHA for HEAD clone",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				bareDir := initBareRepo(t)
				return bareDir, t.TempDir()
			},
			checkSHA: func(t *testing.T, sha string, bareDir string) {
				t.Helper()
				assert.Len(t, sha, 40)
				expected := getSHA(t, bareDir)
				assert.Equal(t, expected, sha)
			},
		},
		{
			name: "SHA ref checkout returns that SHA",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				bareDir := initBareRepo(t)
				sha := getSHA(t, bareDir)
				return bareDir + "#" + sha, t.TempDir()
			},
			checkSHA: func(t *testing.T, sha string, _ string) {
				t.Helper()
				assert.Len(t, sha, 40)
			},
		},
		{
			name: "branch ref checkout succeeds",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				bareDir := initBareRepo(t)
				return bareDir + "#main", t.TempDir()
			},
			checkSHA: func(t *testing.T, sha string, bareDir string) {
				t.Helper()
				expected := getSHA(t, bareDir)
				assert.Equal(t, expected, sha)
			},
		},
		{
			name: "second fetch hits cached repo path",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				bareDir := initBareRepo(t)
				// First fetch populates the cache.
				dest1 := t.TempDir()
				f := &fetcher.GitFetcher{}
				require.NoError(t, f.Fetch(context.Background(), bareDir, dest1))
				// Return same source for second fetch.
				return bareDir, t.TempDir()
			},
			checkSHA: func(t *testing.T, sha string, _ string) {
				t.Helper()
				assert.Len(t, sha, 40)
			},
		},
		{
			name: "nonexistent tag on cached repo returns resolve error",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				bareDir := initBareRepo(t)
				// First: clone with HEAD to populate the cache.
				f := &fetcher.GitFetcher{}
				require.NoError(t, f.Fetch(context.Background(), bareDir, t.TempDir()))
				// Second: try to checkout a tag that doesn't exist in the cached repo.
				return bareDir + "#v99.0.0", t.TempDir()
			},
			wantErr: "resolve tag v99.0.0",
		},
		{
			name: "nonexistent branch on cached repo returns resolve error",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				bareDir := initBareRepo(t)
				// First: clone with HEAD to populate the cache.
				f := &fetcher.GitFetcher{}
				require.NoError(t, f.Fetch(context.Background(), bareDir, t.TempDir()))
				// Second: try to checkout a branch that doesn't exist in the cached repo.
				return bareDir + "#nonexistent-branch", t.TempDir()
			},
			wantErr: "resolve branch nonexistent-branch",
		},
		{
			name: "copy worktree to read-only dest returns error",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				if os.Getuid() == 0 {
					t.Skip("root bypasses permission checks")
				}
				bareDir := initBareRepo(t)
				// Dest is a read-only directory so copyWorktree fails when
				// trying to create subdirectories inside it.
				dst := t.TempDir()
				require.NoError(t, os.Chmod(dst, 0o555))
				t.Cleanup(func() { _ = os.Chmod(dst, 0o755) })
				return bareDir, dst
			},
			wantErr: "copy worktree",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			source, dest := tt.setup(t)

			// Extract bare dir from source (strip any #ref).
			bareDir := source
			if idx := strings.LastIndex(source, "#"); idx >= 0 {
				bareDir = source[:idx]
			}

			f := &fetcher.GitFetcher{}
			sha, err := f.FetchWithResult(context.Background(), source, dest)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.checkSHA != nil {
				tt.checkSHA(t, sha, bareDir)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestIsSHA
// --------------------------------------------------------------------------

func TestIsSHA(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ref  string
		want bool
	}{
		{name: "full 40-char SHA", ref: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2", want: true},
		{name: "short 7-char SHA", ref: "abc1234", want: true},
		{name: "uppercase SHA", ref: "ABC1234", want: true},
		{name: "tag name", ref: "v1.0.0", want: false},
		{name: "branch name", ref: "main", want: false},
		{name: "too short", ref: "abc", want: false},
		{name: "empty string", ref: "", want: false},
		{name: "too long", ref: strings.Repeat("a", 41), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := fetcher.IsSHA(tt.ref)
			assert.Equal(t, tt.want, got)
		})
	}
}
