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

	"github.com/retr0h/agentpack/pkg/fetcher"
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
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// Working repo.
	workDir := t.TempDir()
	run(workDir, "init")
	run(workDir, "checkout", "-b", "main")

	if err := os.WriteFile(
		filepath.Join(workDir, "README.md"),
		[]byte("# test repo\n"),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile README.md: %v", err)
	}

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
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	workDir := t.TempDir()
	run(workDir, "init")
	run(workDir, "checkout", "-b", "main")

	if err := os.WriteFile(
		filepath.Join(workDir, "README.md"),
		[]byte("# test repo\n"),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile README.md: %v", err)
	}

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
				if _, err := os.Stat(filepath.Join(dest, "README.md")); err != nil {
					t.Errorf("README.md not found: %v", err)
				}
				if _, err := os.Stat(filepath.Join(dest, ".git")); err == nil {
					t.Error(".git directory should not be present in dest")
				}
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
				if _, err := os.Stat(filepath.Join(dest, "README.md")); err != nil {
					t.Errorf("README.md not found after tag clone: %v", err)
				}
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
				tt.check(t, dest)
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

			if rawURL != tt.wantRawURL {
				t.Errorf("rawURL = %q, want %q", rawURL, tt.wantRawURL)
			}

			if ref != tt.wantRef {
				t.Errorf("ref = %q, want %q", ref, tt.wantRef)
			}
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
			if got != tt.wantURL {
				t.Errorf("ToGitURL(%q) = %q, want %q", tt.rawURL, got, tt.wantURL)
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
			if got != tt.want {
				t.Errorf("IsSHA(%q) = %v, want %v", tt.ref, got, tt.want)
			}
		})
	}
}
