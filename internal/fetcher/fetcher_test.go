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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/internal/fetcher"
)

// --------------------------------------------------------------------------
// TestExpandShorthand
// --------------------------------------------------------------------------

func TestExpandShorthand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "bare owner/repo expands to github.com",
			input: "jeffallan/claude-skills",
			want:  "github.com/jeffallan/claude-skills",
		},
		{
			name:  "owner/repo with at-skill suffix expands and preserves suffix",
			input: "owner/repo@skill",
			want:  "github.com/owner/repo@skill",
		},
		{
			name:  "owner/repo with hash-ref suffix expands and preserves suffix",
			input: "owner/repo#v1.0",
			want:  "github.com/owner/repo#v1.0",
		},
		{
			name:  "already qualified github.com path is unchanged",
			input: "github.com/owner/repo",
			want:  "github.com/owner/repo",
		},
		{
			name:  "https scheme is unchanged",
			input: "https://github.com/owner/repo",
			want:  "https://github.com/owner/repo",
		},
		{
			name:  "absolute path is unchanged",
			input: "/local/path",
			want:  "/local/path",
		},
		{
			name:  "relative path is unchanged",
			input: "./relative",
			want:  "./relative",
		},
		{
			name:  "home-relative path is unchanged",
			input: "~/home/path",
			want:  "~/home/path",
		},
		{
			name:  "bare name without slash is unchanged",
			input: "just-a-name",
			want:  "just-a-name",
		},
		{
			name:  "three-segment path is unchanged",
			input: "a/b/c",
			want:  "a/b/c",
		},
		{
			name:  "empty string is unchanged",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := fetcher.ExpandShorthand(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

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
			source:   "/tmp/archive.agentpack",
			wantType: "*fetcher.FileFetcher",
		},
		{
			name:     "relative path returns FileFetcher",
			source:   "./archive.agentpack",
			wantType: "*fetcher.FileFetcher",
		},
		{
			name:     "home-relative path returns FileFetcher",
			source:   "~/downloads/archive.agentpack",
			wantType: "*fetcher.FileFetcher",
		},
		{
			name:     "bare filename returns FileFetcher",
			source:   "archive.agentpack",
			wantType: "*fetcher.FileFetcher",
		},
		{
			name:     "github.com host returns GitFetcher",
			source:   "github.com/org/repo",
			wantType: "*fetcher.GitFetcher",
		},
		{
			name:     "gitlab.com host returns GitFetcher",
			source:   "gitlab.com/org/repo",
			wantType: "*fetcher.GitFetcher",
		},
		{
			name:     "bitbucket.org host returns GitFetcher",
			source:   "bitbucket.org/org/repo",
			wantType: "*fetcher.GitFetcher",
		},
		{
			name:     "source ending in .git returns GitFetcher",
			source:   "https://internal.example.com/repo.git",
			wantType: "*fetcher.GitFetcher",
		},
		{
			name:     "github.com with ref returns GitFetcher",
			source:   "github.com/org/repo#v1.0.0",
			wantType: "*fetcher.GitFetcher",
		},
		{
			name:     "http URL returns HTTPFetcher",
			source:   "http://example.com/archive.agentpack",
			wantType: "*fetcher.HTTPFetcher",
		},
		{
			name:     "https URL returns HTTPFetcher",
			source:   "https://example.com/archive.agentpack",
			wantType: "*fetcher.HTTPFetcher",
		},
		{
			name:     "https github.com URL returns GitFetcher",
			source:   "https://github.com/org/repo",
			wantType: "*fetcher.GitFetcher",
		},
		{
			name:     "https gitlab.com URL returns GitFetcher",
			source:   "https://gitlab.com/org/repo",
			wantType: "*fetcher.GitFetcher",
		},
		{
			name:    "s3 scheme returns error",
			source:  "s3://bucket/archive.agentpack",
			wantErr: "s3 backend not yet implemented",
		},
		{
			name:    "gs scheme returns error",
			source:  "gs://bucket/archive.agentpack",
			wantErr: "gs backend not yet implemented",
		},
		{
			name:    "unknown scheme returns error",
			source:  "ftp://example.com/archive.agentpack",
			wantErr: "unknown scheme",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f, err := fetcher.New(tt.source)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, f)

			// Use fmt.Sprintf to get the type name for comparison.
			got := fmt.Sprintf("%T", f)
			assert.Equal(t, tt.wantType, got)
		})
	}
}
