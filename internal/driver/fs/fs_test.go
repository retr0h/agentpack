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

package fs_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/internal/driver/fs"
)

func TestCopyFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (src, dst string)
		wantErr string
	}{
		{
			name: "copies file preserving content and permissions",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := filepath.Join(t.TempDir(), "src.txt")
				require.NoError(t, os.WriteFile(src, []byte("hello"), 0o644))

				return src, filepath.Join(t.TempDir(), "dst.txt")
			},
		},
		{
			name: "error when source does not exist",
			setup: func(t *testing.T) (string, string) {
				t.Helper()

				return filepath.Join(t.TempDir(), "missing.txt"),
					filepath.Join(t.TempDir(), "dst.txt")
			},
			wantErr: "stat",
		},
		{
			name: "error when source is a symlink",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				target := filepath.Join(dir, "real.txt")
				require.NoError(t, os.WriteFile(target, []byte("secret"), 0o644))
				link := filepath.Join(dir, "link.txt")
				require.NoError(t, os.Symlink(target, link))

				return link, filepath.Join(t.TempDir(), "dst.txt")
			},
			wantErr: "symlinks not allowed",
		},
		{
			name: "error when destination is a directory",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := filepath.Join(t.TempDir(), "src.txt")
				require.NoError(t, os.WriteFile(src, []byte("data"), 0o644))
				dstDir := t.TempDir()

				return src, dstDir
			},
			wantErr: "is a directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src, dst := tt.setup(t)
			err := fs.CopyFile(src, dst)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)

			srcData, _ := os.ReadFile(src)
			dstData, _ := os.ReadFile(dst)
			assert.Equal(t, srcData, dstData)
		})
	}
}

func TestCopyTreeIfExists(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (src, dst string)
		wantErr string
		check   func(t *testing.T, dst string)
	}{
		{
			name: "copies directory tree recursively",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(src, "sub"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(src, "a.txt"), []byte("a"), 0o644))
				require.NoError(
					t,
					os.WriteFile(filepath.Join(src, "sub", "b.txt"), []byte("b"), 0o644),
				)

				return src, filepath.Join(t.TempDir(), "out")
			},
			check: func(t *testing.T, dst string) {
				t.Helper()
				data, err := os.ReadFile(filepath.Join(dst, "a.txt"))
				require.NoError(t, err)
				assert.Equal(t, []byte("a"), data)

				data, err = os.ReadFile(filepath.Join(dst, "sub", "b.txt"))
				require.NoError(t, err)
				assert.Equal(t, []byte("b"), data)
			},
		},
		{
			name: "skips symlinks in source tree",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				require.NoError(
					t,
					os.WriteFile(filepath.Join(src, "real.txt"), []byte("ok"), 0o644),
				)
				require.NoError(
					t,
					os.Symlink(filepath.Join(src, "real.txt"), filepath.Join(src, "link.txt")),
				)

				return src, filepath.Join(t.TempDir(), "out")
			},
			check: func(t *testing.T, dst string) {
				t.Helper()
				// Real file was copied.
				_, err := os.Stat(filepath.Join(dst, "real.txt"))
				require.NoError(t, err)
				// Symlink was skipped.
				_, err = os.Stat(filepath.Join(dst, "link.txt"))
				assert.True(t, os.IsNotExist(err))
			},
		},
		{
			name: "no-op when source does not exist",
			setup: func(t *testing.T) (string, string) {
				t.Helper()

				return filepath.Join(t.TempDir(), "nope"), filepath.Join(t.TempDir(), "out")
			},
			check: func(t *testing.T, dst string) {
				t.Helper()
				_, err := os.Stat(dst)
				assert.True(t, os.IsNotExist(err))
			},
		},
		{
			name: "respects context cancellation",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(src, "f.txt"), []byte("x"), 0o644))

				return src, filepath.Join(t.TempDir(), "out")
			},
			wantErr: "context canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src, dst := tt.setup(t)

			ctx := context.Background()
			if tt.wantErr == "context canceled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			err := fs.CopyTreeIfExists(ctx, src, dst)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, dst)
			}
		})
	}
}

func TestEnumerateFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (destDir, baseDir string)
		want    int
		wantErr string
	}{
		{
			name: "enumerates files with relative paths and SHA256",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				base := t.TempDir()
				dest := filepath.Join(base, "skills", "demo")
				require.NoError(t, os.MkdirAll(dest, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(dest, "a.md"), []byte("# A"), 0o644))
				require.NoError(t, os.WriteFile(filepath.Join(dest, "b.md"), []byte("# B"), 0o644))

				return dest, base
			},
			want: 2,
		},
		{
			name: "empty directory returns no files",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				base := t.TempDir()
				dest := filepath.Join(base, "empty")
				require.NoError(t, os.MkdirAll(dest, 0o755))

				return dest, base
			},
			want: 0,
		},
		{
			name: "respects context cancellation",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				base := t.TempDir()
				dest := filepath.Join(base, "skills")
				require.NoError(t, os.MkdirAll(dest, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(dest, "f.md"), []byte("x"), 0o644))

				return dest, base
			},
			wantErr: "context canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			destDir, baseDir := tt.setup(t)

			ctx := context.Background()
			if tt.wantErr == "context canceled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			files, err := fs.EnumerateFiles(ctx, destDir, baseDir)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Len(t, files, tt.want)

			for _, f := range files {
				assert.NotEmpty(t, f.Path)
				assert.NotEmpty(t, f.SHA256)
				assert.Len(t, f.SHA256, 64)
			}
		})
	}
}
