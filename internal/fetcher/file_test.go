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

package fetcher_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/internal/fetcher"
	"github.com/retr0h/agentpack/internal/testutil"
)

// failOnCloseWriter is an io.WriteCloser that succeeds on Write but fails on Close.
type failOnCloseWriter struct{}

func (*failOnCloseWriter) Write(p []byte) (int, error) { return len(p), nil }

func (*failOnCloseWriter) Close() error { return errors.New("simulated close error") }

func TestFileFetcherFetch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setup       func(t *testing.T) (source string, dest string)
		customCtx   context.Context    // if set, use this context instead
		injectFuncs func(t *testing.T) // if set, inject function vars (not parallel-safe)
		noParallel  bool               // if true, do not run subtest in parallel
		wantErr     string
		check       func(t *testing.T, dest string)
	}{
		{
			name: "copies existing file to dest",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				src := filepath.Join(dir, "archive.agentpack")
				require.NoError(t, os.WriteFile(src, []byte("archive data"), 0o644))
				dest := filepath.Join(dir, "copy.agentpack")
				return src, dest
			},
			check: func(t *testing.T, dest string) {
				t.Helper()
				data, err := os.ReadFile(dest)
				require.NoError(t, err)
				assert.Equal(t, "archive data", string(data))
			},
		},
		{
			name: "returns error when source file missing",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				return filepath.Join(
						dir,
						"nonexistent.agentpack",
					), filepath.Join(
						dir,
						"dest.agentpack",
					)
			},
			wantErr: "open source",
		},
		{
			name: "returns error when dest dir missing",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				src := filepath.Join(dir, "archive.agentpack")
				require.NoError(t, os.WriteFile(src, []byte("data"), 0o644))
				return src, filepath.Join(dir, "nonexistent", "dest.agentpack")
			},
			wantErr: "create dest",
		},
		{
			name: "returns error when context is cancelled",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				src := filepath.Join(dir, "archive.agentpack")
				require.NoError(t, os.WriteFile(src, []byte("data"), 0o644))
				return src, filepath.Join(dir, "dest.agentpack")
			},
			wantErr: "context canceled",
		},
		{
			name: "returns error when context is cancelled after opening source",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				src := filepath.Join(dir, "archive.agentpack")
				require.NoError(t, os.WriteFile(src, []byte("data"), 0o644))
				return src, filepath.Join(dir, "dest.agentpack")
			},
			// NewCancelAfterN(1): first Err() call returns nil (1 > 1 is false),
			// second call fires cancel (2 > 1 is true) and returns context.Canceled.
			customCtx: testutil.NewCancelAfterN(1),
			wantErr:   "context canceled",
		},
		{
			name:       "returns error when os.UserHomeDir fails during expandHome",
			noParallel: true,
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return "~/somefile.agentpack", filepath.Join(t.TempDir(), "dest.agentpack")
			},
			injectFuncs: func(t *testing.T) {
				t.Helper()
				restore := fetcher.SetOsUserHomeDir(func() (string, error) {
					return "", errors.New("home dir unavailable")
				})
				t.Cleanup(restore)
			},
			wantErr: "expand path",
		},
		{
			name:       "returns error when io.Copy fails",
			noParallel: true,
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				src := filepath.Join(dir, "archive.agentpack")
				require.NoError(t, os.WriteFile(src, []byte("data"), 0o644))
				return src, filepath.Join(dir, "dest.agentpack")
			},
			injectFuncs: func(t *testing.T) {
				t.Helper()
				restore := fetcher.SetIoCopyFile(func(_ io.Writer, _ io.Reader) (int64, error) {
					return 0, errors.New("simulated copy error")
				})
				t.Cleanup(restore)
			},
			wantErr: "copy",
		},
		{
			name:       "returns error when os.Create fails",
			noParallel: true,
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				src := filepath.Join(dir, "archive.agentpack")
				require.NoError(t, os.WriteFile(src, []byte("data"), 0o644))
				return src, filepath.Join(dir, "dest.agentpack")
			},
			injectFuncs: func(t *testing.T) {
				t.Helper()
				restore := fetcher.SetOsCreateFile(func(_ string) (io.WriteCloser, error) {
					return nil, errors.New("simulated create error")
				})
				t.Cleanup(restore)
			},
			wantErr: "create dest",
		},
		{
			name:       "returns error when dest close fails",
			noParallel: true,
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				src := filepath.Join(dir, "archive.agentpack")
				require.NoError(t, os.WriteFile(src, []byte("data"), 0o644))
				return src, filepath.Join(dir, "dest.agentpack")
			},
			injectFuncs: func(t *testing.T) {
				t.Helper()
				restore := fetcher.SetOsCreateFile(func(_ string) (io.WriteCloser, error) {
					return &failOnCloseWriter{}, nil
				})
				t.Cleanup(restore)
			},
			wantErr: "close dest",
		},
		{
			name: "copies file referenced with home-relative path",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				home, err := os.UserHomeDir()
				if err != nil {
					t.Skipf("cannot get home dir: %v", err)
				}
				// Create a temp file inside the real home dir.
				f, err := os.CreateTemp(home, "agentpack-test-*.agentpack")
				if err != nil {
					t.Skipf("cannot create temp file in home: %v", err)
				}
				_ = f.Close()
				t.Cleanup(func() { _ = os.Remove(f.Name()) })

				require.NoError(t, os.WriteFile(f.Name(), []byte("home data"), 0o644))

				// Build a ~/basename source path.
				rel := "~/" + filepath.Base(f.Name())
				dest := filepath.Join(t.TempDir(), "dest.agentpack")
				return rel, dest
			},
			check: func(t *testing.T, dest string) {
				t.Helper()
				data, err := os.ReadFile(dest)
				require.NoError(t, err)
				assert.Equal(t, "home data", string(data))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.noParallel {
				t.Parallel()
			}

			if tt.injectFuncs != nil {
				tt.injectFuncs(t)
			}

			source, dest := tt.setup(t)

			var ctx context.Context
			var cancel context.CancelFunc

			switch {
			case tt.customCtx != nil:
				ctx = tt.customCtx
				cancel = func() {}
			case strings.Contains(tt.wantErr, "context canceled"):
				ctx, cancel = context.WithCancel(context.Background())
				cancel()
			default:
				ctx = context.Background()
				cancel = func() {}
			}
			defer cancel()

			f := &fetcher.FileFetcher{}
			err := f.Fetch(ctx, source, dest)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, dest)
			}
		})
	}
}
