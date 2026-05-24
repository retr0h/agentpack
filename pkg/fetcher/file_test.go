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
	"time"

	"github.com/retr0h/claudia/pkg/fetcher"
)

// failOnCloseWriter is an io.WriteCloser that succeeds on Write but fails on Close.
type failOnCloseWriter struct{}

func (*failOnCloseWriter) Write(p []byte) (int, error) { return len(p), nil }

func (*failOnCloseWriter) Close() error { return errors.New("simulated close error") }

// cancelOnSecondCallCtx is a context.Context whose Err() returns nil on the
// first call and context.Canceled on all subsequent calls. This allows a test
// to pass the initial ctx.Err() check in FileFetcher.Fetch but then cancel
// before the second check (after os.Open succeeds).
type cancelOnSecondCallCtx struct {
	callCount int
}

func (c *cancelOnSecondCallCtx) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *cancelOnSecondCallCtx) Done() <-chan struct{}       { return nil }
func (c *cancelOnSecondCallCtx) Value(_ any) any             { return nil }
func (c *cancelOnSecondCallCtx) Err() error {
	c.callCount++
	if c.callCount == 1 {
		return nil
	}
	return errors.New("context canceled")
}

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
				src := filepath.Join(dir, "archive.claudia")
				if err := os.WriteFile(src, []byte("archive data"), 0o644); err != nil {
					t.Fatalf("write source: %v", err)
				}
				dest := filepath.Join(dir, "copy.claudia")
				return src, dest
			},
			check: func(t *testing.T, dest string) {
				t.Helper()
				data, err := os.ReadFile(dest)
				if err != nil {
					t.Fatalf("read dest: %v", err)
				}
				if string(data) != "archive data" {
					t.Errorf("dest content = %q, want %q", string(data), "archive data")
				}
			},
		},
		{
			name: "returns error when source file missing",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				return filepath.Join(dir, "nonexistent.claudia"), filepath.Join(dir, "dest.claudia")
			},
			wantErr: "open source",
		},
		{
			name: "returns error when dest dir missing",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				src := filepath.Join(dir, "archive.claudia")
				if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
					t.Fatalf("write source: %v", err)
				}
				return src, filepath.Join(dir, "nonexistent", "dest.claudia")
			},
			wantErr: "create dest",
		},
		{
			name: "returns error when context is cancelled",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				src := filepath.Join(dir, "archive.claudia")
				if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
					t.Fatalf("write source: %v", err)
				}
				return src, filepath.Join(dir, "dest.claudia")
			},
			wantErr: "context canceled",
		},
		{
			name: "returns error when context is cancelled after opening source",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				src := filepath.Join(dir, "archive.claudia")
				if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
					t.Fatalf("write source: %v", err)
				}
				return src, filepath.Join(dir, "dest.claudia")
			},
			customCtx: &cancelOnSecondCallCtx{},
			wantErr:   "context canceled",
		},
		{
			name:       "returns error when os.UserHomeDir fails during expandHome",
			noParallel: true,
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return "~/somefile.claudia", filepath.Join(t.TempDir(), "dest.claudia")
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
				src := filepath.Join(dir, "archive.claudia")
				if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
					t.Fatalf("write source: %v", err)
				}
				return src, filepath.Join(dir, "dest.claudia")
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
				src := filepath.Join(dir, "archive.claudia")
				if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
					t.Fatalf("write source: %v", err)
				}
				return src, filepath.Join(dir, "dest.claudia")
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
				src := filepath.Join(dir, "archive.claudia")
				if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
					t.Fatalf("write source: %v", err)
				}
				return src, filepath.Join(dir, "dest.claudia")
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
				f, err := os.CreateTemp(home, "claudia-test-*.claudia")
				if err != nil {
					t.Skipf("cannot create temp file in home: %v", err)
				}
				_ = f.Close()
				t.Cleanup(func() { _ = os.Remove(f.Name()) })

				if err := os.WriteFile(f.Name(), []byte("home data"), 0o644); err != nil {
					t.Fatalf("write source: %v", err)
				}

				// Build a ~/basename source path.
				rel := "~/" + filepath.Base(f.Name())
				dest := filepath.Join(t.TempDir(), "dest.claudia")
				return rel, dest
			},
			check: func(t *testing.T, dest string) {
				t.Helper()
				data, err := os.ReadFile(dest)
				if err != nil {
					t.Fatalf("read dest: %v", err)
				}
				if string(data) != "home data" {
					t.Errorf("dest content = %q, want %q", string(data), "home data")
				}
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

			if tt.check != nil {
				tt.check(t, dest)
			}
		})
	}
}
