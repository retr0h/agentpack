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

package checksum_test

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/avfs/avfs"
	"github.com/avfs/avfs/vfs/memfs"

	"github.com/retr0h/agentpack/pkg/checksum"
)

// --------------------------------------------------------------------------
// Error-injecting VFS helpers
// --------------------------------------------------------------------------

// openErrorVFS wraps avfs.VFS and returns an error from Open.
type openErrorVFS struct {
	avfs.VFS
}

func (openErrorVFS) Open(string) (avfs.File, error) {
	return nil, errors.New("simulated open error")
}

// copyErrorFile wraps avfs.File so that Read returns an error to trigger the
// io.Copy path in ComputeFile.
type copyErrorFile struct {
	avfs.File
}

func (copyErrorFile) Read([]byte) (int, error) {
	return 0, errors.New("simulated read error")
}

func (copyErrorFile) Close() error { return nil }

// copyErrorVFS returns a copyErrorFile from Open.
type copyErrorVFS struct {
	avfs.VFS
}

func (v copyErrorVFS) Open(name string) (avfs.File, error) {
	f, err := v.VFS.Open(name)
	if err != nil {
		return nil, err
	}
	_ = f.Close()
	return copyErrorFile{}, nil
}

// createErrorVFS wraps avfs.VFS and returns an error from Create.
type createErrorVFS struct {
	avfs.VFS
}

func (createErrorVFS) Create(string) (avfs.File, error) {
	return nil, errors.New("simulated create error")
}

// flushErrorFile is an avfs.File whose underlying writer fails on Flush.
type flushErrorFile struct {
	avfs.File
}

func (flushErrorFile) Write([]byte) (int, error) {
	return 0, errors.New("simulated write error")
}

func (flushErrorFile) Close() error { return nil }

// flushErrorVFS returns a flushErrorFile from Create.
type flushErrorVFS struct {
	avfs.VFS
}

func (v flushErrorVFS) Create(string) (avfs.File, error) {
	return flushErrorFile{}, nil
}

// successWriteCloseErrorFile writes successfully but Close returns an error.
// This exercises the f.Close() error branch in WriteFile.
type successWriteCloseErrorFile struct {
	avfs.File
	buf []byte
}

func (f *successWriteCloseErrorFile) Write(p []byte) (int, error) {
	f.buf = append(f.buf, p...)
	return len(p), nil
}

func (*successWriteCloseErrorFile) Close() error {
	return errors.New("simulated close error")
}

// closeErrorVFS returns a successWriteCloseErrorFile from Create.
type closeErrorVFS struct {
	avfs.VFS
}

func (closeErrorVFS) Create(string) (avfs.File, error) {
	return &successWriteCloseErrorFile{}, nil
}

// --------------------------------------------------------------------------
// Tests
// --------------------------------------------------------------------------

func TestComputeBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		data    []byte
		wantLen int
	}{
		{
			name:    "non-empty data produces 64-char hex hash",
			data:    []byte("hello, agentpack"),
			wantLen: 64,
		},
		{
			name:    "empty data produces 64-char hex hash",
			data:    []byte{},
			wantLen: 64,
		},
		{
			name:    "same input produces same hash",
			data:    []byte("deterministic"),
			wantLen: 64,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := checksum.ComputeBytes(tt.data)
			if len(got) != tt.wantLen {
				t.Errorf("hash length = %d, want %d (hash = %q)", len(got), tt.wantLen, got)
			}

			again := checksum.ComputeBytes(tt.data)
			if got != again {
				t.Errorf("non-deterministic: %q != %q", got, again)
			}
		})
	}
}

func TestComputeFile(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tests := []struct {
		name        string
		setupVFS    func(t *testing.T) (avfs.VFS, string)
		wantLen     int
		wantErr     bool
		wantErrFrag string
	}{
		{
			name: "non-empty file produces 64-char hex hash",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				vfs := memfs.New()
				if err := vfs.WriteFile("/input.bin", []byte("hello, agentpack"), fs.FileMode(0o600)); err != nil {
					t.Fatal(err)
				}
				return vfs, "/input.bin"
			},
			wantLen: 64,
		},
		{
			name: "empty file produces 64-char hex hash",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				vfs := memfs.New()
				if err := vfs.WriteFile("/empty.bin", []byte{}, fs.FileMode(0o600)); err != nil {
					t.Fatal(err)
				}
				return vfs, "/empty.bin"
			},
			wantLen: 64,
		},
		{
			name: "binary-like content produces 64-char hex hash",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				vfs := memfs.New()
				if err := vfs.WriteFile("/bin.bin", []byte{0x00, 0xFF, 0x1A, 0x2B, 0x3C, 0xDE, 0xAD, 0xBE, 0xEF}, fs.FileMode(0o600)); err != nil {
					t.Fatal(err)
				}
				return vfs, "/bin.bin"
			},
			wantLen: 64,
		},
		{
			name: "missing file returns error",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return memfs.New(), "/nonexistent.txt"
			},
			wantErr: true,
		},
		{
			name: "open error returns error",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return openErrorVFS{VFS: memfs.New()}, "/any.txt"
			},
			wantErr:     true,
			wantErrFrag: "simulated open error",
		},
		{
			name: "io.Copy error returns error",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				vfs := memfs.New()
				if err := vfs.WriteFile("/data.bin", []byte("data"), fs.FileMode(0o600)); err != nil {
					t.Fatal(err)
				}
				return copyErrorVFS{VFS: vfs}, "/data.bin"
			},
			wantErr:     true,
			wantErrFrag: "simulated read error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			vfs, path := tt.setupVFS(t)
			got, err := checksum.ComputeFile(ctx, vfs, path)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantErrFrag != "" {
					if err.Error() == "" {
						t.Errorf("expected error containing %q, got empty", tt.wantErrFrag)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(got) != tt.wantLen {
				t.Errorf("hash length = %d, want %d (hash = %q)", len(got), tt.wantLen, got)
			}
		})
	}
}

func TestWriteFile(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tests := []struct {
		name        string
		setupVFS    func(t *testing.T) (avfs.VFS, string)
		entries     []checksum.Entry
		wantErr     bool
		wantErrFrag string
		checkRead   func(t *testing.T, path string, want []checksum.Entry)
	}{
		{
			name: "writes two entries readable by ReadFile",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return memfs.New(), "/checksums.txt"
			},
			entries: []checksum.Entry{
				{
					Hash: "abc123def456abc123def456abc123def456abc123def456abc123def456abcd",
					Path: "file1.txt",
				},
				{
					Hash: "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210",
					Path: "subdir/file2.txt",
				},
			},
			checkRead: func(t *testing.T, path string, want []checksum.Entry) {
				t.Helper()
				got, err := checksum.ReadFile(path)
				if err != nil {
					t.Fatalf("ReadFile: %v", err)
				}
				if len(got) != len(want) {
					t.Fatalf("entry count = %d, want %d", len(got), len(want))
				}
				for i, w := range want {
					if got[i].Hash != w.Hash {
						t.Errorf("entry[%d].Hash = %q, want %q", i, got[i].Hash, w.Hash)
					}
					if got[i].Path != w.Path {
						t.Errorf("entry[%d].Path = %q, want %q", i, got[i].Path, w.Path)
					}
				}
			},
		},
		{
			name: "fails when create returns error",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return createErrorVFS{VFS: memfs.New()}, "/checksums.txt"
			},
			entries: []checksum.Entry{{Hash: "abc", Path: "f.txt"}},
			wantErr: true,
		},
		{
			name: "fails when write returns error during flush",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return flushErrorVFS{VFS: memfs.New()}, "/checksums.txt"
			},
			entries:     []checksum.Entry{{Hash: "abc", Path: "f.txt"}},
			wantErr:     true,
			wantErrFrag: "flush",
		},
		{
			name: "fails when write returns error during fprintf",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return flushErrorVFS{VFS: memfs.New()}, "/checksums.txt"
			},
			// A very long path that exceeds the 4096-byte bufio buffer forces an
			// immediate write to the underlying file, causing fmt.Fprintf to fail.
			entries: []checksum.Entry{{
				Hash: "abc123def456abc123def456abc123def456abc123def456abc123def456abcd",
				Path: string(make([]byte, 5000)),
			}},
			wantErr:     true,
			wantErrFrag: "write entry",
		},
		{
			name: "fails when directory does not exist on real OS",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				// Use the real OS filesystem via a path that doesn't exist.
				// We need an OS-backed VFS for this test since memfs doesn't
				// enforce parent directory existence the same way.
				vfs := memfs.New()
				return vfs, "/nonexistent/dir/checksums.txt"
			},
			entries: []checksum.Entry{{Hash: "abc", Path: "f.txt"}},
			wantErr: true,
		},
		{
			name: "fails when file close returns error",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return closeErrorVFS{VFS: memfs.New()}, "/checksums.txt"
			},
			entries: []checksum.Entry{
				{
					Hash: "abc123def456abc123def456abc123def456abc123def456abc123def456abcd",
					Path: "f.txt",
				},
			},
			wantErr:     true,
			wantErrFrag: "close",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			vfs, path := tt.setupVFS(t)
			err := checksum.WriteFile(ctx, vfs, path, tt.entries)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// For the round-trip check, write to OS temp dir then read from there.
			if tt.checkRead != nil {
				// Write to a real OS file so ReadFile (which uses os.Open) can read it.
				osPath := filepath.Join(t.TempDir(), "checksums.txt")
				osVFS := memfs.New()
				if err2 := checksum.WriteFile(ctx, osVFS, "/checksums.txt", tt.entries); err2 != nil {
					t.Fatalf("write to memfs: %v", err2)
				}
				// Read from memfs and write to real OS file for ReadFile to consume.
				data, err2 := osVFS.ReadFile("/checksums.txt")
				if err2 != nil {
					t.Fatalf("read from memfs: %v", err2)
				}
				if err2 := os.WriteFile(osPath, data, 0o600); err2 != nil {
					t.Fatalf("write to OS: %v", err2)
				}
				tt.checkRead(t, osPath, tt.entries)
			}
		})
	}
}

func TestReadFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string // file content to write; empty means no file
		want    []checksum.Entry
		wantErr bool
	}{
		{
			name:    "parses two valid entries",
			content: "abc123def456abc123def456abc123def456abc123def456abc123def456abcd  file1.txt\nfedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210  subdir/file2.txt\n",
			want: []checksum.Entry{
				{
					Hash: "abc123def456abc123def456abc123def456abc123def456abc123def456abcd",
					Path: "file1.txt",
				},
				{
					Hash: "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210",
					Path: "subdir/file2.txt",
				},
			},
		},
		{
			name:    "skips blank lines",
			content: "abc123def456abc123def456abc123def456abc123def456abc123def456abcd  file.txt\n\n",
			want: []checksum.Entry{
				{
					Hash: "abc123def456abc123def456abc123def456abc123def456abc123def456abcd",
					Path: "file.txt",
				},
			},
		},
		{
			name:    "returns error for malformed line",
			content: "badhash file.txt\n",
			wantErr: true,
		},
		{
			name:    "missing file returns error",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var p string
			if tt.content != "" {
				p = filepath.Join(t.TempDir(), "checksums.txt")
				if err := os.WriteFile(p, []byte(tt.content), 0o600); err != nil {
					t.Fatalf("setup: write file: %v", err)
				}
			} else {
				p = filepath.Join(t.TempDir(), "nonexistent.txt")
			}

			got, err := checksum.ReadFile(p)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("entry count = %d, want %d", len(got), len(tt.want))
			}

			for i, w := range tt.want {
				if got[i].Hash != w.Hash {
					t.Errorf("entry[%d].Hash = %q, want %q", i, got[i].Hash, w.Hash)
				}
				if got[i].Path != w.Path {
					t.Errorf("entry[%d].Path = %q, want %q", i, got[i].Path, w.Path)
				}
			}
		})
	}
}

func TestVerify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setupFiles map[string][]byte // relative path -> content
		entries    []checksum.Entry
		cancelCtx  bool
		wantOK     []bool
		wantErrStr []string // empty string means no specific check
		wantErr    bool
	}{
		{
			name: "matching checksum passes",
			setupFiles: map[string][]byte{
				"file.txt": []byte("hello"),
			},
			// SHA256("hello") = 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824
			entries: []checksum.Entry{
				{
					Hash: "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
					Path: "file.txt",
				},
			},
			wantOK: []bool{true},
		},
		{
			name: "mismatched checksum fails",
			setupFiles: map[string][]byte{
				"file.txt": []byte("hello"),
			},
			entries: []checksum.Entry{
				{
					Hash: "0000000000000000000000000000000000000000000000000000000000000000",
					Path: "file.txt",
				},
			},
			wantOK: []bool{false},
		},
		{
			name:       "missing file fails",
			setupFiles: map[string][]byte{},
			entries: []checksum.Entry{
				{
					Hash: "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
					Path: "ghost.txt",
				},
			},
			wantOK: []bool{false},
		},
		{
			name: "cancelled context returns error",
			setupFiles: map[string][]byte{
				"file.txt": []byte("hello"),
			},
			entries: []checksum.Entry{
				{
					Hash: "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
					Path: "file.txt",
				},
			},
			cancelCtx: true,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()

			for rel, content := range tt.setupFiles {
				path := filepath.Join(dir, rel)
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatalf("setup: mkdir: %v", err)
				}

				if err := os.WriteFile(path, content, 0o600); err != nil {
					t.Fatalf("setup: write %s: %v", rel, err)
				}
			}

			ctx := context.Background()
			if tt.cancelCtx {
				cancelCtx, cancel := context.WithCancel(context.Background())
				cancel()
				ctx = cancelCtx
			}

			results, err := checksum.Verify(ctx, dir, tt.entries)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Verify returned unexpected error: %v", err)
			}

			if len(results) != len(tt.wantOK) {
				t.Fatalf("result count = %d, want %d", len(results), len(tt.wantOK))
			}

			for i, want := range tt.wantOK {
				if results[i].OK != want {
					t.Errorf(
						"results[%d].OK = %v, want %v (Err=%q)",
						i,
						results[i].OK,
						want,
						results[i].Err,
					)
				}

				if !want && results[i].Err == "" {
					t.Errorf("results[%d].OK=false but Err is empty", i)
				}
			}
		})
	}
}

// Compile-time interface checks.
var (
	_ io.Reader = copyErrorFile{}
	_ io.Writer = flushErrorFile{}
)
