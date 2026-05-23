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
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/retr0h/claudia/internal/archive"
)

func TestCreate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setupFiles map[string][]byte // relative to tmpdir
		entries    func(dir string) []archive.FileEntry
		wantErr    bool
		checkTar   func(t *testing.T, archivePath string)
	}{
		{
			name: "creates archive from disk files",
			setupFiles: map[string][]byte{
				"hello.txt": []byte("hello world"),
			},
			entries: func(dir string) []archive.FileEntry {
				return []archive.FileEntry{
					{
						Src:         filepath.Join(dir, "hello.txt"),
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
			entries: func(_ string) []archive.FileEntry {
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
			setupFiles: map[string][]byte{
				"skill.md": []byte("# My Skill"),
			},
			entries: func(dir string) []archive.FileEntry {
				return []archive.FileEntry{
					{Src: filepath.Join(dir, "skill.md"), ArchivePath: "marketplaces/p/skills/skill.md"},
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
			entries: func(_ string) []archive.FileEntry {
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
			entries: func(dir string) []archive.FileEntry {
				return []archive.FileEntry{
					{Src: filepath.Join(dir, "nonexistent.txt"), ArchivePath: "test.txt"},
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srcDir := t.TempDir()
			for rel, content := range tt.setupFiles {
				path := filepath.Join(srcDir, rel)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.WriteFile(path, content, 0o644); err != nil {
					t.Fatalf("write %s: %v", rel, err)
				}
			}

			outPath := filepath.Join(t.TempDir(), "test.claudia")
			err := archive.Create(outPath, tt.entries(srcDir))

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.checkTar != nil {
				tt.checkTar(t, outPath)
			}
		})
	}
}

func TestExtract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		buildTar   func(t *testing.T) string // returns archive path
		wantErr    string
		checkFiles func(t *testing.T, destDir string)
	}{
		{
			name: "round-trip create and extract",
			buildTar: func(t *testing.T) string {
				t.Helper()
				srcDir := t.TempDir()
				if err := os.WriteFile(filepath.Join(srcDir, "data.txt"), []byte("content"), 0o644); err != nil {
					t.Fatal(err)
				}
				outPath := filepath.Join(t.TempDir(), "rt.claudia")
				if err := archive.Create(outPath, []archive.FileEntry{
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			archivePath := tt.buildTar(t)
			destDir := t.TempDir()

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

func TestCreatePreservesExecutableBit(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	binPath := filepath.Join(srcDir, "my-server")
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\necho hi"), 0o755); err != nil {
		t.Fatal(err)
	}

	outPath := filepath.Join(t.TempDir(), "exec.claudia")
	if err := archive.Create(outPath, []archive.FileEntry{
		{Src: binPath, ArchivePath: "marketplaces/p/mcp/my-server"},
	}); err != nil {
		t.Fatal(err)
	}

	destDir := t.TempDir()
	if err := archive.Extract(outPath, destDir); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(filepath.Join(destDir, "marketplaces", "p", "mcp", "my-server"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Errorf("expected executable bit, got %v", info.Mode())
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
			if _, err := tr.Read(buf); err != nil && err.Error() != "EOF" {
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
