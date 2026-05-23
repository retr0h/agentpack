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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/retr0h/claudia/pkg/fetcher"
)

func TestFileFetcherFetch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (source string, dest string)
		wantErr string
		check   func(t *testing.T, dest string)
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			source, dest := tt.setup(t)

			var ctx context.Context
			var cancel context.CancelFunc

			if strings.Contains(tt.wantErr, "context canceled") {
				ctx, cancel = context.WithCancel(context.Background())
				cancel()
			} else {
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
