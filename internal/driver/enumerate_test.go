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

package driver_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/internal/driver"
)

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
		{
			name: "returns error when file is unreadable",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				if os.Getuid() == 0 {
					t.Skip("root bypasses file permissions")
				}
				base := t.TempDir()
				dest := filepath.Join(base, "skills")
				require.NoError(t, os.MkdirAll(dest, 0o755))
				p := filepath.Join(dest, "secret.md")
				require.NoError(t, os.WriteFile(p, []byte("secret"), 0o000))
				t.Cleanup(func() { _ = os.Chmod(p, 0o644) })

				return dest, base
			},
			wantErr: "permission denied",
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

			files, err := driver.EnumerateFiles(ctx, destDir, baseDir)

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
