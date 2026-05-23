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
	"os"
	"path/filepath"
	"strings"
	"testing"

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

				if err := runBuild(dir, nil); err != nil {
					t.Fatalf("build: %v", err)
				}
				return filepath.Join(dir, "verify-test-0.1.0.claudia")
			},
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
				outPath := filepath.Join(t.TempDir(), "no-checksums.claudia")
				if err := archive.Create(outPath, []archive.FileEntry{
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
