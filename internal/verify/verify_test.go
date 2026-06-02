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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/internal/archive"
	"github.com/retr0h/agentpack/internal/verify"
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
		name      string
		setup     func(t *testing.T) string // returns dir
		wantErr   string
		checkErr  func(t *testing.T, err error) // optional extra assertions on error
		checkPath func(t *testing.T, path string)
	}{
		{
			name: "finds checksums.txt inside .agentpack dir",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				agentpackDir := filepath.Join(dir, "marketplaces", "my-plugin", ".agentpack")
				require.NoError(t, os.MkdirAll(agentpackDir, 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(agentpackDir, "checksums.txt"),
					[]byte("hash  file.txt\n"),
					0o644,
				))
				return dir
			},
			checkPath: func(t *testing.T, path string) {
				t.Helper()
				assert.True(t, strings.HasSuffix(path, "checksums.txt"))
			},
		},
		{
			name: "returns errChecksumsNotFound sentinel when checksums.txt absent",
			setup: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			wantErr: "checksums.txt not found",
			// The sentinel must be detectable via errors.Is so Run can
			// distinguish a missing checksums.txt (new-format archive,
			// ADR-009) from a real I/O failure and skip gracefully.
			checkErr: func(t *testing.T, err error) {
				t.Helper()
				assert.True(t, errors.Is(err, verify.ErrChecksumsNotFound))
			},
		},
		{
			name: "returns error when dir entry callback gets an error",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				// Create a subdirectory we cannot read (permission denied).
				subdir := filepath.Join(dir, "locked")
				require.NoError(t, os.MkdirAll(subdir, 0o000))
				t.Cleanup(func() { _ = os.Chmod(subdir, 0o755) })
				return dir
			},
			wantErr: "searching for checksums.txt",
			// The walk I/O error must NOT match the not-found sentinel — Run
			// must surface it as a real error rather than silently skipping.
			checkErr: func(t *testing.T, err error) {
				t.Helper()
				assert.False(t, errors.Is(err, verify.ErrChecksumsNotFound))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := tt.setup(t)
			path, err := verify.FindChecksums(dir)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				if tt.checkErr != nil {
					tt.checkErr(t, err)
				}
				return
			}

			require.NoError(t, err)

			if tt.checkPath != nil {
				tt.checkPath(t, path)
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
	require.NoError(t, archive.Create(context.Background(), vfs, outPath, []archive.FileEntry{
		{ArchivePath: filePath, Content: content},
		{ArchivePath: checksumPath, Content: []byte(checksumContent)},
	}))

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
	require.NoError(t, archive.Create(context.Background(), vfs, outPath, []archive.FileEntry{
		{ArchivePath: filePath, Content: content},
		{ArchivePath: checksumPath, Content: []byte(checksumContent)},
	}))

	return outPath
}

// buildArchiveWithoutChecksums creates an archive with no checksums.txt.
func buildArchiveWithoutChecksums(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	vfs := osfs.NewWithNoIdm()

	outPath := filepath.Join(dir, "nochecksum.agentpack")
	require.NoError(t, archive.Create(context.Background(), vfs, outPath, []archive.FileEntry{
		{ArchivePath: "marketplaces/my-plugin/skills/intro.md", Content: []byte("content")},
	}))

	return outPath
}

// buildArchiveWithBadChecksumsFormat creates an archive whose checksums.txt
// has invalid format lines (missing double-space separator).
func buildArchiveWithBadChecksumsFormat(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	vfs := osfs.NewWithNoIdm()

	outPath := filepath.Join(dir, "badformat.agentpack")
	require.NoError(t, archive.Create(context.Background(), vfs, outPath, []archive.FileEntry{
		{
			ArchivePath: "marketplaces/my-plugin/.agentpack/checksums.txt",
			Content:     []byte("badhash file.txt\n"), // missing double-space separator
		},
	}))

	return outPath
}

// buildTarWithSymlink creates a raw tarball containing a symlink entry
// (which archive.Extract rejects).
func buildTarWithSymlink(t *testing.T) string {
	t.Helper()

	outPath := filepath.Join(t.TempDir(), "symlink.agentpack")
	f, err := os.Create(outPath)
	if err != nil {
		require.NoError(t, err)
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
		require.NoError(t, err)
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
				require.NotNil(t, r)
				assert.NotEmpty(t, r.ArchiveName)
				for _, f := range r.Files {
					assert.True(t, f.OK)
				}
			},
		},
		{
			name:        "tampered file fails verification",
			archivePath: buildArchiveWithTamperedFile,
			ctx:         func() context.Context { return context.Background() },
			checkResult: func(t *testing.T, r *verify.Result) {
				t.Helper()
				require.NotNil(t, r)
				failed := 0
				for _, f := range r.Files {
					if !f.OK {
						failed++
					}
				}
				assert.NotZero(t, failed)
			},
		},
		{
			// New-format archives (ADR-009) omit checksums.txt. Run must return
			// a success result with an empty Files slice rather than an error.
			// Archive-level integrity is handled by the .sha256 sidecar at the
			// command layer, before Run is called.
			name:        "archive without checksums.txt skips internal verification",
			archivePath: buildArchiveWithoutChecksums,
			ctx:         func() context.Context { return context.Background() },
			checkResult: func(t *testing.T, r *verify.Result) {
				t.Helper()
				require.NotNil(t, r)
				assert.NotEmpty(t, r.ArchiveName)
				assert.Empty(t, r.Files)
			},
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

			result, err := verify.New().Run(ctx, verify.Options{ArchivePath: archivePath})

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.checkResult != nil {
				tt.checkResult(t, result)
			}
		})
	}
}
