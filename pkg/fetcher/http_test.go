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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/retr0h/claudia/pkg/fetcher"
)

func TestHTTPFetcherFetch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		handler   http.HandlerFunc
		setupDest func(t *testing.T) string
		cancelCtx bool
		wantErr   string
		checkBody string
	}{
		{
			name: "downloads 200 OK response to dest",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte("archive content"))
			},
			setupDest: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "out.claudia")
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
				return filepath.Join(t.TempDir(), "out.claudia")
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
				return filepath.Join(t.TempDir(), "out.claudia")
			},
			cancelCtx: true,
			wantErr:   "context canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(tt.handler)
			defer srv.Close()

			dest := tt.setupDest(t)

			var ctx context.Context
			var cancel context.CancelFunc

			if tt.cancelCtx {
				ctx, cancel = context.WithCancel(context.Background())
				cancel()
			} else {
				ctx = context.Background()
				cancel = func() {}
			}
			defer cancel()

			f := &fetcher.HTTPFetcher{}
			err := f.Fetch(ctx, srv.URL, dest)

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
