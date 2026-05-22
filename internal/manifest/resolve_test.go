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

package manifest_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/retr0h/claudia/internal/manifest"
)

// makeFixtures creates a temporary directory tree with the given relative file
// paths (all written with empty content) and returns the base directory.
func makeFixtures(t *testing.T, files []string) string {
	t.Helper()
	base := t.TempDir()
	for _, f := range files {
		full := filepath.Join(base, f)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("makeFixtures mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(""), 0o644); err != nil {
			t.Fatalf("makeFixtures write: %v", err)
		}
	}
	return base
}

// --------------------------------------------------------------------------
// ResolveEntries
// --------------------------------------------------------------------------

func TestResolveEntries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		files   []string         // fixture files to create relative to baseDir
		entries []manifest.Entry // input entries
		wantN   int              // expected number of FilePairs on success
		wantErr string           // non-empty means we expect an error containing this
	}{
		{
			name:  "glob matches two files",
			files: []string{"skills/a.md", "skills/b.md"},
			entries: []manifest.Entry{
				{Glob: "skills/*.md"},
			},
			wantN: 2,
		},
		{
			name:  "glob matches no files returns error",
			files: []string{},
			entries: []manifest.Entry{
				{Glob: "skills/*.md"},
			},
			wantErr: "pattern 'skills/*.md' matched no files",
		},
		{
			name:  "src/dest with directory dest",
			files: []string{"prompts/hello.md"},
			entries: []manifest.Entry{
				{Src: "prompts/hello.md", Dest: "skills/"},
			},
			wantN: 1,
		},
		{
			name:  "src/dest with file dest rename",
			files: []string{"prompts/review.md"},
			entries: []manifest.Entry{
				{Src: "prompts/review.md", Dest: "skills/renamed.md"},
			},
			wantN: 1,
		},
		{
			name:  "src/dest with glob src",
			files: []string{"prompts/a.md", "prompts/b.md"},
			entries: []manifest.Entry{
				{Src: "prompts/*.md", Dest: "skills/"},
			},
			wantN: 2,
		},
		{
			name:  "src file not found returns error",
			files: []string{},
			entries: []manifest.Entry{
				{Src: "prompts/missing.md", Dest: "skills/missing.md"},
			},
			wantErr: "src file not found: prompts/missing.md",
		},
		{
			name:    "empty entries returns empty slice",
			files:   []string{},
			entries: []manifest.Entry{},
			wantN:   0,
		},
		{
			name:    "nil entries returns empty slice",
			files:   []string{},
			entries: nil,
			wantN:   0,
		},
		{
			name:  "entry with neither glob nor src returns error",
			files: []string{},
			entries: []manifest.Entry{
				{},
			},
			wantErr: "entry has neither glob nor src",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			base := makeFixtures(t, tc.files)
			got, err := manifest.ResolveEntries(base, tc.entries)

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tc.wantErr)
				}
				if msg := err.Error(); msg != tc.wantErr {
					t.Fatalf("error = %q, want %q", msg, tc.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tc.wantN {
				t.Fatalf("len(pairs) = %d, want %d", len(got), tc.wantN)
			}
		})
	}
}

// --------------------------------------------------------------------------
// ResolveEntries — destination path shape
// --------------------------------------------------------------------------

func TestResolveEntriesDestPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		files     []string
		entries   []manifest.Entry
		wantDests []string // expected Dest values, in any order
	}{
		{
			name:  "glob preserves relative path",
			files: []string{"skills/intro.md", "skills/review.md"},
			entries: []manifest.Entry{
				{Glob: "skills/*.md"},
			},
			wantDests: []string{"skills/intro.md", "skills/review.md"},
		},
		{
			name:  "src/dest dir preserves filename",
			files: []string{"prompts/hello.md"},
			entries: []manifest.Entry{
				{Src: "prompts/hello.md", Dest: "skills/"},
			},
			wantDests: []string{"skills/hello.md"},
		},
		{
			name:  "src/dest file renames destination",
			files: []string{"prompts/review.md"},
			entries: []manifest.Entry{
				{Src: "prompts/review.md", Dest: "skills/renamed.md"},
			},
			wantDests: []string{"skills/renamed.md"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			base := makeFixtures(t, tc.files)
			got, err := manifest.ResolveEntries(base, tc.entries)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(got) != len(tc.wantDests) {
				t.Fatalf("len(pairs) = %d, want %d", len(got), len(tc.wantDests))
			}

			// Build a set of actual dest values for order-independent comparison.
			actual := make(map[string]bool, len(got))
			for _, fp := range got {
				actual[fp.Dest] = true
			}
			for _, want := range tc.wantDests {
				if !actual[want] {
					t.Errorf("missing dest %q in results %v", want, got)
				}
			}

			// Every Src must be an absolute path.
			for _, fp := range got {
				if !filepath.IsAbs(fp.Src) {
					t.Errorf("Src %q is not absolute", fp.Src)
				}
			}
		})
	}
}
