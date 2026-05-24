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

package verify_test

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/avfs/avfs/vfs/osfs"

	"github.com/retr0h/agentpack/pkg/archive"
	"github.com/retr0h/agentpack/pkg/verify"
)

// cancelAfterN returns nil from Err() for the first n calls, then returns a
// "context canceled" error. This allows tests to pass early ctx checks and
// trigger cancellation at a specific point inside the function.
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
// FindChecksums
// --------------------------------------------------------------------------

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
				if err := os.WriteFile(
					filepath.Join(agentpackDir, "checksums.txt"),
					[]byte("hash  file.txt\n"),
					0o644,
				); err != nil {
					t.Fatalf("write: %v", err)
				}
				return dir
			},
			check: func(t *testing.T, path string) {
				t.Helper()
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
			name: "returns error when dir entry callback gets an error",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				// Create a subdirectory we cannot read (permission denied).
				subdir := filepath.Join(dir, "locked")
				if err := os.MkdirAll(subdir, 0o000); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				t.Cleanup(func() { _ = os.Chmod(subdir, 0o755) })
				return dir
			},
			wantErr: "searching for checksums.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := tt.setup(t)
			path, err := verify.FindChecksums(dir)

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
// Helpers
// --------------------------------------------------------------------------

// sha256Hex returns the hex-encoded SHA256 of data.
func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// buildValidArchive creates a .agentpack archive with one file and a valid
// checksums.txt, returning the archive path.
func buildValidArchive(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	vfs := osfs.NewWithNoIdm()

	content := []byte("hello agentpack")
	filePath := "marketplaces/my-plugin/skills/intro.md"
	checksumPath := "marketplaces/my-plugin/.agentpack/checksums.txt"

	skillHashStr := sha256Hex(content)
	checksumLine := fmt.Sprintf("%s  %s\n", skillHashStr, filePath)

	// The checksums.txt also checksums itself, but that creates a circular
	// dependency. In practice agentpack.yaml archives store checksums for the
	// content files only — the checksums.txt file is excluded from its own
	// list. We keep it simple here: one file, one checksum entry.
	checksumContent := checksumLine

	outPath := filepath.Join(dir, "test.agentpack")
	if err := archive.Create(context.Background(), vfs, outPath, []archive.FileEntry{
		{ArchivePath: filePath, Content: content},
		{ArchivePath: checksumPath, Content: []byte(checksumContent)},
	}); err != nil {
		t.Fatalf("create archive: %v", err)
	}

	return outPath
}

// buildArchiveWithTamperedFile creates an archive where a file's content
// doesn't match the checksum recorded in checksums.txt.
func buildArchiveWithTamperedFile(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	vfs := osfs.NewWithNoIdm()

	content := []byte("original content")
	filePath := "marketplaces/my-plugin/skills/intro.md"
	checksumPath := "marketplaces/my-plugin/.agentpack/checksums.txt"

	// Record a wrong hash to simulate tampering.
	badHash := strings.Repeat("0", 64)
	checksumContent := fmt.Sprintf("%s  %s\n", badHash, filePath)

	outPath := filepath.Join(dir, "tampered.agentpack")
	if err := archive.Create(context.Background(), vfs, outPath, []archive.FileEntry{
		{ArchivePath: filePath, Content: content},
		{ArchivePath: checksumPath, Content: []byte(checksumContent)},
	}); err != nil {
		t.Fatalf("create archive: %v", err)
	}

	return outPath
}

// buildArchiveWithoutChecksums creates an archive with no checksums.txt.
func buildArchiveWithoutChecksums(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	vfs := osfs.NewWithNoIdm()

	outPath := filepath.Join(dir, "nochecksum.agentpack")
	if err := archive.Create(context.Background(), vfs, outPath, []archive.FileEntry{
		{ArchivePath: "marketplaces/my-plugin/skills/intro.md", Content: []byte("content")},
	}); err != nil {
		t.Fatalf("create archive: %v", err)
	}

	return outPath
}

// buildArchiveWithBadChecksumsFormat creates an archive whose checksums.txt
// has invalid format lines (missing double-space separator).
func buildArchiveWithBadChecksumsFormat(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	vfs := osfs.NewWithNoIdm()

	outPath := filepath.Join(dir, "badformat.agentpack")
	if err := archive.Create(context.Background(), vfs, outPath, []archive.FileEntry{
		{
			ArchivePath: "marketplaces/my-plugin/.agentpack/checksums.txt",
			Content:     []byte("badhash file.txt\n"), // missing double-space separator
		},
	}); err != nil {
		t.Fatalf("create archive: %v", err)
	}

	return outPath
}

// buildTarWithSymlink creates a raw tarball containing a symlink entry
// (which archive.Extract rejects).
func buildTarWithSymlink(t *testing.T) string {
	t.Helper()

	outPath := filepath.Join(t.TempDir(), "symlink.agentpack")
	f, err := os.Create(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	gw := gzip.NewWriter(f)
	defer func() { _ = gw.Close() }()

	tw := tar.NewWriter(gw)
	defer func() { _ = tw.Close() }()

	if err := tw.WriteHeader(&tar.Header{
		Name:     "evil-link",
		Typeflag: tar.TypeSymlink,
		Linkname: "/etc/passwd",
	}); err != nil {
		t.Fatal(err)
	}

	return outPath
}

// --------------------------------------------------------------------------
// Run
// --------------------------------------------------------------------------

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		archivePath func(t *testing.T) string
		ctx         func() context.Context
		injectFuncs func(t *testing.T) // if set, swap package vars (not parallel-safe)
		noParallel  bool               // if true, skip t.Parallel()
		wantErr     string
		checkResult func(t *testing.T, r *verify.Result)
	}{
		{
			name:        "valid archive passes verification",
			archivePath: buildValidArchive,
			ctx:         func() context.Context { return context.Background() },
			checkResult: func(t *testing.T, r *verify.Result) {
				t.Helper()
				if r == nil {
					t.Fatal("result is nil")
				}
				if r.ArchiveName == "" {
					t.Error("ArchiveName is empty")
				}
				for _, f := range r.Files {
					if !f.OK {
						t.Errorf("file %q failed: %s", f.Path, f.Err)
					}
				}
			},
		},
		{
			name:        "tampered file fails verification",
			archivePath: buildArchiveWithTamperedFile,
			ctx:         func() context.Context { return context.Background() },
			checkResult: func(t *testing.T, r *verify.Result) {
				t.Helper()
				if r == nil {
					t.Fatal("result is nil")
				}
				failed := 0
				for _, f := range r.Files {
					if !f.OK {
						failed++
					}
				}
				if failed == 0 {
					t.Error("expected at least one failed file, got none")
				}
			},
		},
		{
			name:        "archive without checksums.txt returns error",
			archivePath: buildArchiveWithoutChecksums,
			ctx:         func() context.Context { return context.Background() },
			wantErr:     "checksums.txt not found",
		},
		{
			name:        "malformed archive returns extract error",
			archivePath: func(t *testing.T) string { return filepath.Join(t.TempDir(), "nonexistent.agentpack") },
			ctx:         func() context.Context { return context.Background() },
			wantErr:     "extract",
		},
		{
			name:        "archive with symlinks rejected during extract",
			archivePath: buildTarWithSymlink,
			ctx:         func() context.Context { return context.Background() },
			wantErr:     "extract",
		},
		{
			name:        "bad checksums format returns error",
			archivePath: buildArchiveWithBadChecksumsFormat,
			ctx:         func() context.Context { return context.Background() },
			wantErr:     "reading checksums",
		},
		{
			name:       "returns error when MkdirTemp fails",
			noParallel: true,
			archivePath: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "unused.agentpack")
			},
			ctx: func() context.Context { return context.Background() },
			injectFuncs: func(t *testing.T) {
				t.Helper()
				restore := verify.SetOsMkdirTemp(verify.MkdirTempAlwaysFails)
				t.Cleanup(restore)
			},
			wantErr: "create temp dir",
		},
		{
			name:        "cancelled context returns error before extract",
			archivePath: buildValidArchive,
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			wantErr: "context canceled",
		},
		{
			name:        "cancelled context returns error after extract",
			archivePath: buildValidArchive,
			ctx: func() context.Context {
				// Passes the first ctx.Err() check (line 53), then fails the
				// first ctx.Err() check inside Extract's loop.
				return newCancelAfterN(1)
			},
			wantErr: "context canceled",
		},
		{
			name:        "cancelled context returns error after extract completes",
			archivePath: buildValidArchive,
			ctx: func() context.Context {
				// The valid archive has 4 directory entries + 2 file entries = 6
				// tar entries. Extract calls ctx.Err() once per entry and once
				// more for the EOF check = 7 calls. Plus call 1 at verify.Run
				// line 53 = 8 total before reaching line 67.
				return newCancelAfterN(8)
			},
			wantErr: "context canceled",
		},
		{
			name:        "cancelled context returns error after reading checksums",
			archivePath: buildValidArchive,
			ctx: func() context.Context {
				// Call 1 (line 53) + 7 (Extract) + 1 (line 67) = 9 calls before
				// reaching verify.Run line 81 (after findChecksums + ReadFile).
				return newCancelAfterN(9)
			},
			wantErr: "context canceled",
		},
		{
			name:        "cancelled context propagates into checksum.Verify",
			archivePath: buildValidArchive,
			ctx: func() context.Context {
				// 9 calls before line 81 + 1 more (line 81) = 10 total; call 11
				// fires inside checksum.Verify (once per checksum entry).
				return newCancelAfterN(10)
			},
			wantErr: "verify",
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

			archivePath := tt.archivePath(t)
			ctx := tt.ctx()

			result, err := verify.Run(ctx, archivePath)

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
				tt.checkResult(t, result)
			}
		})
	}
}
