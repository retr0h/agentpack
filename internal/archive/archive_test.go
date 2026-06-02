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
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/avfs/avfs"
	"github.com/avfs/avfs/vfs/memfs"
	"github.com/avfs/avfs/vfs/osfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/internal/archive"
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

func (statErrorVFS) Stat(string) (fs.FileInfo, error) {
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

// closeErrorFile wraps avfs.File so that Close returns an error, exercising
// the f.Close() error branch in Create.
type closeErrorFile struct {
	avfs.File
}

func (closeErrorFile) Close() error {
	return errors.New("simulated close error")
}

// closeErrorVFS returns a closeErrorFile from Create, backed by a real memfs
// file so writes succeed but Close fails.
type closeErrorVFS struct {
	avfs.VFS
}

func (v closeErrorVFS) Create(name string) (avfs.File, error) {
	f, err := v.VFS.Create(name)
	if err != nil {
		return nil, err
	}
	return closeErrorFile{File: f}, nil
}

// readErrorFile wraps avfs.File so that Read always returns an error, causing
// io.Copy from this file to fail.
type readErrorFile struct {
	avfs.File
}

func (readErrorFile) Read([]byte) (int, error) {
	return 0, errors.New("simulated read error")
}

func (readErrorFile) Close() error { return nil }

// readErrorVFS has Stat and Open succeed but returns a readErrorFile that fails
// on Read, exercising the io.Copy error branch in addFileFromDisk.
type readErrorVFS struct {
	avfs.VFS
}

func (v readErrorVFS) Stat(name string) (fs.FileInfo, error) {
	return v.VFS.Stat(name)
}

func (v readErrorVFS) Open(name string) (avfs.File, error) {
	f, err := v.VFS.Open(name)
	if err != nil {
		return nil, err
	}
	_ = f.Close()
	return readErrorFile{}, nil
}

// writeErrorFile wraps avfs.File so that Write always fails, causing tar/gzip
// close operations (which write final bytes) to return errors.
type writeErrorFile struct {
	avfs.File
}

func (writeErrorFile) Write([]byte) (int, error) {
	return 0, errors.New("simulated write error")
}

func (writeErrorFile) Close() error { return nil }

// writeErrorVFS returns a writeErrorFile from Create.
type writeErrorVFS struct {
	avfs.VFS
}

func (v writeErrorVFS) Create(name string) (avfs.File, error) {
	f, err := v.VFS.Create(name)
	if err != nil {
		return nil, err
	}
	_ = f.Close()
	return writeErrorFile{}, nil
}

// writeAfterNFile wraps avfs.File and fails writes after N bytes have been
// written. This lets tar.Close succeed (it writes 1024 bytes of zeros) but
// gzip.Close fail (it writes the gzip trailer after the tar data).
type writeAfterNFile struct {
	avfs.File
	limit   int
	written int
}

func (f *writeAfterNFile) Write(p []byte) (int, error) {
	if f.written >= f.limit {
		return 0, errors.New("simulated write error after limit")
	}
	n := len(p)
	if f.written+n > f.limit {
		n = f.limit - f.written
	}
	f.written += n
	return n, nil
}

func (f *writeAfterNFile) Close() error { return nil }

// writeAfterNVFS returns a writeAfterNFile from Create with the given byte limit.
type writeAfterNVFS struct {
	avfs.VFS
	limit int
}

func (v writeAfterNVFS) Create(name string) (avfs.File, error) {
	f, err := v.VFS.Create(name)
	if err != nil {
		return nil, err
	}
	_ = f.Close()
	return &writeAfterNFile{limit: v.limit}, nil
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
					require.NoError(t, err)
				}
				if err := vfs.WriteFile("/src/hello.txt", []byte("hello world"), fs.FileMode(0o644)); err != nil {
					require.NoError(t, err)
				}
				return vfs, "/out"
			},
			entries: func(_ avfs.VFS, _ string) []archive.FileEntry {
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
				assert.Contains(t, entries, "marketplaces/test/hello.txt")
			},
		},
		{
			name: "creates archive from virtual files",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return memfs.New(), "/out"
			},
			entries: func(_ avfs.VFS, _ string) []archive.FileEntry {
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
				assert.Equal(t, `{"key":"value"}`, string(content))
			},
		},
		{
			name: "mixed disk and virtual files",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				vfs := memfs.New()
				if err := vfs.WriteFile("/skill.md", []byte("# My Skill"), fs.FileMode(0o644)); err != nil {
					require.NoError(t, err)
				}
				return vfs, "/out"
			},
			entries: func(_ avfs.VFS, _ string) []archive.FileEntry {
				return []archive.FileEntry{
					{Src: "/skill.md", ArchivePath: "marketplaces/p/skills/skill.md"},
					{ArchivePath: "marketplaces/p/.agentpack/metadata.json", Content: []byte("{}")},
				}
			},
			checkTar: func(t *testing.T, archivePath string) {
				t.Helper()
				entries := listTarEntries(t, archivePath)
				assert.Contains(t, entries, "marketplaces/p/skills/skill.md")
				assert.Contains(t, entries, "marketplaces/p/.agentpack/metadata.json")
			},
		},
		{
			name: "creates intermediate directories",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return memfs.New(), "/out"
			},
			entries: func(_ avfs.VFS, _ string) []archive.FileEntry {
				return []archive.FileEntry{
					{ArchivePath: "a/b/c/file.txt", Content: []byte("deep")},
				}
			},
			checkTar: func(t *testing.T, archivePath string) {
				t.Helper()
				entries := listTarEntries(t, archivePath)
				assert.Contains(t, entries, "a/")
				assert.Contains(t, entries, "a/b/")
				assert.Contains(t, entries, "a/b/c/")
			},
		},
		{
			name: "fails on missing source file",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return memfs.New(), "/out"
			},
			entries: func(_ avfs.VFS, _ string) []archive.FileEntry {
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
				return createErrorVFS{VFS: memfs.New()}, "/out/test.agentpack"
			},
			entries: func(_ avfs.VFS, _ string) []archive.FileEntry {
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
			entries: func(_ avfs.VFS, _ string) []archive.FileEntry {
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
					require.NoError(t, err)
				}
				if err := base.WriteFile("/src/file.txt", []byte("data"), fs.FileMode(0o644)); err != nil {
					require.NoError(t, err)
				}
				return openErrorVFS{VFS: base}, "/out"
			},
			entries: func(_ avfs.VFS, _ string) []archive.FileEntry {
				return []archive.FileEntry{
					{Src: "/src/file.txt", ArchivePath: "test.txt"},
				}
			},
			wantErr:    true,
			wantErrStr: "open",
		},
		{
			name: "fails when reading disk file content fails during copy",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				base := memfs.New()
				if err := base.MkdirAll("/src", 0o755); err != nil {
					require.NoError(t, err)
				}
				if err := base.WriteFile("/src/file.txt", []byte("data"), fs.FileMode(0o644)); err != nil {
					require.NoError(t, err)
				}
				return readErrorVFS{VFS: base}, "/out"
			},
			entries: func(_ avfs.VFS, _ string) []archive.FileEntry {
				return []archive.FileEntry{
					{Src: "/src/file.txt", ArchivePath: "test.txt"},
				}
			},
			wantErr:    true,
			wantErrStr: "copy",
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
				err := archive.Extract(context.Background(), archivePath, destDir)
				require.NoError(t, err)
				info, err := os.Stat(
					filepath.Join(destDir, "marketplaces", "p", "mcp", "my-server"),
				)
				require.NoError(t, err)
				assert.NotEqual(t, fs.FileMode(0), info.Mode()&0o111)
			},
		},
		{
			name: "returns error when context is already cancelled",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return memfs.New(), "/out"
			},
			entries: func(_ avfs.VFS, _ string) []archive.FileEntry {
				return []archive.FileEntry{
					{ArchivePath: "file.txt", Content: []byte("data")},
				}
			},
			wantErr:    true,
			wantErrStr: "context canceled",
		},
		{
			name: "returns error when archive file close fails",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return closeErrorVFS{VFS: memfs.New()}, "/out"
			},
			entries: func(_ avfs.VFS, _ string) []archive.FileEntry {
				return []archive.FileEntry{
					{ArchivePath: "file.txt", Content: []byte("data")},
				}
			},
			wantErr:    true,
			wantErrStr: "close",
		},
		{
			name: "returns error when virtual file header write fails",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return writeErrorVFS{VFS: memfs.New()}, "/out"
			},
			entries: func(_ avfs.VFS, _ string) []archive.FileEntry {
				return []archive.FileEntry{
					{ArchivePath: "file.txt", Content: []byte("data")},
				}
			},
			wantErr: true,
		},
		{
			name: "fails when disk file write header fails",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				// writeErrorVFS fails all writes to the output file. A flat
				// ArchivePath means ensureDirs writes nothing, so the first
				// gzip Write (the 10-byte gzip header) happens inside
				// addFileFromDisk's tw.WriteHeader call, which then fails.
				base := memfs.New()
				if err := base.MkdirAll("/src", 0o755); err != nil {
					require.NoError(t, err)
				}
				if err := base.WriteFile("/src/real.txt", []byte("data"), fs.FileMode(0o644)); err != nil {
					require.NoError(t, err)
				}
				return writeErrorVFS{VFS: base}, "/out"
			},
			entries: func(_ avfs.VFS, _ string) []archive.FileEntry {
				return []archive.FileEntry{
					{Src: "/src/real.txt", ArchivePath: "real.txt"},
				}
			},
			wantErr:    true,
			wantErrStr: "write header",
		},
		{
			name: "returns error when directory header write fails in ensureDirs",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				// writeErrorVFS fails all writes immediately. A nested archive
				// path causes ensureDirs to call tw.WriteHeader for the parent
				// directory, which fails on the first write.
				return writeErrorVFS{VFS: memfs.New()}, "/out"
			},
			entries: func(_ avfs.VFS, _ string) []archive.FileEntry {
				return []archive.FileEntry{
					{ArchivePath: "nested/dir/file.txt", Content: []byte("data")},
				}
			},
			wantErr: true,
		},
		{
			name: "returns error when tar writer close fails",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return writeErrorVFS{VFS: memfs.New()}, "/out"
			},
			entries: func(_ avfs.VFS, _ string) []archive.FileEntry {
				// An empty entry list means no files to write, but tw.Close()
				// still writes the end-of-archive marker, which triggers the
				// write error.
				return []archive.FileEntry{}
			},
			wantErr:    true,
			wantErrStr: "close tar writer",
		},
		{
			name: "returns error when gzip writer close fails",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				// With an empty file list, gzip compresses the tar end-of-archive
				// (1024 zero bytes) into exactly 10 bytes, then gzip.Close writes
				// its DEFLATE flush (10 bytes) + CRC32/size trailer (8 bytes).
				// Allowing exactly 20 bytes lets tw.Close succeed but makes
				// gzip.Close fail when it attempts its third write of 4 bytes.
				return writeAfterNVFS{VFS: memfs.New(), limit: 20}, "/out"
			},
			entries: func(_ avfs.VFS, _ string) []archive.FileEntry {
				return []archive.FileEntry{}
			},
			wantErr:    true,
			wantErrStr: "close gzip writer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			vfs, outDir := tt.setupVFS(t)

			var outPath string
			if strings.HasSuffix(outDir, ".agentpack") {
				outPath = outDir
			} else {
				_ = vfs.MkdirAll(outDir, 0o755)
				outPath = outDir + "/test.agentpack"
			}

			// Use a cancelled context for the context-cancellation test case.
			testCtx := ctx
			if tt.wantErrStr == "context canceled" {
				cancelCtx, cancel := context.WithCancel(context.Background())
				cancel()
				testCtx = cancelCtx
			}

			err := archive.Create(testCtx, vfs, outPath, tt.entries(vfs, outDir))

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrStr != "" {
					require.ErrorContains(t, err, tt.wantErrStr)
				}
				return
			}

			require.NoError(t, err)

			// For checkTar, we need to read the archive from the VFS and
			// write it to a real temp dir so the tar reader can open it.
			if tt.checkTar != nil {
				data, err := vfs.ReadFile(outPath)
				require.NoError(t, err)
				realPath := filepath.Join(t.TempDir(), "test.agentpack")
				err = os.WriteFile(realPath, data, 0o644)
				require.NoError(t, err)
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
					require.NoError(t, err)
				}
				outPath := filepath.Join(t.TempDir(), "rt.agentpack")
				if err := archive.Create(ctx, vfs, outPath, []archive.FileEntry{
					{Src: filepath.Join(srcDir, "data.txt"), ArchivePath: "marketplaces/pkg/data.txt"},
					{ArchivePath: "marketplaces/pkg/.agentpack/meta.json", Content: []byte(`{"v":1}`)},
				}); err != nil {
					require.NoError(t, err)
				}
				return outPath
			},
			checkFiles: func(t *testing.T, destDir string) {
				t.Helper()
				got, err := os.ReadFile(filepath.Join(destDir, "marketplaces", "pkg", "data.txt"))
				require.NoError(t, err)
				assert.Equal(t, "content", string(got))
				got2, err := os.ReadFile(
					filepath.Join(destDir, "marketplaces", "pkg", ".agentpack", "meta.json"),
				)
				require.NoError(t, err)
				assert.Equal(t, `{"v":1}`, string(got2))
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
				require.NoError(t, err)
				assert.True(t, info.IsDir())
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
				return filepath.Join(t.TempDir(), "nonexistent.agentpack")
			},
			wantErr: "open archive",
		},
		{
			name: "fails when TypeDir mkdir fails",
			buildTar: func(t *testing.T) string {
				t.Helper()
				// A tar with a TypeDir entry named "conflict/".
				outPath := filepath.Join(t.TempDir(), "withdir.agentpack")
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
					Name:     "conflict/",
					Mode:     0o755,
					Typeflag: tar.TypeDir,
				}); err != nil {
					require.NoError(t, err)
				}
				return outPath
			},
			setupDest: func(t *testing.T, destDir string) {
				t.Helper()
				// Create a regular file at the path where the directory "conflict"
				// would land. MkdirAll cannot replace a file with a directory.
				require.NoError(
					t,
					os.WriteFile(filepath.Join(destDir, "conflict"), []byte("file"), 0o644),
				)
			},
			wantErr: "mkdir",
		},
		{
			name: "fails when dest dir is read-only",
			buildTar: func(t *testing.T) string {
				t.Helper()
				ctx := context.Background()
				vfs := osfs.NewWithNoIdm()
				outPath := filepath.Join(t.TempDir(), "test.agentpack")
				if err := archive.Create(ctx, vfs, outPath, []archive.FileEntry{
					{ArchivePath: "file.txt", Content: []byte("data")},
				}); err != nil {
					require.NoError(t, err)
				}
				return outPath
			},
			setupDest: func(t *testing.T, destDir string) {
				t.Helper()
				if err := os.Chmod(destDir, 0o555); err != nil {
					require.NoError(t, err)
				}
				t.Cleanup(func() { _ = os.Chmod(destDir, 0o755) })
			},
			wantErr: "create",
		},
		{
			name: "fails on invalid gzip data",
			buildTar: func(t *testing.T) string {
				t.Helper()
				outPath := filepath.Join(t.TempDir(), "notgzip.agentpack")
				if err := os.WriteFile(outPath, []byte("this is not gzip data"), 0o644); err != nil {
					require.NoError(t, err)
				}
				return outPath
			},
			wantErr: "gzip reader",
		},
		{
			name: "fails on truncated tar data",
			buildTar: func(t *testing.T) string {
				t.Helper()
				outPath := filepath.Join(t.TempDir(), "badtar.agentpack")
				var buf bytes.Buffer
				gw := gzip.NewWriter(&buf)
				// Write incomplete/corrupted tar data inside gzip.
				_, _ = gw.Write([]byte("this is garbage tar data that will fail to parse"))
				_ = gw.Close()
				if err := os.WriteFile(outPath, buf.Bytes(), 0o644); err != nil {
					require.NoError(t, err)
				}
				return outPath
			},
			wantErr: "reading tar",
		},
		{
			name: "cancels mid-extract",
			buildTar: func(t *testing.T) string {
				t.Helper()
				ctx := context.Background()
				vfs := osfs.NewWithNoIdm()
				outPath := filepath.Join(t.TempDir(), "test.agentpack")
				if err := archive.Create(ctx, vfs, outPath, []archive.FileEntry{
					{ArchivePath: "file.txt", Content: []byte("data")},
				}); err != nil {
					require.NoError(t, err)
				}
				return outPath
			},
			setupDest: func(_ *testing.T, _ string) {},
			wantErr:   "context canceled",
		},
		{
			name: "fails when file content is truncated during extraction",
			buildTar: func(t *testing.T) string {
				t.Helper()
				// Build a tar where a file header claims 1000 bytes but only
				// 10 bytes are written. Closing only gzip (not tar) leaves the
				// stream truncated so io.Copy inside extractFile returns an error.
				outPath := filepath.Join(t.TempDir(), "truncated-content.agentpack")
				f, err := os.Create(outPath)
				if err != nil {
					require.NoError(t, err)
				}
				defer func() { _ = f.Close() }()

				gw := gzip.NewWriter(f)
				tw := tar.NewWriter(gw)

				if err := tw.WriteHeader(&tar.Header{
					Name:     "big-file.txt",
					Size:     1000,
					Mode:     0o644,
					Typeflag: tar.TypeReg,
				}); err != nil {
					require.NoError(t, err)
				}
				// Write only 10 bytes — less than the 1000 promised.
				if _, err := tw.Write([]byte("0123456789")); err != nil {
					require.NoError(t, err)
				}
				// Close only gzip, not tar — leaves the content truncated.
				if err := gw.Close(); err != nil {
					require.NoError(t, err)
				}

				return outPath
			},
			wantErr: "extract",
		},
		{
			name: "fails when TypeReg parent mkdir fails",
			buildTar: func(t *testing.T) string {
				t.Helper()
				// Build an archive with a TypeReg entry under a subdirectory.
				outPath := filepath.Join(t.TempDir(), "regdir.agentpack")
				f, err := os.Create(outPath)
				if err != nil {
					require.NoError(t, err)
				}
				defer func() { _ = f.Close() }()
				gw := gzip.NewWriter(f)
				defer func() { _ = gw.Close() }()
				tw := tar.NewWriter(gw)
				defer func() { _ = tw.Close() }()
				content := []byte("data")
				if err := tw.WriteHeader(&tar.Header{
					Name:     "conflict/file.txt",
					Size:     int64(len(content)),
					Mode:     0o644,
					Typeflag: tar.TypeReg,
				}); err != nil {
					require.NoError(t, err)
				}
				if _, err := tw.Write(content); err != nil {
					require.NoError(t, err)
				}
				return outPath
			},
			setupDest: func(t *testing.T, destDir string) {
				t.Helper()
				// Create a regular file where "conflict" directory should go.
				// os.MkdirAll("conflict") will fail because "conflict" is a file.
				require.NoError(t, os.WriteFile(
					filepath.Join(destDir, "conflict"), []byte("file"), 0o644,
				))
			},
			wantErr: "mkdir for",
		},
		{
			name: "extracts file with zero mode using default 0644",
			buildTar: func(t *testing.T) string {
				t.Helper()
				outPath := filepath.Join(t.TempDir(), "zeromode.agentpack")
				f, err := os.Create(outPath)
				if err != nil {
					require.NoError(t, err)
				}
				defer func() { _ = f.Close() }()

				gw := gzip.NewWriter(f)
				defer func() { _ = gw.Close() }()

				tw := tar.NewWriter(gw)
				defer func() { _ = tw.Close() }()

				content := []byte("zero mode file content")
				if err := tw.WriteHeader(&tar.Header{
					Name:     "zero-mode.txt",
					Size:     int64(len(content)),
					Mode:     0, // zero mode triggers the default 0644 path
					Typeflag: tar.TypeReg,
				}); err != nil {
					require.NoError(t, err)
				}
				if _, err := tw.Write(content); err != nil {
					require.NoError(t, err)
				}

				return outPath
			},
			checkFiles: func(t *testing.T, destDir string) {
				t.Helper()
				info, err := os.Stat(filepath.Join(destDir, "zero-mode.txt"))
				require.NoError(t, err)
				assert.Equal(t, fs.FileMode(0o644), info.Mode()&0o777)
			},
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

			testCtx := context.Background()
			if tt.wantErr == "context canceled" {
				cancelCtx, cancel := context.WithCancel(context.Background())
				cancel()
				testCtx = cancelCtx
			}

			err := archive.Extract(testCtx, archivePath, destDir)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

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
		require.NoError(t, err)
	}
	defer func() { _ = f.Close() }()

	gr, err := gzip.NewReader(f)
	if err != nil {
		require.NoError(t, err)
	}
	defer func() { _ = gr.Close() }()

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
		require.NoError(t, err)
	}
	defer func() { _ = f.Close() }()

	gr, err := gzip.NewReader(f)
	if err != nil {
		require.NoError(t, err)
	}
	defer func() { _ = gr.Close() }()

	tr := tar.NewReader(gr)

	for {
		hdr, err := tr.Next()
		if err != nil {
			require.FailNow(t, "")
		}
		if hdr.Name == name {
			buf := make([]byte, hdr.Size)
			_, readErr := tr.Read(buf)
			if readErr != nil && !errors.Is(readErr, io.EOF) {
				require.NoError(t, readErr)
			}
			return buf
		}
	}
}

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

func buildTarWithTraversal(t *testing.T) string {
	t.Helper()

	outPath := filepath.Join(t.TempDir(), "traversal.agentpack")
	f, err := os.Create(outPath)
	if err != nil {
		require.NoError(t, err)
	}
	defer func() { _ = f.Close() }()

	gw := gzip.NewWriter(f)
	defer func() { _ = gw.Close() }()

	tw := tar.NewWriter(gw)
	defer func() { _ = tw.Close() }()

	content := []byte("pwned")
	if err := tw.WriteHeader(&tar.Header{
		Name:     "../../../etc/evil",
		Size:     int64(len(content)),
		Mode:     0o644,
		Typeflag: tar.TypeReg,
	}); err != nil {
		require.NoError(t, err)
	}
	if _, err := tw.Write(content); err != nil {
		require.NoError(t, err)
	}

	return outPath
}

// --------------------------------------------------------------------------
// TestAddVirtualFile — white-box test via export_test.go
// --------------------------------------------------------------------------

// failAfterNWriter is an io.Writer that fails after n bytes have been written.
// It is used to make tar.Writer.Write fail without a gzip layer in between.
type failAfterNWriter struct {
	limit   int
	written int
}

func (w *failAfterNWriter) Write(p []byte) (int, error) {
	if w.written >= w.limit {
		return 0, errors.New("simulated write error after limit")
	}
	n := len(p)
	if w.written+n > w.limit {
		n = w.limit - w.written
	}
	w.written += n
	return n, nil
}

func TestAddVirtualFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		limitBytes int // bytes allowed before writer fails; -1 = unlimited
		entry      archive.FileEntry
		wantErr    bool
		wantErrStr string
	}{
		{
			name:       "succeeds with unlimited writer",
			limitBytes: -1,
			entry: archive.FileEntry{
				ArchivePath: "ok.txt",
				Content:     []byte("hello"),
			},
		},
		{
			name: "fails when tw.WriteHeader fails",
			// A header is 512 bytes. Fail before tar can write any header.
			limitBytes: 0,
			entry: archive.FileEntry{
				ArchivePath: "fail.txt",
				Content:     []byte("data"),
			},
			wantErr:    true,
			wantErrStr: "write header",
		},
		{
			name: "fails when tw.Write fails after header",
			// Allow exactly 512 bytes (one tar header block) so WriteHeader
			// succeeds but the subsequent tw.Write of content fails.
			limitBytes: 512,
			entry: archive.FileEntry{
				ArchivePath: "content-fail.txt",
				Content:     []byte("data that fails to write"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var tw *tar.Writer
			if tt.limitBytes < 0 {
				var buf bytes.Buffer
				tw = tar.NewWriter(&buf)
			} else {
				tw = tar.NewWriter(&failAfterNWriter{limit: tt.limitBytes})
			}

			err := archive.AddVirtualFile(tw, tt.entry)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrStr != "" {
					require.ErrorContains(t, err, tt.wantErrStr)
				}
				return
			}

			require.NoError(t, err)
		})
	}
}

// buildTarWithDir creates a tarball containing a TypeDir entry.
func buildTarWithDir(t *testing.T) string {
	t.Helper()

	outPath := filepath.Join(t.TempDir(), "withdir.agentpack")
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
		Name:     "mydir/",
		Typeflag: tar.TypeDir,
		Mode:     0o755,
	}); err != nil {
		require.NoError(t, err)
	}

	return outPath
}
