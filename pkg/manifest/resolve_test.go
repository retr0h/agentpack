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
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/avfs/avfs"
	"github.com/avfs/avfs/vfs/memfs"

	"github.com/retr0h/claudia/pkg/manifest"
)

// --------------------------------------------------------------------------
// Error-injecting VFS helpers for resolve tests
// --------------------------------------------------------------------------

// statErrorVFS wraps avfs.VFS and injects an error from Stat that is NOT
// an IsNotExist error.
type statErrorVFS struct {
	avfs.VFS
}

func (statErrorVFS) Stat(string) (fs.FileInfo, error) {
	return nil, errors.New("permission denied: stat failed")
}

// relErrorVFS wraps avfs.VFS and injects an error from Rel, exercising the
// error branch in resolveGlob after a successful Glob match.
type relErrorVFS struct {
	avfs.VFS
}

func (relErrorVFS) Rel(_, _ string) (string, error) {
	return "", errors.New("simulated rel error")
}

// makeFixtures creates a memfs with the given relative file paths (written
// with empty content) rooted at /base and returns the VFS and base path.
func makeFixtures(t *testing.T, files []string) (avfs.VFS, string) {
	t.Helper()
	vfs := memfs.New()
	base := "/base"
	if err := vfs.MkdirAll(base, 0o755); err != nil {
		t.Fatalf("makeFixtures mkdir base: %v", err)
	}
	for _, f := range files {
		full := filepath.Join(base, f)
		if err := vfs.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("makeFixtures mkdir: %v", err)
		}
		if err := vfs.WriteFile(full, []byte(""), fs.FileMode(0o644)); err != nil {
			t.Fatalf("makeFixtures write: %v", err)
		}
	}
	return vfs, base
}

// --------------------------------------------------------------------------
// ResolveEntries
// --------------------------------------------------------------------------

func TestResolveEntries(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tests := []struct {
		name      string
		setup     func(t *testing.T) (avfs.VFS, string)
		entries   []manifest.Entry
		wantN     int
		wantDests []string // if set, verify destination paths
		wantErr   string
		exactErr  bool
	}{
		{
			name: "glob matches two files",
			setup: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return makeFixtures(t, []string{"skills/a.md", "skills/b.md"})
			},
			entries: []manifest.Entry{
				{Glob: "skills/*.md"},
			},
			wantN: 2,
		},
		{
			name: "glob matches no files returns error",
			setup: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return makeFixtures(t, []string{})
			},
			entries: []manifest.Entry{
				{Glob: "skills/*.md"},
			},
			wantErr:  "pattern 'skills/*.md' matched no files",
			exactErr: true,
		},
		{
			name: "src with empty dest uses filename",
			setup: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return makeFixtures(t, []string{"prompts/hello.md"})
			},
			entries: []manifest.Entry{
				{Src: "prompts/hello.md", Dest: ""},
			},
			wantN: 1,
		},
		{
			name: "src/dest with directory dest",
			setup: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return makeFixtures(t, []string{"prompts/hello.md"})
			},
			entries: []manifest.Entry{
				{Src: "prompts/hello.md", Dest: "skills/"},
			},
			wantN: 1,
		},
		{
			name: "src/dest with file dest rename",
			setup: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return makeFixtures(t, []string{"prompts/review.md"})
			},
			entries: []manifest.Entry{
				{Src: "prompts/review.md", Dest: "skills/renamed.md"},
			},
			wantN: 1,
		},
		{
			name: "src/dest with glob src",
			setup: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return makeFixtures(t, []string{"prompts/a.md", "prompts/b.md"})
			},
			entries: []manifest.Entry{
				{Src: "prompts/*.md", Dest: "skills/"},
			},
			wantN: 2,
		},
		{
			name: "src file not found returns error",
			setup: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return makeFixtures(t, []string{})
			},
			entries: []manifest.Entry{
				{Src: "prompts/missing.md", Dest: "skills/missing.md"},
			},
			wantErr:  "src file not found: prompts/missing.md",
			exactErr: true,
		},
		{
			name: "empty entries returns empty slice",
			setup: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return makeFixtures(t, []string{})
			},
			entries: []manifest.Entry{},
			wantN:   0,
		},
		{
			name: "nil entries returns empty slice",
			setup: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return makeFixtures(t, []string{})
			},
			entries: nil,
			wantN:   0,
		},
		{
			name: "entry with neither glob nor src returns error",
			setup: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return makeFixtures(t, []string{})
			},
			entries: []manifest.Entry{
				{},
			},
			wantErr:  "entry has neither glob nor src",
			exactErr: true,
		},
		{
			name: "non-IsNotExist stat error in resolveSrcDest",
			setup: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				vfs := statErrorVFS{VFS: memfs.New()}
				return vfs, "/base"
			},
			entries: []manifest.Entry{
				{Src: "somefile.txt", Dest: "dest.txt"},
			},
			wantErr: "stat",
		},
		{
			name: "bad glob pattern in resolveGlob",
			setup: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				// memfs.Glob returns a syntax error for an unclosed bracket.
				return makeFixtures(t, []string{})
			},
			entries: []manifest.Entry{
				{Glob: "["}, // unclosed bracket — Glob returns syntax error
			},
			wantErr: "invalid glob pattern",
		},
		{
			name: "bad glob src pattern in resolveSrcDest",
			setup: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return makeFixtures(t, []string{})
			},
			entries: []manifest.Entry{
				{Src: "[", Dest: "out/"},
			},
			wantErr: "invalid glob pattern",
		},
		{
			name: "glob src matches no files returns error",
			setup: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return makeFixtures(t, []string{})
			},
			entries: []manifest.Entry{
				{Src: "nonexistent/*.md", Dest: "skills/"},
			},
			wantErr:  "pattern 'nonexistent/*.md' matched no files",
			exactErr: true,
		},
		{
			name: "returns error when vfs.Rel fails in resolveGlob",
			setup: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				// Use relErrorVFS so that Glob succeeds (finds files) but Rel
				// returns an error.
				base := memfs.New()
				if err := base.MkdirAll("/base/skills", 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := base.WriteFile(
					"/base/skills/intro.md", []byte(""), fs.FileMode(0o644),
				); err != nil {
					t.Fatalf("write: %v", err)
				}
				return relErrorVFS{VFS: base}, "/base"
			},
			entries: []manifest.Entry{
				{Glob: "skills/*.md"},
			},
			wantErr: "computing relative path",
		},
		{
			name: "glob preserves relative path in dest",
			setup: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return makeFixtures(t, []string{"skills/intro.md", "skills/review.md"})
			},
			entries: []manifest.Entry{
				{Glob: "skills/*.md"},
			},
			wantN:     2,
			wantDests: []string{"skills/intro.md", "skills/review.md"},
		},
		{
			name: "src/dest dir preserves filename in dest",
			setup: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return makeFixtures(t, []string{"prompts/hello.md"})
			},
			entries: []manifest.Entry{
				{Src: "prompts/hello.md", Dest: "skills/"},
			},
			wantN:     1,
			wantDests: []string{"skills/hello.md"},
		},
		{
			name: "src/dest file renames in dest",
			setup: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return makeFixtures(t, []string{"prompts/review.md"})
			},
			entries: []manifest.Entry{
				{Src: "prompts/review.md", Dest: "skills/renamed.md"},
			},
			wantN:     1,
			wantDests: []string{"skills/renamed.md"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			vfs, base := tc.setup(t)
			got, err := manifest.ResolveEntries(ctx, vfs, base, tc.entries)

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tc.wantErr)
				}
				if tc.exactErr {
					if err.Error() != tc.wantErr {
						t.Fatalf("error = %q, want %q", err.Error(), tc.wantErr)
					}
				} else {
					if !strings.Contains(err.Error(), tc.wantErr) {
						t.Fatalf("error = %q, want it to contain %q", err.Error(), tc.wantErr)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tc.wantN {
				t.Fatalf("len(pairs) = %d, want %d", len(got), tc.wantN)
			}

			if len(tc.wantDests) > 0 {
				actual := make(map[string]bool, len(got))
				for _, fp := range got {
					actual[fp.Dest] = true
				}
				for _, want := range tc.wantDests {
					if !actual[want] {
						t.Errorf("missing dest %q in results %v", want, got)
					}
				}
				for _, fp := range got {
					if !filepath.IsAbs(fp.Src) {
						t.Errorf("Src %q is not absolute", fp.Src)
					}
				}
			}
		})
	}
}
