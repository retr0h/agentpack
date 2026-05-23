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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/avfs/avfs/vfs/osfs"

	"github.com/retr0h/claudia/pkg/archive"
	"github.com/retr0h/claudia/pkg/verify"
)

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

// sha256Hex returns the hex-encoded SHA256 of data.
func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// buildValidArchive creates a .claudia archive with one file and a valid
// checksums.txt, returning the archive path.
func buildValidArchive(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	vfs := osfs.NewWithNoIdm()

	content := []byte("hello claudia")
	filePath := "marketplaces/my-plugin/skills/intro.md"
	checksumPath := "marketplaces/my-plugin/.claudia/checksums.txt"

	skillHashStr := sha256Hex(content)
	checksumLine := fmt.Sprintf("%s  %s\n", skillHashStr, filePath)

	// The checksums.txt also checksums itself, but that creates a circular
	// dependency. In practice claudia.yaml archives store checksums for the
	// content files only — the checksums.txt file is excluded from its own
	// list. We keep it simple here: one file, one checksum entry.
	checksumContent := checksumLine

	outPath := filepath.Join(dir, "test.claudia")
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
	checksumPath := "marketplaces/my-plugin/.claudia/checksums.txt"

	// Record a wrong hash to simulate tampering.
	badHash := strings.Repeat("0", 64)
	checksumContent := fmt.Sprintf("%s  %s\n", badHash, filePath)

	outPath := filepath.Join(dir, "tampered.claudia")
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

	outPath := filepath.Join(dir, "nochecksum.claudia")
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

	outPath := filepath.Join(dir, "badformat.claudia")
	if err := archive.Create(context.Background(), vfs, outPath, []archive.FileEntry{
		{
			ArchivePath: "marketplaces/my-plugin/.claudia/checksums.txt",
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

	outPath := filepath.Join(t.TempDir(), "symlink.claudia")
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
			archivePath: func(t *testing.T) string { return filepath.Join(t.TempDir(), "nonexistent.claudia") },
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
			name:        "cancelled context returns error before extract",
			archivePath: buildValidArchive,
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			wantErr: "context canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

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
