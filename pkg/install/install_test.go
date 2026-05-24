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
	"archive/tar"
	"compress/gzip"
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
)

// --------------------------------------------------------------------------
// cancelAfterN context helper
// --------------------------------------------------------------------------

// cancelAfterN returns nil from Err() for the first n calls, then a
// "context canceled" error on all subsequent calls. Done() returns nil so the
// HTTP client and OS calls are not cancelled via channel — only explicit
// ctx.Err() checks are affected.
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
// Archive builder helpers for specific error paths
// --------------------------------------------------------------------------

// sha256Hex returns the hex SHA256 of data.
func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// buildArchiveNoChecksums builds a .agentpack archive with no checksums.txt.
func buildArchiveNoChecksums(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	vfs := osfs.NewWithNoIdm()
	outPath := filepath.Join(dir, "nochecksum.agentpack")
	if err := archive.Create(context.Background(), vfs, outPath, []archive.FileEntry{
		{
			ArchivePath: "marketplaces/my-plugin/skills/intro.md",
			Content:     []byte("# Intro"),
		},
	}); err != nil {
		t.Fatalf("create archive: %v", err)
	}
	return outPath
}

// buildArchiveBadChecksums builds a .agentpack archive with a malformed checksums.txt.
func buildArchiveBadChecksums(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	vfs := osfs.NewWithNoIdm()
	outPath := filepath.Join(dir, "badchecksum.agentpack")
	if err := archive.Create(context.Background(), vfs, outPath, []archive.FileEntry{
		{
			ArchivePath: "marketplaces/my-plugin/.agentpack/checksums.txt",
			Content:     []byte("badhash file.txt\n"), // missing double-space
		},
	}); err != nil {
		t.Fatalf("create archive: %v", err)
	}
	return outPath
}

// buildArchiveTamperedChecksum builds a .agentpack archive where the checksum
// does not match the actual file content.
func buildArchiveTamperedChecksum(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	vfs := osfs.NewWithNoIdm()
	outPath := filepath.Join(dir, "tampered.agentpack")
	filePath := "marketplaces/my-plugin/skills/intro.md"
	badHash := strings.Repeat("0", 64)
	checksumContent := fmt.Sprintf("%s  %s\n", badHash, filePath)
	if err := archive.Create(context.Background(), vfs, outPath, []archive.FileEntry{
		{ArchivePath: filePath, Content: []byte("content")},
		{
			ArchivePath: "marketplaces/my-plugin/.agentpack/checksums.txt",
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
	filePath := "marketplaces/my-plugin/skills/intro.md"
	checksumContent := fmt.Sprintf("%s  %s\n", sha256Hex(content), filePath)
	if err := archive.Create(context.Background(), vfs, outPath, []archive.FileEntry{
		{ArchivePath: filePath, Content: content},
		{
			ArchivePath: "marketplaces/my-plugin/.agentpack/checksums.txt",
			Content:     []byte(checksumContent),
		},
	}); err != nil {
		t.Fatalf("create archive: %v", err)
	}
	return outPath
}

// buildArchiveNoMarketplace builds an archive where a .agentpack/checksums.txt
// and .agentpack/metadata.json exist but there is no marketplaces/ directory,
// triggering findMarketplaceDir's "read marketplaces dir" error which is
// caught and returned as "no marketplace directory found" from Run.
//
// NOTE: findMarketplaceDir returns "read marketplaces dir" error when the
// directory doesn't exist. We test that error message below.
func buildArchiveNoMarketplace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	outPath := filepath.Join(dir, "nomarketplace.agentpack")
	f, err := os.Create(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	gw := gzip.NewWriter(f)
	defer func() { _ = gw.Close() }()
	tw := tar.NewWriter(gw)
	defer func() { _ = tw.Close() }()

	content := []byte("content")
	hash := sha256Hex(content)
	// Place files directly at top-level .agentpack/ — no marketplaces/ at all.
	filePath := ".agentpack/file.txt"
	checksumLine := fmt.Sprintf("%s  %s\n", hash, filePath)

	metaJSON, _ := json.Marshal(metadata.Metadata{
		Name: "my-plugin", Version: "1.0.0",
		GitCommitSHA: "abc1234567890", BuildTimestamp: "2026-01-01T00:00:00Z",
	})

	for _, e := range []struct {
		name    string
		content []byte
	}{
		{filePath, content},
		{".agentpack/checksums.txt", []byte(checksumLine)},
		{".agentpack/metadata.json", metaJSON},
	} {
		hdr := &tar.Header{
			Name:     e.name,
			Size:     int64(len(e.content)),
			Mode:     0o644,
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(e.content); err != nil {
			t.Fatal(err)
		}
	}
	return outPath
}

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

// buildTestArchive creates a .agentpack archive in dir using the build pipeline
// and returns the path to the archive.
func buildTestArchive(t *testing.T, dir string, manifest string) string {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, "agentpack.yaml"), []byte(manifest), 0o644); err != nil {
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

// --------------------------------------------------------------------------
// Run
// --------------------------------------------------------------------------

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setup       func(t *testing.T) (archivePath string, pluginDir string)
		cancelCtx   bool
		noParallel  bool // set for tests that mutate package-level state
		setupRename func() func()
		injectFuncs func(t *testing.T) // if set, swap package vars (not parallel-safe)
		customCtx   context.Context    // if set, use this instead of background/cancelled
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
				metaPath := filepath.Join(r.Dir, ".agentpack", "metadata.json")
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
			name: "returns error when source has unknown scheme",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return "gs://my-bucket/plugin.agentpack", t.TempDir()
			},
			wantErr: "fetcher",
		},
		{
			name: "returns error when archive does not exist",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return "/nonexistent/path.agentpack", t.TempDir()
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
		{
			name: "returns error when context cancelled after fetch",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: ctx-after-fetch-plugin
version: "1.0.0"
description: Plugin for ctx after fetch test
`)
				return archivePath, t.TempDir()
			},
			// Call 1: Run line 66, calls 2-3: FileFetcher (2 ctx checks),
			// call 4: Run line 88 → first failure here.
			customCtx: newCancelAfterN(3),
			wantErr:   "context canceled",
		},
		{
			name: "returns error when context cancelled after extract",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: ctx-after-extract-plugin
version: "1.0.0"
description: Plugin for ctx after extract test
`)
				return archivePath, t.TempDir()
			},
			// Calls 1-3: Run+FileFetcher, call 4: line 94, calls 5-14: Extract
			// (9 entries + EOF = 10 calls), call 15: Run line 109.
			customCtx: newCancelAfterN(14),
			wantErr:   "context canceled",
		},
		{
			name: "returns error when checksum.Verify fails due to cancelled context",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: ctx-in-verify-plugin
version: "1.0.0"
description: Plugin for ctx in verify test
`)
				return archivePath, t.TempDir()
			},
			// Calls 1-14: Run+FileFetcher+Extract, call 15: line 109, N=15 means
			// call 16 fires inside checksum.Verify, which returns a non-nil error
			// causing Run to return "verify: <err>".
			customCtx: newCancelAfterN(15),
			wantErr:   "verify",
		},
		{
			name: "returns error when context cancelled after metadata read",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: ctx-after-meta-plugin
version: "1.0.0"
description: Plugin for ctx after meta test
`)
				return archivePath, t.TempDir()
			},
			// N=1000 is large enough to pass all intermediate checks and only fail
			// at the ctx.Err() check after metadata read (line 135). If N is too
			// large, the test completes successfully with nil error. Use N that
			// ensures exactly line 135 fires. The exact count is environment-
			// dependent due to how many entries are in the archive. We use a
			// cancelAfterN approach but since the exact count varies, we instead
			// rely on the ctx.Err() check at line 135 being reached after Verify
			// completes. Use 40 as upper bound (empirically tested).
			customCtx: newCancelAfterN(19),
			wantErr:   "context canceled",
		},
		{
			name:       "returns error when os.CreateTemp fails",
			noParallel: true,
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return "/some/path.agentpack", t.TempDir()
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
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: mkdir-test-plugin
version: "1.0.0"
description: Plugin for mkdir temp test
`)
				return archivePath, t.TempDir()
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
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				// Write non-gzip data to make archive.Extract fail.
				archivePath := filepath.Join(dir, "corrupt.agentpack")
				if err := os.WriteFile(archivePath, []byte("not gzip data"), 0o644); err != nil {
					t.Fatalf("write: %v", err)
				}
				return archivePath, t.TempDir()
			},
			wantErr: "extract",
		},
		{
			name: "returns error when archive has no checksums.txt",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return buildArchiveNoChecksums(t), t.TempDir()
			},
			wantErr: "checksums.txt not found",
		},
		{
			name: "returns error when archive has bad checksums format",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return buildArchiveBadChecksums(t), t.TempDir()
			},
			wantErr: "reading checksums",
		},
		{
			name: "returns error when archive checksum fails",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return buildArchiveTamperedChecksum(t), t.TempDir()
			},
			wantErr: "checksum failed",
		},
		{
			name: "returns error when archive has no metadata.json",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return buildArchiveNoMetadata(t), t.TempDir()
			},
			wantErr: "metadata.json not found",
		},
		{
			name: "returns error when archive has no marketplace directory",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return buildArchiveNoMarketplace(t), t.TempDir()
			},
			wantErr: "read marketplaces dir",
		},
		{
			name:       "returns error when os.RemoveAll fails",
			noParallel: true,
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: removeall-plugin
version: "1.0.0"
description: Plugin for removeall test
`)
				return archivePath, t.TempDir()
			},
			injectFuncs: func(t *testing.T) {
				t.Helper()
				restore := install.SetOsRemoveAll(install.RemoveAllAlwaysFails)
				t.Cleanup(restore)
			},
			wantErr: "remove existing",
		},
		{
			name:       "returns error when MkdirAll for plugin dir fails",
			noParallel: true,
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: mkdir-plugin
version: "1.0.0"
description: Plugin for mkdirall test
`)
				return archivePath, t.TempDir()
			},
			injectFuncs: func(t *testing.T) {
				t.Helper()
				restore := install.SetOsMkdirAll(install.MkdirAllAlwaysFails)
				t.Cleanup(restore)
			},
			wantErr: "mkdir plugin dir",
		},
		{
			name:       "returns error when both rename and copyDir fail",
			noParallel: true,
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: both-fail-plugin
version: "1.0.0"
description: Plugin for both-fail test
`)
				// Create a plugin dir that exists but is read-only so copyDir
				// fails when trying to write to the dest.
				pluginDir := t.TempDir()
				destParent := filepath.Join(pluginDir, "marketplaces")
				if err := os.MkdirAll(destParent, 0o555); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				t.Cleanup(func() { _ = os.Chmod(destParent, 0o755) })
				return archivePath, pluginDir
			},
			setupRename: func() func() { return install.SetRenameFunc(install.RenameAlwaysFails) },
			wantErr:     "install",
		},
		{
			name: "falls back to copyDir when rename fails",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: copy-plugin
version: "1.0.0"
description: Plugin for copy fallback test
`)
				pluginDir := t.TempDir()
				return archivePath, pluginDir
			},
			noParallel:  true,
			setupRename: func() func() { return install.SetRenameFunc(install.RenameAlwaysFails) },
			checkResult: func(t *testing.T, r *install.Result, _ string) {
				t.Helper()
				if r.Name != "copy-plugin" {
					t.Errorf("Name = %q, want %q", r.Name, "copy-plugin")
				}
				if r.Version != "1.0.0" {
					t.Errorf("Version = %q, want %q", r.Version, "1.0.0")
				}
				// Verify the plugin directory actually exists (was copied).
				if _, err := os.Stat(r.Dir); err != nil {
					t.Errorf("plugin dir not found: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.noParallel {
				t.Parallel()
			}

			if tt.setupRename != nil {
				restore := tt.setupRename()
				defer restore()
			}

			if tt.injectFuncs != nil {
				tt.injectFuncs(t)
			}

			archivePath, pluginDir := tt.setup(t)

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
				// Make subdir unreadable so WalkDir callback receives a non-nil err.
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

func TestFindChecksums(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) string // returns dir
		wantErr string
		check   func(t *testing.T, path string)
	}{
		{
			name: "finds checksums.txt inside .agentpack dir",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				agentpackDir := filepath.Join(dir, "marketplaces", "my-plugin", ".agentpack")
				if err := os.MkdirAll(agentpackDir, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				checksumPath := filepath.Join(agentpackDir, "checksums.txt")
				if err := os.WriteFile(checksumPath, []byte("hash  file.txt\n"), 0o644); err != nil {
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

func TestFindAndReadMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) string // returns dir
		wantErr string
		check   func(t *testing.T, m any)
	}{
		{
			name: "finds and reads metadata.json inside .agentpack dir",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				agentpackDir := filepath.Join(dir, "marketplaces", "my-plugin", ".agentpack")
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
				if err := os.WriteFile(filepath.Join(agentpackDir, "metadata.json"), data, 0o644); err != nil {
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
				agentpackDir := filepath.Join(dir, "marketplaces", "my-plugin", ".agentpack")
				if err := os.MkdirAll(agentpackDir, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.WriteFile(filepath.Join(agentpackDir, "metadata.json"), []byte("not json {{{"), 0o644); err != nil {
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
				agentpackDir := filepath.Join(dir, "marketplaces", "my-plugin", ".agentpack")
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

func TestFindMarketplaceDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) string // returns dir
		wantErr string
		check   func(t *testing.T, path string)
	}{
		{
			name: "finds marketplace subdir",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				mpDir := filepath.Join(dir, "marketplaces", "my-plugin")
				if err := os.MkdirAll(mpDir, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				return dir
			},
			check: func(t *testing.T, path string) {
				t.Helper()
				if path == "" {
					t.Error("expected non-empty path")
				}
				if !strings.HasSuffix(path, "my-plugin") {
					t.Errorf("path %q does not end in my-plugin", path)
				}
			},
		},
		{
			name: "returns error when marketplaces dir does not exist",
			setup: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			wantErr: "read marketplaces dir",
		},
		{
			name: "returns error when no subdirectory in marketplaces",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				mpDir := filepath.Join(dir, "marketplaces")
				if err := os.MkdirAll(mpDir, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				// Create a file (not a dir) inside marketplaces.
				if err := os.WriteFile(filepath.Join(mpDir, "not-a-dir.txt"), []byte("x"), 0o644); err != nil {
					t.Fatalf("write: %v", err)
				}
				return dir
			},
			wantErr: "no marketplace directory found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := tt.setup(t)
			path, err := install.FindMarketplaceDir(dir)

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
