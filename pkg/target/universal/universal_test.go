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

package universal_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/pkg/target"
	"github.com/retr0h/agentpack/pkg/target/universal"
)

func TestUniversal_Name(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{name: "returns universal", want: "universal"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			u := universal.New()
			assert.Equal(t, tt.want, u.Name())
		})
	}
}

func TestUniversal_DisplayName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{name: "returns Universal", want: "Universal"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			u := universal.New()
			assert.Equal(t, tt.want, u.DisplayName())
		})
	}
}

func TestUniversal_Detect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want bool
	}{
		{name: "always returns true", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			u := universal.New()
			assert.Equal(t, tt.want, u.Detect())
		})
	}
}

func TestUniversal_Install(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setupSrc  func(t *testing.T) string
		cancelCtx bool
		wantErr   string
		check     func(t *testing.T, destBase string, pluginName string)
	}{
		{
			name: "copies skills into .agents/skills/{name}/",
			setupSrc: func(t *testing.T) string {
				t.Helper()
				src := t.TempDir()
				skillsDir := filepath.Join(src, "skills")

				require.NoError(t, os.MkdirAll(skillsDir, 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(skillsDir, "skill.md"),
					[]byte("# Skill"),
					0o644,
				))

				return src
			},
			check: func(t *testing.T, destBase string, pluginName string) {
				t.Helper()
				p := filepath.Join(destBase, ".agents", "skills", pluginName, "skill.md")
				_, err := os.Stat(p)
				assert.NoError(t, err)
			},
		},
		{
			name: "no skills dir is a no-op",
			setupSrc: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
		},
		{
			name:      "cancelled context returns error",
			cancelCtx: true,
			setupSrc: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			wantErr: "context canceled",
		},
		{
			name: "cwdFunc error propagates",
			setupSrc: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			wantErr: "getwd",
		},
		{
			name: "mkdirAll failure for destDir propagates error",
			setupSrc: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			wantErr: "mkdir universal skills dir",
		},
		{
			name: "copyTreeIfExists error propagates from Install",
			setupSrc: func(t *testing.T) string {
				t.Helper()
				src := t.TempDir()
				skillsDir := filepath.Join(src, "skills")
				locked := filepath.Join(skillsDir, "locked")
				if err := os.MkdirAll(locked, 0o755); err != nil {
					require.NoError(t, err)
				}
				if err := os.WriteFile(filepath.Join(locked, "x.md"), []byte("x"), 0o644); err != nil {
					require.NoError(t, err)
				}
				if err := os.Chmod(locked, 0o000); err != nil {
					require.NoError(t, err)
				}
				t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
				return src
			},
			wantErr: "copy skills",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srcDir := tt.setupSrc(t)
			destBase := t.TempDir()

			cwdFunc := func() (string, error) { return destBase, nil }
			if tt.wantErr == "getwd" {
				cwdFunc = func() (string, error) { return "", errors.New("getwd failed") }
			}
			if tt.wantErr == "mkdir universal skills dir" {
				roDir := t.TempDir()
				require.NoError(t, os.Chmod(roDir, 0o555))
				t.Cleanup(func() { _ = os.Chmod(roDir, 0o755) })
				cwdFunc = func() (string, error) { return roDir, nil }
			}

			u := universal.NewWithCWD(cwdFunc)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if tt.cancelCtx {
				cancel()
			}

			opts := target.InstallOpts{
				Name:      "my-plugin",
				SourceDir: srcDir,
			}

			err := u.Install(ctx, opts)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, destBase, opts.Name)
			}
		})
	}
}

func TestUniversal_List(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		wantLen int
	}{
		{name: "returns empty slice", wantLen: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			u := universal.New()
			got, err := u.List()

			require.NoError(t, err)
			assert.Len(t, got, tt.wantLen)
		})
	}
}

func TestUniversal_CopyTreeIfExists(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (src, dst string)
		ctx     func() context.Context
		wantErr string
	}{
		{
			name: "no-op when src does not exist",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return filepath.Join(t.TempDir(), "nonexistent"), t.TempDir()
			},
			ctx: func() context.Context { return context.Background() },
		},
		{
			name: "copies files when src exists",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				if err := os.WriteFile(filepath.Join(src, "a.md"), []byte("a"), 0o644); err != nil {
					require.NoError(t, err)
				}
				return src, t.TempDir()
			},
			ctx: func() context.Context { return context.Background() },
		},
		{
			name: "cancelled context inside walk returns error",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				if err := os.WriteFile(filepath.Join(src, "a.md"), []byte("a"), 0o644); err != nil {
					require.NoError(t, err)
				}
				return src, t.TempDir()
			},
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			wantErr: "context canceled",
		},
		{
			name: "walkDir error on unreadable subdirectory",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				locked := filepath.Join(src, "locked")
				if err := os.MkdirAll(locked, 0o755); err != nil {
					require.NoError(t, err)
				}
				if err := os.WriteFile(filepath.Join(locked, "x.md"), []byte("x"), 0o644); err != nil {
					require.NoError(t, err)
				}
				if err := os.Chmod(locked, 0o000); err != nil {
					require.NoError(t, err)
				}
				t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
				return src, t.TempDir()
			},
			ctx:     func() context.Context { return context.Background() },
			wantErr: "permission denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src, dst := tt.setup(t)
			err := universal.CopyTreeIfExists(tt.ctx(), src, dst)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestUniversal_CopyFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (src, dst string)
		wantErr string
	}{
		{
			name: "copies file successfully",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := filepath.Join(t.TempDir(), "src.txt")
				if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
					require.NoError(t, err)
				}
				return src, filepath.Join(t.TempDir(), "dst.txt")
			},
		},
		{
			name: "read error on missing src",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return filepath.Join(t.TempDir(), "missing.txt"), filepath.Join(t.TempDir(), "dst.txt")
			},
			wantErr: "read",
		},
		{
			name: "write error when dst is an existing directory",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := filepath.Join(t.TempDir(), "src.txt")
				if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
					require.NoError(t, err)
				}
				dstParent := t.TempDir()
				existingDir := filepath.Join(dstParent, "subdir")
				if err := os.MkdirAll(existingDir, 0o755); err != nil {
					require.NoError(t, err)
				}
				return src, existingDir
			},
			wantErr: "write",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src, dst := tt.setup(t)
			err := universal.CopyFile(src, dst)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
		})
	}
}
