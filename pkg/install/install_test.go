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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/avfs/avfs/vfs/osfs"

	"github.com/retr0h/agentpack/pkg/archive"
	"github.com/retr0h/agentpack/pkg/build"
	"github.com/retr0h/agentpack/pkg/install"
	"github.com/retr0h/agentpack/pkg/metadata"
	"github.com/retr0h/agentpack/pkg/target"
)

// --------------------------------------------------------------------------
// cancelAfterN context helper
// --------------------------------------------------------------------------

type cancelAfterN struct {
	n    int
	call int
}

func newCancelAfterN(n int) *cancelAfterN { return &cancelAfterN{n: n} }

func (c *cancelAfterN) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *cancelAfterN) Done() <-chan struct{}       { return nil }
func (c *cancelAfterN) Value(_ any) any             { return nil }
func (c *cancelAfterN) Err() error {
	c.call++
	if c.call <= c.n {
		return nil
	}

	return errors.New("context canceled")
}

// --------------------------------------------------------------------------
// stub Target
// --------------------------------------------------------------------------

// stubTarget records Install calls and optionally returns an injected error.
type stubTarget struct {
	name        string
	displayName string
	installErr  error
	installedAt string
}

func (s *stubTarget) Name() string                            { return s.name }
func (s *stubTarget) DisplayName() string                     { return s.displayName }
func (s *stubTarget) Detect() bool                            { return true }
func (s *stubTarget) List() ([]target.InstalledPlugin, error) { return nil, nil }
func (s *stubTarget) Install(_ context.Context, opts target.InstallOpts) error {
	s.installedAt = opts.SourceDir

	return s.installErr
}

// --------------------------------------------------------------------------
// helpers
// --------------------------------------------------------------------------

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)

	return hex.EncodeToString(h[:])
}

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

// buildTestArchive creates a .agentpack archive using the build pipeline and
// returns the path to the archive.
func buildTestArchive(t *testing.T, dir string, manifestContent string) string {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, "agentpack.yaml"), []byte(manifestContent), 0o644); err != nil {
		t.Fatalf("write agentpack.yaml: %v", err)
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

// buildArchiveNoChecksums builds an archive with no .agentpack/checksums.txt.
func buildArchiveNoChecksums(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	vfs := osfs.NewWithNoIdm()
	outPath := filepath.Join(dir, "nochecksum.agentpack")

	if err := archive.Create(context.Background(), vfs, outPath, []archive.FileEntry{
		{
			ArchivePath: "skills/intro.md",
			Content:     []byte("# Intro"),
		},
	}); err != nil {
		t.Fatalf("create archive: %v", err)
	}

	return outPath
}

// buildArchiveBadChecksums builds an archive with a malformed checksums.txt.
func buildArchiveBadChecksums(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	vfs := osfs.NewWithNoIdm()
	outPath := filepath.Join(dir, "badchecksum.agentpack")

	if err := archive.Create(context.Background(), vfs, outPath, []archive.FileEntry{
		{
			ArchivePath: ".agentpack/checksums.txt",
			Content:     []byte("badhash file.txt\n"), // missing double-space
		},
	}); err != nil {
		t.Fatalf("create archive: %v", err)
	}

	return outPath
}

// buildArchiveTamperedChecksum builds an archive where the checksum does not
// match the actual file content.
func buildArchiveTamperedChecksum(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	vfs := osfs.NewWithNoIdm()
	outPath := filepath.Join(dir, "tampered.agentpack")
	filePath := "skills/intro.md"
	badHash := strings.Repeat("0", 64)
	checksumContent := fmt.Sprintf("%s  %s\n", badHash, filePath)

	if err := archive.Create(context.Background(), vfs, outPath, []archive.FileEntry{
		{ArchivePath: filePath, Content: []byte("content")},
		{
			ArchivePath: ".agentpack/checksums.txt",
			Content:     []byte(checksumContent),
		},
	}); err != nil {
		t.Fatalf("create archive: %v", err)
	}

	return outPath
}

// buildArchiveNoMetadata builds a valid archive (with checksums.txt) but
// without a metadata.json file.
func buildArchiveNoMetadata(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	vfs := osfs.NewWithNoIdm()
	outPath := filepath.Join(dir, "nometa.agentpack")
	content := []byte("# Intro")
	filePath := "skills/intro.md"
	checksumContent := fmt.Sprintf("%s  %s\n", sha256Hex(content), filePath)

	if err := archive.Create(context.Background(), vfs, outPath, []archive.FileEntry{
		{ArchivePath: filePath, Content: content},
		{
			ArchivePath: ".agentpack/checksums.txt",
			Content:     []byte(checksumContent),
		},
	}); err != nil {
		t.Fatalf("create archive: %v", err)
	}

	return outPath
}

// buildArchiveWithMeta builds an archive that has .agentpack/checksums.txt and
// .agentpack/metadata.json at the top level (new generic format).
func buildArchiveWithMeta(t *testing.T, name, version string) string {
	t.Helper()

	dir := t.TempDir()
	vfs := osfs.NewWithNoIdm()
	outPath := filepath.Join(dir, "withmeta.agentpack")
	content := []byte("# Skill")
	filePath := "skills/intro.md"

	metaJSON, err := json.Marshal(metadata.Metadata{
		Name:           name,
		Version:        version,
		GitCommitSHA:   "abc1234567890",
		BuildTimestamp: "2026-01-01T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("marshal meta: %v", err)
	}

	checksumsContent := fmt.Sprintf(
		"%s  %s\n%s  %s\n",
		sha256Hex(content), filePath,
		sha256Hex(metaJSON), ".agentpack/metadata.json",
	)

	if err := archive.Create(context.Background(), vfs, outPath, []archive.FileEntry{
		{ArchivePath: filePath, Content: content},
		{ArchivePath: ".agentpack/metadata.json", Content: metaJSON},
		{ArchivePath: ".agentpack/checksums.txt", Content: []byte(checksumsContent)},
	}); err != nil {
		t.Fatalf("create archive: %v", err)
	}

	return outPath
}

// --------------------------------------------------------------------------
// TestRun
// --------------------------------------------------------------------------

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setup       func(t *testing.T) (archivePath string, targets []target.Target)
		cancelCtx   bool
		noParallel  bool
		customCtx   context.Context
		injectFuncs func(t *testing.T)
		wantErr     string
		checkResult func(t *testing.T, r *install.Result)
	}{
		{
			name: "installs archive to stub target",
			setup: func(t *testing.T) (string, []target.Target) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: test-plugin
version: "1.0.0"
description: A test plugin
`)
				stub := &stubTarget{name: "test", displayName: "Test"}

				return archivePath, []target.Target{stub}
			},
			checkResult: func(t *testing.T, r *install.Result) {
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
			},
		},
		{
			name: "installs to multiple targets",
			setup: func(t *testing.T) (string, []target.Target) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: multi-plugin
version: "1.0.0"
description: Multi-target plugin
`)
				stubs := []target.Target{
					&stubTarget{name: "alpha", displayName: "Alpha"},
					&stubTarget{name: "beta", displayName: "Beta"},
				}

				return archivePath, stubs
			},
			checkResult: func(t *testing.T, r *install.Result) {
				t.Helper()

				if r.Name != "multi-plugin" {
					t.Errorf("Name = %q, want %q", r.Name, "multi-plugin")
				}
			},
		},
		{
			name: "succeeds with no targets (empty list)",
			setup: func(t *testing.T) (string, []target.Target) {
				t.Helper()
				archivePath := buildArchiveWithMeta(t, "empty-targets-plugin", "1.0.0")

				return archivePath, []target.Target{}
			},
			checkResult: func(t *testing.T, r *install.Result) {
				t.Helper()

				if r.Name != "empty-targets-plugin" {
					t.Errorf("Name = %q, want %q", r.Name, "empty-targets-plugin")
				}
			},
		},
		{
			name: "returns error when source has unknown scheme",
			setup: func(t *testing.T) (string, []target.Target) {
				t.Helper()

				return "gs://my-bucket/plugin.agentpack", nil
			},
			wantErr: "fetcher",
		},
		{
			name: "returns error when archive does not exist",
			setup: func(t *testing.T) (string, []target.Target) {
				t.Helper()

				return "/nonexistent/path.agentpack", nil
			},
			wantErr: "fetch",
		},
		{
			name: "returns error when context is cancelled",
			setup: func(t *testing.T) (string, []target.Target) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: cancel-plugin
version: "1.0.0"
description: Plugin for cancel test
`)

				return archivePath, []target.Target{&stubTarget{name: "stub"}}
			},
			cancelCtx: true,
			wantErr:   "context canceled",
		},
		{
			name: "returns error when context cancelled after fetch",
			setup: func(t *testing.T) (string, []target.Target) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: ctx-after-fetch
version: "1.0.0"
description: ctx after fetch
`)

				return archivePath, []target.Target{&stubTarget{name: "stub"}}
			},
			customCtx: newCancelAfterN(3),
			wantErr:   "context canceled",
		},
		{
			name: "returns error when context cancelled after extract",
			setup: func(t *testing.T) (string, []target.Target) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: ctx-after-extract
version: "1.0.0"
description: ctx after extract
`)

				return archivePath, []target.Target{&stubTarget{name: "stub"}}
			},
			// cancelAfterN(9): calls 1-9 return nil; call 10 fires at the
			// ctx.Err() check immediately after archive.Extract returns (Run line
			// 114). Call path: Run(1) + Fetch(2,3) + Run(4) + Extract-loop(5-9).
			customCtx: newCancelAfterN(9),
			wantErr:   "context canceled",
		},
		{
			name: "returns error when context cancelled inside checksum verification",
			setup: func(t *testing.T) (string, []target.Target) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: ctx-in-verify
version: "1.0.0"
description: ctx in verify
`)

				return archivePath, []target.Target{&stubTarget{name: "stub"}}
			},
			// N=15 is sufficient to pass the initial ctx checks and fetch but
			// fires somewhere inside verify or shortly after — the exact call
			// count depends on archive size. We verify cancellation propagates.
			customCtx: newCancelAfterN(15),
			wantErr:   "context canceled",
		},
		{
			name:       "returns error when os.CreateTemp fails",
			noParallel: true,
			setup: func(t *testing.T) (string, []target.Target) {
				t.Helper()

				return "/some/path.agentpack", nil
			},
			injectFuncs: func(t *testing.T) {
				t.Helper()
				restore := install.SetOsCreateTemp(install.CreateTempAlwaysFails)
				t.Cleanup(restore)
			},
			wantErr: "create temp file",
		},
		{
			name:       "returns error when os.MkdirTemp fails",
			noParallel: true,
			setup: func(t *testing.T) (string, []target.Target) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: mkdir-test-plugin
version: "1.0.0"
description: Plugin for mkdir temp test
`)

				return archivePath, nil
			},
			injectFuncs: func(t *testing.T) {
				t.Helper()
				restore := install.SetOsMkdirTemp(install.MkdirTempAlwaysFails)
				t.Cleanup(restore)
			},
			wantErr: "create temp dir",
		},
		{
			name: "returns error when archive is corrupt",
			setup: func(t *testing.T) (string, []target.Target) {
				t.Helper()
				dir := t.TempDir()
				archivePath := filepath.Join(dir, "corrupt.agentpack")

				if err := os.WriteFile(archivePath, []byte("not gzip data"), 0o644); err != nil {
					t.Fatalf("write: %v", err)
				}

				return archivePath, nil
			},
			wantErr: "extract",
		},
		{
			name: "returns error when archive has no checksums.txt",
			setup: func(t *testing.T) (string, []target.Target) {
				t.Helper()

				return buildArchiveNoChecksums(t), nil
			},
			wantErr: "checksums.txt not found",
		},
		{
			name: "returns error when archive has bad checksums format",
			setup: func(t *testing.T) (string, []target.Target) {
				t.Helper()

				return buildArchiveBadChecksums(t), nil
			},
			wantErr: "reading checksums",
		},
		{
			name: "returns error when archive checksum fails",
			setup: func(t *testing.T) (string, []target.Target) {
				t.Helper()

				return buildArchiveTamperedChecksum(t), nil
			},
			wantErr: "checksum failed",
		},
		{
			name: "returns error when archive has no metadata.json",
			setup: func(t *testing.T) (string, []target.Target) {
				t.Helper()

				return buildArchiveNoMetadata(t), nil
			},
			wantErr: "metadata.json not found",
		},
		{
			name: "returns error when target Install fails",
			setup: func(t *testing.T) (string, []target.Target) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: tgt-fail-plugin
version: "1.0.0"
description: target fail test
`)
				stub := &stubTarget{
					name:       "fail-target",
					installErr: errors.New("target install error"),
				}

				return archivePath, []target.Target{stub}
			},
			wantErr: "install to fail-target",
		},
		{
			name:       "returns error when copyToTemp MkdirTemp fails",
			noParallel: true,
			setup: func(t *testing.T) (string, []target.Target) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: copytotemp-mkdir-plugin
version: "1.0.0"
description: test copyToTemp mkdir fail
`)
				return archivePath, []target.Target{&stubTarget{name: "stub"}}
			},
			injectFuncs: func(t *testing.T) {
				t.Helper()
				// Succeed on the first osMkdirTemp call (extract temp dir) and
				// fail on the second (inside copyToTemp).
				restore := install.SetOsMkdirTemp(install.MkdirTempFailAfterN(1))
				t.Cleanup(restore)
			},
			wantErr: "create target temp dir",
		},
		{
			name: "returns error when context cancelled during checksum verify",
			setup: func(t *testing.T) (string, []target.Target) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: ctx-verify-plugin
version: "1.0.0"
description: ctx during verify
`)
				return archivePath, []target.Target{&stubTarget{name: "stub"}}
			},
			// cancelAfterN(10): the first 10 ctx.Err() calls return nil.
			// Call 11 fires inside checksum.Verify (first entry check), causing
			// Verify to return (nil, err) → line 131 "verify: %w".
			// Call path: Run(1) + Fetch(2,3) + Run(4) + Extract(5-9) + Run(10)
			// + Verify entry 1 = call 11.
			customCtx: newCancelAfterN(10),
			wantErr:   "context canceled",
		},
		{
			name: "returns error when context cancelled after metadata read",
			setup: func(t *testing.T) (string, []target.Target) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: ctx-post-meta-plugin
version: "1.0.0"
description: ctx after metadata
`)
				return archivePath, []target.Target{&stubTarget{name: "stub"}}
			},
			// cancelAfterN(12): calls 11-12 are the two Verify entries (which
			// succeed). Call 13 fires at Run line 146 ctx.Err() check after
			// findAndReadMetadata completes.
			customCtx: newCancelAfterN(12),
			wantErr:   "context canceled",
		},
		{
			name: "returns error when context cancelled inside targets loop",
			setup: func(t *testing.T) (string, []target.Target) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: ctx-loop-plugin
version: "1.0.0"
description: ctx inside targets loop
`)
				return archivePath, []target.Target{&stubTarget{name: "stub"}}
			},
			// cancelAfterN(13): call 13 is the Run line 146 check (passes nil).
			// Call 14 fires at Run line 165 (top of targets for-loop).
			customCtx: newCancelAfterN(13),
			wantErr:   "context canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.noParallel {
				t.Parallel()
			}

			if tt.injectFuncs != nil {
				tt.injectFuncs(t)
			}

			archivePath, targets := tt.setup(t)

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

			r, err := install.Run(ctx, install.Options{
				Source:  archivePath,
				Targets: targets,
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
				tt.checkResult(t, r)
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
			name: "returns first 7 chars of long SHA",
			sha:  "a1b2c3d4e5f6789abcdef",
			want: "a1b2c3d",
		},
		{
			name: "returns full string when exactly 7 chars",
			sha:  "a1b2c3d",
			want: "a1b2c3d",
		},
		{
			name: "returns full string when shorter than 7 chars",
			sha:  "abc",
			want: "abc",
		},
		{
			name: "returns empty string when empty",
			sha:  "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := install.ShortSHA(tt.sha)

			if got != tt.want {
				t.Errorf("ShortSHA(%q) = %q, want %q", tt.sha, got, tt.want)
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
		setup   func(t *testing.T) (src string, dst string)
		wantErr string
		check   func(t *testing.T, dst string)
	}{
		{
			name: "copies file content and mode",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				src := filepath.Join(dir, "src.txt")

				if err := os.WriteFile(src, []byte("hello copy"), 0o644); err != nil {
					t.Fatalf("write src: %v", err)
				}

				return src, filepath.Join(dir, "dst.txt")
			},
			check: func(t *testing.T, dst string) {
				t.Helper()
				data, err := os.ReadFile(dst)

				if err != nil {
					t.Fatalf("read dst: %v", err)
				}

				if string(data) != "hello copy" {
					t.Errorf("dst content = %q, want %q", string(data), "hello copy")
				}
			},
		},
		{
			name: "returns error when src does not exist",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()

				return filepath.Join(dir, "nonexistent.txt"), filepath.Join(dir, "dst.txt")
			},
			wantErr: "read",
		},
		{
			name: "returns error when dst dir does not exist",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				src := filepath.Join(dir, "src.txt")

				if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
					t.Fatalf("write src: %v", err)
				}

				return src, filepath.Join(dir, "nonexistent", "dst.txt")
			},
			wantErr: "write",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src, dst := tt.setup(t)
			err := install.CopyFile(src, dst)

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
// TestCopyDir
// --------------------------------------------------------------------------

func TestCopyDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(t *testing.T) (src string, dst string)
		cancelCtx bool
		wantErr   string
		check     func(t *testing.T, dst string)
	}{
		{
			name: "copies directory tree recursively",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				src := filepath.Join(dir, "src")

				if err := os.MkdirAll(filepath.Join(src, "subdir"), 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}

				if err := os.WriteFile(filepath.Join(src, "file.txt"), []byte("root"), 0o644); err != nil {
					t.Fatalf("write root file: %v", err)
				}

				if err := os.WriteFile(filepath.Join(src, "subdir", "nested.txt"), []byte("nested"), 0o644); err != nil {
					t.Fatalf("write nested file: %v", err)
				}

				return src, filepath.Join(dir, "dst")
			},
			check: func(t *testing.T, dst string) {
				t.Helper()
				data, err := os.ReadFile(filepath.Join(dst, "file.txt"))

				if err != nil {
					t.Fatalf("read file.txt: %v", err)
				}

				if string(data) != "root" {
					t.Errorf("file.txt = %q, want %q", string(data), "root")
				}

				data2, err := os.ReadFile(filepath.Join(dst, "subdir", "nested.txt"))

				if err != nil {
					t.Fatalf("read nested.txt: %v", err)
				}

				if string(data2) != "nested" {
					t.Errorf("nested.txt = %q, want %q", string(data2), "nested")
				}
			},
		},
		{
			name: "returns error when context is cancelled",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				src := filepath.Join(dir, "src")

				if err := os.MkdirAll(src, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}

				if err := os.WriteFile(filepath.Join(src, "file.txt"), []byte("data"), 0o644); err != nil {
					t.Fatalf("write file: %v", err)
				}

				return src, filepath.Join(dir, "dst")
			},
			cancelCtx: true,
			wantErr:   "context canceled",
		},
		{
			name: "returns error when walkdir encounters permission denied in src",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				src := filepath.Join(dir, "src")
				subdir := filepath.Join(src, "subdir")

				if err := os.MkdirAll(subdir, 0o755); err != nil {
					t.Fatalf("mkdir subdir: %v", err)
				}

				if err := os.WriteFile(filepath.Join(subdir, "file.txt"), []byte("x"), 0o644); err != nil {
					t.Fatalf("write: %v", err)
				}

				if err := os.Chmod(subdir, 0o000); err != nil {
					t.Fatalf("chmod: %v", err)
				}

				t.Cleanup(func() { _ = os.Chmod(subdir, 0o755) })

				return src, filepath.Join(dir, "dst")
			},
			wantErr: "permission denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src, dst := tt.setup(t)

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

			err := install.CopyDir(ctx, src, dst)

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
// TestFindChecksums
// --------------------------------------------------------------------------

func TestFindChecksums(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		wantErr string
		check   func(t *testing.T, path string)
	}{
		{
			name: "finds checksums.txt inside .agentpack dir",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				agentpackDir := filepath.Join(dir, ".agentpack")

				if err := os.MkdirAll(agentpackDir, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}

				if err := os.WriteFile(
					filepath.Join(agentpackDir, "checksums.txt"),
					[]byte("hash  file.txt\n"),
					0o644,
				); err != nil {
					t.Fatalf("write checksums.txt: %v", err)
				}

				return dir
			},
			check: func(t *testing.T, path string) {
				t.Helper()

				if path == "" {
					t.Error("expected non-empty path")
				}

				if !strings.HasSuffix(path, "checksums.txt") {
					t.Errorf("path %q does not end in checksums.txt", path)
				}
			},
		},
		{
			name: "returns error when checksums.txt not found",
			setup: func(t *testing.T) string {
				t.Helper()

				return t.TempDir()
			},
			wantErr: "checksums.txt not found",
		},
		{
			name: "returns error when walkdir encounters permission denied",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				locked := filepath.Join(dir, "locked")

				if err := os.MkdirAll(locked, 0o000); err != nil {
					t.Fatalf("mkdir: %v", err)
				}

				t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

				return dir
			},
			wantErr: "searching for checksums.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := tt.setup(t)
			path, err := install.FindChecksums(dir)

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
				tt.check(t, path)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestFindAndReadMetadata
// --------------------------------------------------------------------------

func TestFindAndReadMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		wantErr string
		check   func(t *testing.T, m any)
	}{
		{
			name: "finds and reads metadata.json inside .agentpack dir",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				agentpackDir := filepath.Join(dir, ".agentpack")

				if err := os.MkdirAll(agentpackDir, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}

				meta := metadata.Metadata{
					Name:    "my-plugin",
					Version: "1.0.0",
				}
				data, err := json.Marshal(meta)

				if err != nil {
					t.Fatalf("marshal: %v", err)
				}

				if err := os.WriteFile(
					filepath.Join(agentpackDir, "metadata.json"), data, 0o644,
				); err != nil {
					t.Fatalf("write metadata.json: %v", err)
				}

				return dir
			},
			check: func(t *testing.T, m any) {
				t.Helper()

				if m == nil {
					t.Error("expected non-nil metadata")
				}
			},
		},
		{
			name: "returns error when metadata.json not found",
			setup: func(t *testing.T) string {
				t.Helper()

				return t.TempDir()
			},
			wantErr: "metadata.json not found",
		},
		{
			name: "returns error when metadata.json is invalid JSON",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				agentpackDir := filepath.Join(dir, ".agentpack")

				if err := os.MkdirAll(agentpackDir, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}

				if err := os.WriteFile(
					filepath.Join(agentpackDir, "metadata.json"),
					[]byte("not json {{{"),
					0o644,
				); err != nil {
					t.Fatalf("write: %v", err)
				}

				return dir
			},
			wantErr: "parse metadata.json",
		},
		{
			name: "returns error when walkdir encounters permission denied",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				locked := filepath.Join(dir, "locked")

				if err := os.MkdirAll(locked, 0o000); err != nil {
					t.Fatalf("mkdir: %v", err)
				}

				t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

				return dir
			},
			wantErr: "searching for metadata.json",
		},
		{
			name: "returns error when metadata.json cannot be read",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				agentpackDir := filepath.Join(dir, ".agentpack")

				if err := os.MkdirAll(agentpackDir, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}

				metaPath := filepath.Join(agentpackDir, "metadata.json")

				if err := os.WriteFile(metaPath, []byte(`{"name":"p"}`), 0o000); err != nil {
					t.Fatalf("write: %v", err)
				}

				t.Cleanup(func() { _ = os.Chmod(metaPath, 0o644) })

				return dir
			},
			wantErr: "read metadata.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := tt.setup(t)
			m, err := install.FindAndReadMetadata(dir)

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
				tt.check(t, m)
			}
		})
	}
}
