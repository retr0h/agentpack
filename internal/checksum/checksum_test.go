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
	"os"
	"path/filepath"
	"testing"

	"github.com/retr0h/claudia/internal/checksum"
)

func TestComputeBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		data    []byte
		wantLen int
	}{
		{
			name:    "non-empty data produces 64-char hex hash",
			data:    []byte("hello, claudia"),
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

	tests := []struct {
		name        string
		content     []byte
		missingFile bool
		wantLen     int
		wantErr     bool
	}{
		{
			name:    "non-empty file produces 64-char hex hash",
			content: []byte("hello, claudia"),
			wantLen: 64,
		},
		{
			name:    "empty file produces 64-char hex hash",
			content: []byte{},
			wantLen: 64,
		},
		{
			name:    "binary-like content produces 64-char hex hash",
			content: []byte{0x00, 0xFF, 0x1A, 0x2B, 0x3C, 0xDE, 0xAD, 0xBE, 0xEF},
			wantLen: 64,
		},
		{
			name:        "missing file returns error",
			missingFile: true,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var path string

			if tt.missingFile {
				path = filepath.Join(t.TempDir(), "nonexistent.txt")
			} else {
				path = filepath.Join(t.TempDir(), "input.bin")
				if err := os.WriteFile(path, tt.content, 0o600); err != nil {
					t.Fatalf("setup: write file: %v", err)
				}
			}

			got, err := checksum.ComputeFile(path)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
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

	tests := []struct {
		name      string
		path      func(dir string) string // returns the path to write to
		entries   []checksum.Entry
		wantErr   bool
		checkRead func(t *testing.T, path string, want []checksum.Entry)
	}{
		{
			name: "writes two entries readable by ReadFile",
			path: func(dir string) string { return filepath.Join(dir, "checksums.txt") },
			entries: []checksum.Entry{
				{Hash: "abc123def456abc123def456abc123def456abc123def456abc123def456abcd", Path: "file1.txt"},
				{Hash: "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210", Path: "subdir/file2.txt"},
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
			name:    "fails when directory does not exist",
			path:    func(dir string) string { return filepath.Join(dir, "nonexistent", "checksums.txt") },
			entries: []checksum.Entry{{Hash: "abc", Path: "f.txt"}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := tt.path(t.TempDir())
			err := checksum.WriteFile(p, tt.entries)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.checkRead != nil {
				tt.checkRead(t, p, tt.entries)
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
				{Hash: "abc123def456abc123def456abc123def456abc123def456abc123def456abcd", Path: "file1.txt"},
				{Hash: "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210", Path: "subdir/file2.txt"},
			},
		},
		{
			name:    "skips blank lines",
			content: "abc123def456abc123def456abc123def456abc123def456abc123def456abcd  file.txt\n\n",
			want: []checksum.Entry{
				{Hash: "abc123def456abc123def456abc123def456abc123def456abc123def456abcd", Path: "file.txt"},
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
		wantOK     []bool
		wantErrStr []string // empty string means no specific check
	}{
		{
			name: "matching checksum passes",
			setupFiles: map[string][]byte{
				"file.txt": []byte("hello"),
			},
			// SHA256("hello") = 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824
			entries: []checksum.Entry{
				{Hash: "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", Path: "file.txt"},
			},
			wantOK: []bool{true},
		},
		{
			name: "mismatched checksum fails",
			setupFiles: map[string][]byte{
				"file.txt": []byte("hello"),
			},
			entries: []checksum.Entry{
				{Hash: "0000000000000000000000000000000000000000000000000000000000000000", Path: "file.txt"},
			},
			wantOK: []bool{false},
		},
		{
			name:       "missing file fails",
			setupFiles: map[string][]byte{},
			entries: []checksum.Entry{
				{Hash: "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", Path: "ghost.txt"},
			},
			wantOK: []bool{false},
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

			results, err := checksum.Verify(dir, tt.entries)
			if err != nil {
				t.Fatalf("Verify returned unexpected error: %v", err)
			}

			if len(results) != len(tt.wantOK) {
				t.Fatalf("result count = %d, want %d", len(results), len(tt.wantOK))
			}

			for i, want := range tt.wantOK {
				if results[i].OK != want {
					t.Errorf("results[%d].OK = %v, want %v (Err=%q)", i, results[i].OK, want, results[i].Err)
				}

				if !want && results[i].Err == "" {
					t.Errorf("results[%d].OK=false but Err is empty", i)
				}
			}
		})
	}
}
