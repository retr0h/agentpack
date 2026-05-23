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
	"path/filepath"
	"strings"
	"testing"

	"github.com/avfs/avfs/vfs/osfs"

	"github.com/retr0h/claudia/internal/archive"
)

func TestRunVerify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		buildArchive func(t *testing.T) string // returns archive path
		wantErr      string
	}{
		{
			name: "valid archive passes verification",
			buildArchive: func(t *testing.T) string {
				t.Helper()
				ctx := context.Background()
				vfs := osfs.NewWithNoIdm()
				dir := t.TempDir()
				initTestRepo(t, dir)
				writeFile(t, filepath.Join(dir, "claudia.yaml"), `
name: verify-test
version: 0.1.0
description: "Verify test plugin"
skills:
  - skills/hello.md
`)
				writeFile(t, filepath.Join(dir, "skills", "hello.md"), "# Hello")
				commitAll(t, dir)

				if err := runBuild(ctx, vfs, dir, nil); err != nil {
					t.Fatalf("build: %v", err)
				}
				return filepath.Join(dir, "verify-test-0.1.0.claudia")
			},
		},
		{
			name: "tampered archive fails verification",
			buildArchive: func(t *testing.T) string {
				t.Helper()
				ctx := context.Background()
				vfs := osfs.NewWithNoIdm()
				dir := t.TempDir()
				initTestRepo(t, dir)
				writeFile(t, filepath.Join(dir, "claudia.yaml"), `
name: tamper-test
version: 0.1.0
description: "Tamper test plugin"
skills:
  - skills/hello.md
`)
				writeFile(t, filepath.Join(dir, "skills", "hello.md"), "# Hello")
				commitAll(t, dir)

				if err := runBuild(ctx, vfs, dir, nil); err != nil {
					t.Fatalf("build: %v", err)
				}
				archivePath := filepath.Join(dir, "tamper-test-0.1.0.claudia")

				// Extract archive, tamper a file, then build a new checksums.txt
				// that references the tampered content with a wrong hash.
				// Simplest approach: extract the archive, then rebuild it with a
				// file whose content changed but the checksum in checksums.txt
				// still references the old hash.
				tmpDir := t.TempDir()
				if err := archive.Extract(archivePath, tmpDir); err != nil {
					t.Fatalf("extract: %v", err)
				}

				// Tamper: overwrite a skill file with different content.
				skillPath := filepath.Join(tmpDir, "marketplaces", "tamper-test", "skills", "hello.md")
				if err := os.WriteFile(skillPath, []byte("# TAMPERED"), 0o644); err != nil {
					t.Fatalf("tamper: %v", err)
				}

				// Re-pack the tampered directory as a new archive.
				tamperedPath := filepath.Join(t.TempDir(), "tampered.claudia")
				var entries []archive.FileEntry

				// Walk the extracted dir and pack everything back.
				if err := filepath.WalkDir(tmpDir, func(path string, d os.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if d.IsDir() {
						return nil
					}
					rel, _ := filepath.Rel(tmpDir, path)
					entries = append(entries, archive.FileEntry{
						Src:         path,
						ArchivePath: rel,
					})
					return nil
				}); err != nil {
					t.Fatalf("walk: %v", err)
				}

				if err := archive.Create(ctx, vfs, tamperedPath, entries); err != nil {
					t.Fatalf("create tampered archive: %v", err)
				}

				return tamperedPath
			},
			wantErr: "failed verification",
		},
		{
			name: "missing archive returns error",
			buildArchive: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "nonexistent.claudia")
			},
			wantErr: "open archive",
		},
		{
			name: "archive without checksums.txt returns error",
			buildArchive: func(t *testing.T) string {
				t.Helper()
				ctx := context.Background()
				vfs := osfs.NewWithNoIdm()
				outPath := filepath.Join(t.TempDir(), "no-checksums.claudia")
				if err := archive.Create(ctx, vfs, outPath, []archive.FileEntry{
					{ArchivePath: "marketplaces/p/skills/hello.md", Content: []byte("# Hi")},
				}); err != nil {
					t.Fatalf("create archive: %v", err)
				}
				return outPath
			},
			wantErr: "checksums.txt not found in archive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			archivePath := tt.buildArchive(t)
			err := runVerify(archivePath)

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
		})
	}
}

func TestFindChecksums(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setupDir   func(t *testing.T) string // returns base dir
		wantErr    string
		wantSuffix string // expected path suffix
	}{
		{
			name: "finds checksums.txt inside .claudia directory",
			setupDir: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				p := filepath.Join(dir, "marketplaces", "pkg", ".claudia", "checksums.txt")
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.WriteFile(p, []byte("hash  file\n"), 0o644); err != nil {
					t.Fatalf("write: %v", err)
				}
				return dir
			},
			wantSuffix: filepath.Join(".claudia", "checksums.txt"),
		},
		{
			name: "returns error when checksums.txt is absent",
			setupDir: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			wantErr: "checksums.txt not found in archive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := tt.setupDir(t)
			got, err := findChecksums(dir)

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

			if !strings.HasSuffix(got, tt.wantSuffix) {
				t.Errorf("path %q does not have suffix %q", got, tt.wantSuffix)
			}
		})
	}
}
