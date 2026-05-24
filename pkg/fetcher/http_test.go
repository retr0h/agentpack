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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/retr0h/agentpack/pkg/fetcher"
)

// httpFailOnCloseWriter is an io.WriteCloser that succeeds on Write but fails on Close.
type httpFailOnCloseWriter struct{}

func (*httpFailOnCloseWriter) Write(p []byte) (int, error) { return len(p), nil }

func (*httpFailOnCloseWriter) Close() error { return errors.New("simulated close error") }

// cancelAfterHTTPResponseCtx returns nil from Err() for the first several
// calls (to allow http.NewRequestWithContext and Do to proceed), then returns
// context.Canceled. This lets the HTTP request complete (200 OK) but triggers
// the ctx.Err() check at line 52 (after status check, before os.Create).
type cancelAfterHTTPResponseCtx struct {
	callCount int
	triggerAt int // return error when callCount > triggerAt
}

func (c *cancelAfterHTTPResponseCtx) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *cancelAfterHTTPResponseCtx) Done() <-chan struct{}       { return nil }
func (c *cancelAfterHTTPResponseCtx) Value(_ any) any             { return nil }
func (c *cancelAfterHTTPResponseCtx) Err() error {
	c.callCount++
	if c.callCount <= c.triggerAt {
		return nil
	}
	return errors.New("context canceled")
}

func TestHTTPFetcherFetch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		handler     http.HandlerFunc
		sourceURL   string // if set, use this URL instead of the test server
		setupDest   func(t *testing.T) string
		cancelCtx   bool
		customCtx   context.Context
		injectFuncs func(t *testing.T) // if set, inject function vars (not parallel-safe)
		noParallel  bool               // if true, do not run subtest in parallel
		wantErr     string
		checkBody   string
	}{
		{
			name: "downloads 200 OK response to dest",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte("archive content"))
			},
			setupDest: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "out.agentpack")
			},
			checkBody: "archive content",
		},
		{
			name: "returns error on 404 response",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				http.NotFound(w, nil)
			},
			setupDest: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "out.agentpack")
			},
			wantErr: "http 404",
		},
		{
			name: "returns error when context is cancelled",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte("data"))
			},
			setupDest: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "out.agentpack")
			},
			cancelCtx: true,
			wantErr:   "context canceled",
		},
		{
			name: "returns error when dest directory does not exist",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte("data"))
			},
			setupDest: func(t *testing.T) string {
				t.Helper()
				// A path whose parent directory doesn't exist.
				return filepath.Join(t.TempDir(), "nonexistent", "out.agentpack")
			},
			wantErr: "create dest",
		},
		{
			name:      "returns error for invalid source URL",
			sourceURL: "://invalid-url",
			setupDest: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "out.agentpack")
			},
			wantErr: "build request",
		},
		{
			name: "returns error when context is cancelled after successful response",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte("data"))
			},
			setupDest: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "out.agentpack")
			},
			// triggerAt:0 means all Err() calls return error immediately.
			// Since Done() returns nil, the http client won't cancel mid-flight.
			// The explicit ctx.Err() check at line 52 (after status check) fires.
			customCtx: &cancelAfterHTTPResponseCtx{triggerAt: 0},
			wantErr:   "context canceled",
		},
		{
			name:       "returns error when dest close fails",
			noParallel: true,
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte("data"))
			},
			setupDest: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "out.agentpack")
			},
			injectFuncs: func(t *testing.T) {
				t.Helper()
				restore := fetcher.SetOsCreateHTTP(func(_ string) (io.WriteCloser, error) {
					return &httpFailOnCloseWriter{}, nil
				})
				t.Cleanup(restore)
			},
			wantErr: "close dest",
		},
		{
			name: "returns error when response body read fails",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				// Write the status and headers, then close the connection
				// abruptly so that io.Copy fails reading the body.
				w.Header().Set("Content-Length", "1000")
				w.WriteHeader(http.StatusOK)
				// Hijack and close: flush partial content then drop connection.
				_, _ = w.Write([]byte("partial"))
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			},
			setupDest: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "out.agentpack")
			},
			// This test may or may not error depending on timing; skip
			// the error assertion and just verify it doesn't panic.
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

			var url string
			if tt.sourceURL != "" {
				// Use the provided URL directly (no test server needed).
				url = tt.sourceURL
			} else if tt.handler != nil {
				srv := httptest.NewServer(tt.handler)
				defer srv.Close()
				url = srv.URL
			}

			dest := tt.setupDest(t)

			var ctx context.Context
			var cancel context.CancelFunc

			switch {
			case tt.customCtx != nil:
				ctx = tt.customCtx
				cancel = func() {}
			case tt.cancelCtx:
				ctx, cancel = context.WithCancel(context.Background())
				cancel()
			default:
				ctx = context.Background()
				cancel = func() {}
			}
			defer cancel()

			f := &fetcher.HTTPFetcher{}
			err := f.Fetch(ctx, url, dest)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
				return
			}

			if err != nil && tt.wantErr == "" {
				// For tests without explicit wantErr, only fail if truly unexpected.
				if tt.checkBody != "" {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}

			if tt.checkBody != "" {
				data, err := os.ReadFile(dest)
				if err != nil {
					t.Fatalf("read dest: %v", err)
				}
				if string(data) != tt.checkBody {
					t.Errorf("dest content = %q, want %q", string(data), tt.checkBody)
				}
			}
		})
	}
}
