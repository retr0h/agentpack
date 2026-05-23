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

package archive_test

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/avfs/avfs"
	"github.com/avfs/avfs/vfs/memfs"
	"github.com/avfs/avfs/vfs/osfs"

	"github.com/retr0h/claudia/internal/archive"
)

// --------------------------------------------------------------------------
// Error-injecting VFS helpers
// --------------------------------------------------------------------------

// createErrorVFS returns an error from Create.
type createErrorVFS struct {
	avfs.VFS
}

func (createErrorVFS) Create(string) (avfs.File, error) {
	return nil, errors.New("simulated create error")
}

// statErrorVFS returns an error from Stat (for Src file stat path).
type statErrorVFS struct {
	avfs.VFS
}

func (v statErrorVFS) Stat(name string) (fs.FileInfo, error) {
	return nil, errors.New("simulated stat error")
}

// openErrorVFS has Stat succeed but Open fail — exercises the open-error
// branch inside addFileFromDisk.
type openErrorVFS struct {
	avfs.VFS
}

func (v openErrorVFS) Stat(name string) (fs.FileInfo, error) {
	return v.VFS.Stat(name)
}

func (openErrorVFS) Open(string) (avfs.File, error) {
	return nil, errors.New("simulated open error")
}

// --------------------------------------------------------------------------
// Tests
// --------------------------------------------------------------------------

func TestCreate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tests := []struct {
		name       string
		setupVFS   func(t *testing.T) (avfs.VFS, string)
		entries    func(vfs avfs.VFS, outDir string) []archive.FileEntry
		wantErr    bool
		wantErrStr string
		checkTar   func(t *testing.T, archivePath string)
	}{
		{
			name: "creates archive from disk files",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				vfs := memfs.New()
				if err := vfs.MkdirAll("/src", 0o755); err != nil {
					t.Fatal(err)
				}
				if err := vfs.WriteFile("/src/hello.txt", []byte("hello world"), fs.FileMode(0o644)); err != nil {
					t.Fatal(err)
				}
				return vfs, "/out"
			},
			entries: func(_ avfs.VFS, outDir string) []archive.FileEntry {
				return []archive.FileEntry{
					{
						Src:         "/src/hello.txt",
						ArchivePath: "marketplaces/test/hello.txt",
					},
				}
			},
			checkTar: func(t *testing.T, archivePath string) {
				t.Helper()
				entries := listTarEntries(t, archivePath)
				assertContains(t, entries, "marketplaces/test/hello.txt")
			},
		},
		{
			name: "creates archive from virtual files",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return memfs.New(), "/out"
			},
			entries: func(_ avfs.VFS, outDir string) []archive.FileEntry {
				return []archive.FileEntry{
					{
						ArchivePath: "marketplaces/test/generated.json",
						Content:     []byte(`{"key":"value"}`),
					},
				}
			},
			checkTar: func(t *testing.T, archivePath string) {
				t.Helper()
				content := readTarFile(t, archivePath, "marketplaces/test/generated.json")
				if string(content) != `{"key":"value"}` {
					t.Errorf("content = %q, want %q", content, `{"key":"value"}`)
				}
			},
		},
		{
			name: "mixed disk and virtual files",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				vfs := memfs.New()
				if err := vfs.WriteFile("/skill.md", []byte("# My Skill"), fs.FileMode(0o644)); err != nil {
					t.Fatal(err)
				}
				return vfs, "/out"
			},
			entries: func(_ avfs.VFS, outDir string) []archive.FileEntry {
				return []archive.FileEntry{
					{Src: "/skill.md", ArchivePath: "marketplaces/p/skills/skill.md"},
					{ArchivePath: "marketplaces/p/.claudia/metadata.json", Content: []byte("{}")},
				}
			},
			checkTar: func(t *testing.T, archivePath string) {
				t.Helper()
				entries := listTarEntries(t, archivePath)
				assertContains(t, entries, "marketplaces/p/skills/skill.md")
				assertContains(t, entries, "marketplaces/p/.claudia/metadata.json")
			},
		},
		{
			name: "creates intermediate directories",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return memfs.New(), "/out"
			},
			entries: func(_ avfs.VFS, outDir string) []archive.FileEntry {
				return []archive.FileEntry{
					{ArchivePath: "a/b/c/file.txt", Content: []byte("deep")},
				}
			},
			checkTar: func(t *testing.T, archivePath string) {
				t.Helper()
				entries := listTarEntries(t, archivePath)
				assertContains(t, entries, "a/")
				assertContains(t, entries, "a/b/")
				assertContains(t, entries, "a/b/c/")
			},
		},
		{
			name: "fails on missing source file",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return memfs.New(), "/out"
			},
			entries: func(_ avfs.VFS, outDir string) []archive.FileEntry {
				return []archive.FileEntry{
					{Src: "/nonexistent.txt", ArchivePath: "test.txt"},
				}
			},
			wantErr: true,
		},
		{
			name: "fails when vfs Create returns error",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return createErrorVFS{VFS: memfs.New()}, "/out/test.claudia"
			},
			entries: func(_ avfs.VFS, outDir string) []archive.FileEntry {
				return []archive.FileEntry{
					{ArchivePath: "file.txt", Content: []byte("data")},
				}
			},
			wantErr:    true,
			wantErrStr: "create archive",
		},
		{
			name: "fails when stat returns error for src file",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return statErrorVFS{VFS: memfs.New()}, "/out"
			},
			entries: func(_ avfs.VFS, outDir string) []archive.FileEntry {
				return []archive.FileEntry{
					{Src: "/somefile.txt", ArchivePath: "test.txt"},
				}
			},
			wantErr:    true,
			wantErrStr: "stat",
		},
		{
			name: "fails when open returns error after stat succeeds",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				base := memfs.New()
				if err := base.MkdirAll("/src", 0o755); err != nil {
					t.Fatal(err)
				}
				if err := base.WriteFile("/src/file.txt", []byte("data"), fs.FileMode(0o644)); err != nil {
					t.Fatal(err)
				}
				return openErrorVFS{VFS: base}, "/out"
			},
			entries: func(_ avfs.VFS, outDir string) []archive.FileEntry {
				return []archive.FileEntry{
					{Src: "/src/file.txt", ArchivePath: "test.txt"},
				}
			},
			wantErr:    true,
			wantErrStr: "open",
		},
		{
			name: "preserves executable bit on disk files",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				vfs := osfs.NewWithNoIdm()
				return vfs, t.TempDir()
			},
			entries: func(_ avfs.VFS, outDir string) []archive.FileEntry {
				binPath := filepath.Join(outDir, "my-server")
				if err := os.WriteFile(binPath, []byte("#!/bin/sh\necho hi"), 0o755); err != nil {
					panic(err)
				}
				return []archive.FileEntry{
					{Src: binPath, ArchivePath: "marketplaces/p/mcp/my-server"},
				}
			},
			checkTar: func(t *testing.T, archivePath string) {
				t.Helper()
				destDir := t.TempDir()
				if err := archive.Extract(archivePath, destDir); err != nil {
					t.Fatalf("extract: %v", err)
				}
				info, err := os.Stat(filepath.Join(destDir, "marketplaces", "p", "mcp", "my-server"))
				if err != nil {
					t.Fatal(err)
				}
				if info.Mode()&0o111 == 0 {
					t.Errorf("expected executable bit, got %v", info.Mode())
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			vfs, outDir := tt.setupVFS(t)

			var outPath string
			if strings.HasSuffix(outDir, ".claudia") {
				outPath = outDir
			} else {
				if err := vfs.MkdirAll(outDir, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
					// createErrorVFS doesn't support MkdirAll; skip
				}
				outPath = outDir + "/test.claudia"
			}

			err := archive.Create(ctx, vfs, outPath, tt.entries(vfs, outDir))

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantErrStr != "" && !strings.Contains(err.Error(), tt.wantErrStr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.wantErrStr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// For checkTar, we need to read the archive from the VFS and
			// write it to a real temp dir so the tar reader can open it.
			if tt.checkTar != nil {
				data, err := vfs.ReadFile(outPath)
				if err != nil {
					t.Fatalf("read archive from vfs: %v", err)
				}
				realPath := filepath.Join(t.TempDir(), "test.claudia")
				if err := os.WriteFile(realPath, data, 0o644); err != nil {
					t.Fatalf("write archive to real fs: %v", err)
				}
				tt.checkTar(t, realPath)
			}
		})
	}
}

func TestExtract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		buildTar   func(t *testing.T) string // returns archive path
		setupDest  func(t *testing.T, destDir string)
		wantErr    string
		checkFiles func(t *testing.T, destDir string)
	}{
		{
			name: "round-trip create and extract",
			buildTar: func(t *testing.T) string {
				t.Helper()
				ctx := context.Background()
				vfs := osfs.NewWithNoIdm()
				srcDir := t.TempDir()
				if err := os.WriteFile(filepath.Join(srcDir, "data.txt"), []byte("content"), 0o644); err != nil {
					t.Fatal(err)
				}
				outPath := filepath.Join(t.TempDir(), "rt.claudia")
				if err := archive.Create(ctx, vfs, outPath, []archive.FileEntry{
					{Src: filepath.Join(srcDir, "data.txt"), ArchivePath: "marketplaces/pkg/data.txt"},
					{ArchivePath: "marketplaces/pkg/.claudia/meta.json", Content: []byte(`{"v":1}`)},
				}); err != nil {
					t.Fatal(err)
				}
				return outPath
			},
			checkFiles: func(t *testing.T, destDir string) {
				t.Helper()
				got, err := os.ReadFile(filepath.Join(destDir, "marketplaces", "pkg", "data.txt"))
				if err != nil {
					t.Fatalf("read data.txt: %v", err)
				}
				if string(got) != "content" {
					t.Errorf("data.txt = %q, want %q", got, "content")
				}
				got2, err := os.ReadFile(filepath.Join(destDir, "marketplaces", "pkg", ".claudia", "meta.json"))
				if err != nil {
					t.Fatalf("read meta.json: %v", err)
				}
				if string(got2) != `{"v":1}` {
					t.Errorf("meta.json = %q", got2)
				}
			},
		},
		{
			name: "extracts TypeDir entries",
			buildTar: func(t *testing.T) string {
				t.Helper()
				return buildTarWithDir(t)
			},
			checkFiles: func(t *testing.T, destDir string) {
				t.Helper()
				info, err := os.Stat(filepath.Join(destDir, "mydir"))
				if err != nil {
					t.Fatalf("stat mydir: %v", err)
				}
				if !info.IsDir() {
					t.Errorf("expected mydir to be a directory")
				}
			},
		},
		{
			name: "rejects symlinks",
			buildTar: func(t *testing.T) string {
				t.Helper()
				return buildTarWithSymlink(t)
			},
			wantErr: "symlinks not allowed",
		},
		{
			name: "rejects path traversal",
			buildTar: func(t *testing.T) string {
				t.Helper()
				return buildTarWithTraversal(t)
			},
			wantErr: "path traversal detected",
		},
		{
			name: "fails on missing archive",
			buildTar: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "nonexistent.claudia")
			},
			wantErr: "open archive",
		},
		{
			name: "fails when dest dir is read-only",
			buildTar: func(t *testing.T) string {
				t.Helper()
				ctx := context.Background()
				vfs := osfs.NewWithNoIdm()
				outPath := filepath.Join(t.TempDir(), "test.claudia")
				if err := archive.Create(ctx, vfs, outPath, []archive.FileEntry{
					{ArchivePath: "file.txt", Content: []byte("data")},
				}); err != nil {
					t.Fatal(err)
				}
				return outPath
			},
			setupDest: func(t *testing.T, destDir string) {
				t.Helper()
				if err := os.Chmod(destDir, 0o555); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(destDir, 0o755) })
			},
			wantErr: "create",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			archivePath := tt.buildTar(t)
			destDir := t.TempDir()

			if tt.setupDest != nil {
				tt.setupDest(t, destDir)
			}

			err := archive.Extract(archivePath, destDir)

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

			if tt.checkFiles != nil {
				tt.checkFiles(t, destDir)
			}
		})
	}
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

func listTarEntries(t *testing.T, archivePath string) []string {
	t.Helper()

	f, err := os.Open(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	var names []string

	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		names = append(names, hdr.Name)
	}

	return names
}

func readTarFile(t *testing.T, archivePath, name string) []byte {
	t.Helper()

	f, err := os.Open(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	for {
		hdr, err := tr.Next()
		if err != nil {
			t.Fatalf("file %q not found in archive", name)
		}
		if hdr.Name == name {
			buf := make([]byte, hdr.Size)
			if _, err := tr.Read(buf); err != nil && !errors.Is(err, io.EOF) {
				t.Fatalf("read %s: %v", name, err)
			}
			return buf
		}
	}
}

func assertContains(t *testing.T, entries []string, want string) {
	t.Helper()

	if !slices.Contains(entries, want) {
		t.Errorf("entries %v does not contain %q", entries, want)
	}
}

func buildTarWithSymlink(t *testing.T) string {
	t.Helper()

	outPath := filepath.Join(t.TempDir(), "symlink.claudia")
	f, err := os.Create(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	if err := tw.WriteHeader(&tar.Header{
		Name:     "evil-link",
		Typeflag: tar.TypeSymlink,
		Linkname: "/etc/passwd",
	}); err != nil {
		t.Fatal(err)
	}

	return outPath
}

func buildTarWithTraversal(t *testing.T) string {
	t.Helper()

	outPath := filepath.Join(t.TempDir(), "traversal.claudia")
	f, err := os.Create(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	content := []byte("pwned")
	if err := tw.WriteHeader(&tar.Header{
		Name:     "../../../etc/evil",
		Size:     int64(len(content)),
		Mode:     0o644,
		Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}

	return outPath
}

// buildTarWithDir creates a tarball containing a TypeDir entry.
func buildTarWithDir(t *testing.T) string {
	t.Helper()

	outPath := filepath.Join(t.TempDir(), "withdir.claudia")
	f, err := os.Create(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	if err := tw.WriteHeader(&tar.Header{
		Name:     "mydir/",
		Typeflag: tar.TypeDir,
		Mode:     0o755,
	}); err != nil {
		t.Fatal(err)
	}

	return outPath
}
