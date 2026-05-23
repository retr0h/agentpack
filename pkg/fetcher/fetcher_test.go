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
	"fmt"
	"strings"
	"testing"

	"github.com/retr0h/claudia/pkg/fetcher"
)

func TestNew(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		source   string
		wantType string
		wantErr  string
	}{
		{
			name:     "absolute path returns FileFetcher",
			source:   "/tmp/archive.claudia",
			wantType: "*fetcher.FileFetcher",
		},
		{
			name:     "relative path returns FileFetcher",
			source:   "./archive.claudia",
			wantType: "*fetcher.FileFetcher",
		},
		{
			name:     "home-relative path returns FileFetcher",
			source:   "~/downloads/archive.claudia",
			wantType: "*fetcher.FileFetcher",
		},
		{
			name:     "bare filename returns FileFetcher",
			source:   "archive.claudia",
			wantType: "*fetcher.FileFetcher",
		},
		{
			name:     "http URL returns HTTPFetcher",
			source:   "http://example.com/archive.claudia",
			wantType: "*fetcher.HTTPFetcher",
		},
		{
			name:     "https URL returns HTTPFetcher",
			source:   "https://example.com/archive.claudia",
			wantType: "*fetcher.HTTPFetcher",
		},
		{
			name:    "s3 scheme returns error",
			source:  "s3://bucket/archive.claudia",
			wantErr: "s3 backend not yet implemented",
		},
		{
			name:    "gs scheme returns error",
			source:  "gs://bucket/archive.claudia",
			wantErr: "gs backend not yet implemented",
		},
		{
			name:    "unknown scheme returns error",
			source:  "ftp://example.com/archive.claudia",
			wantErr: "unknown scheme",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f, err := fetcher.New(tt.source)

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

			if f == nil {
				t.Fatal("expected non-nil Fetcher")
			}

			// Use fmt.Sprintf to get the type name for comparison.
			got := fmt.Sprintf("%T", f)
			if got != tt.wantType {
				t.Errorf("fetcher type = %q, want %q", got, tt.wantType)
			}
		})
	}
}
