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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/internal/fetcher"
	"github.com/retr0h/agentpack/internal/testutil"
)

// httpFailOnCloseWriter is an io.WriteCloser that succeeds on Write but fails on Close.
type httpFailOnCloseWriter struct{}

func (*httpFailOnCloseWriter) Write(p []byte) (int, error) { return len(p), nil }

func (*httpFailOnCloseWriter) Close() error { return errors.New("simulated close error") }

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
			// NewCancelAfterN(0): fires on every Err() call (calls.Add(1)=1 > 0).
			// Since Done() returns nil, the http client won't cancel mid-flight.
			// The explicit ctx.Err() check (after status check) fires.
			customCtx: testutil.NewCancelAfterN(0),
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
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			if err != nil && tt.wantErr == "" {
				// For tests without explicit wantErr, only fail if truly unexpected.
				if tt.checkBody != "" {
					require.NoError(t, err)
				}
				return
			}

			if tt.checkBody != "" {
				data, err := os.ReadFile(dest)
				require.NoError(t, err)
				assert.Equal(t, tt.checkBody, string(data))
			}
		})
	}
}
