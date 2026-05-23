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
	"strings"
	"testing"

	"github.com/avfs/avfs"
	"github.com/avfs/avfs/vfs/memfs"
	"gopkg.in/yaml.v3"

	"github.com/retr0h/claudia/internal/manifest"
)

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

// memfsWithYAML creates a memfs, writes content as claudia.yaml under /dir,
// and returns the VFS and the directory path "/dir".
func memfsWithYAML(t *testing.T, content string) (avfs.VFS, string) {
	t.Helper()
	vfs := memfs.New()
	if err := vfs.MkdirAll("/dir", 0o755); err != nil {
		t.Fatalf("MkdirAll /dir: %v", err)
	}
	if err := vfs.WriteFile("/dir/claudia.yaml", []byte(content), fs.FileMode(0o644)); err != nil {
		t.Fatalf("WriteFile claudia.yaml: %v", err)
	}
	return vfs, "/dir"
}

// --------------------------------------------------------------------------
// Error-injecting VFS helpers
// --------------------------------------------------------------------------

// readFileErrorVFS wraps avfs.VFS and returns an error from ReadFile for any
// path that exists. Used to trigger the "reading claudia.yaml" error branch.
type readFileErrorVFS struct {
	avfs.VFS
}

func (readFileErrorVFS) ReadFile(string) ([]byte, error) {
	return nil, errors.New("permission denied")
}

// --------------------------------------------------------------------------
// Load
// --------------------------------------------------------------------------

func TestLoad(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tests := []struct {
		name      string
		setupVFS  func(t *testing.T) (avfs.VFS, string)
		wantErr   string // substring that must appear in error; empty = no error
		wantName  string
		wantPlugs int // number of plugins (Plugins field), 0 for single-plugin form
	}{
		{
			name: "single plugin valid",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return memfsWithYAML(t, `
name: my-plugin
version: "1.0.0"
description: A test plugin
`)
			},
			wantName: "my-plugin",
		},
		{
			name: "multi plugin valid",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return memfsWithYAML(t, `
plugins:
  - name: plugin-a
    version: "1.0.0"
    description: First plugin
  - name: plugin-b
    version: "2.0.0"
    description: Second plugin
`)
			},
			wantPlugs: 2,
		},
		{
			name: "missing name single plugin",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return memfsWithYAML(t, `
version: "1.0.0"
description: A test plugin
`)
			},
			wantErr: "name is required",
		},
		{
			name: "missing version single plugin",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return memfsWithYAML(t, `
name: my-plugin
description: A test plugin
`)
			},
			wantErr: "version is required",
		},
		{
			name: "missing description single plugin",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return memfsWithYAML(t, `
name: my-plugin
version: "1.0.0"
`)
			},
			wantErr: "description is required",
		},
		{
			name: "both top-level name and plugins",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return memfsWithYAML(t, `
name: my-plugin
version: "1.0.0"
description: A test plugin
plugins:
  - name: plugin-a
    version: "1.0.0"
    description: First plugin
`)
			},
			wantErr: "manifest has both top-level 'name' and 'plugins'; use one or the other",
		},
		{
			name: "empty plugins list",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return memfsWithYAML(t, "plugins: []\n")
			},
			wantErr: "no plugins defined in claudia.yaml",
		},
		{
			name: "multi plugin missing name",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return memfsWithYAML(t, `
plugins:
  - version: "1.0.0"
    description: First plugin
`)
			},
			wantErr: "name is required",
		},
		{
			name: "multi plugin missing version",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return memfsWithYAML(t, `
plugins:
  - name: plugin-a
    description: First plugin
`)
			},
			wantErr: "version is required",
		},
		{
			name: "multi plugin missing description",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				return memfsWithYAML(t, `
plugins:
  - name: plugin-a
    version: "1.0.0"
`)
			},
			wantErr: "description is required",
		},
		{
			name: "file not found",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				// Return an empty memfs with no claudia.yaml — triggers IsNotExist.
				vfs := memfs.New()
				if err := vfs.MkdirAll("/dir", 0o755); err != nil {
					t.Fatal(err)
				}
				return vfs, "/dir"
			},
			wantErr: "claudia.yaml not found in",
		},
		{
			name: "non-IsNotExist read error triggers reading branch",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				// The readFileErrorVFS always returns "permission denied" which
				// is not an IsNotExist error, so it hits the reading branch.
				return readFileErrorVFS{VFS: memfs.New()}, "/dir"
			},
			wantErr: "reading claudia.yaml",
		},
		{
			name: "malformed YAML returns parse error",
			setupVFS: func(t *testing.T) (avfs.VFS, string) {
				t.Helper()
				// Write invalid YAML to trigger the yaml.Unmarshal error branch.
				return memfsWithYAML(t, "name: [invalid yaml\n")
			},
			wantErr: "parsing claudia.yaml",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			vfs, dir := tc.setupVFS(t)
			m, err := manifest.Load(ctx, vfs, dir)

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if got := err.Error(); !strings.Contains(got, tc.wantErr) {
					t.Fatalf("error %q does not contain %q", got, tc.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tc.wantName != "" && m.Name != tc.wantName {
				t.Errorf("Name = %q, want %q", m.Name, tc.wantName)
			}
			if tc.wantPlugs > 0 && len(m.Plugins) != tc.wantPlugs {
				t.Errorf("len(Plugins) = %d, want %d", len(m.Plugins), tc.wantPlugs)
			}
		})
	}
}

// --------------------------------------------------------------------------
// Normalize
// --------------------------------------------------------------------------

func TestNormalize(t *testing.T) {
	t.Parallel()

	sharedAuthor := manifest.Author{Name: "Shared Author", Email: "shared@example.com"}

	tests := []struct {
		name    string
		input   *manifest.Manifest
		wantLen int
		check   func(t *testing.T, plugins []manifest.Plugin)
	}{
		{
			name: "single plugin becomes one-element slice",
			input: &manifest.Manifest{
				Name:        "solo",
				Version:     "1.0.0",
				Description: "solo desc",
				Author:      sharedAuthor,
				License:     "MIT",
				Homepage:    "https://example.com",
				Keywords:    []string{"foo"},
				Category:    "tools",
			},
			wantLen: 1,
			check: func(t *testing.T, plugins []manifest.Plugin) {
				t.Helper()
				p := plugins[0]
				if p.Name != "solo" {
					t.Errorf("Name = %q, want %q", p.Name, "solo")
				}
				if p.Version != "1.0.0" {
					t.Errorf("Version = %q, want %q", p.Version, "1.0.0")
				}
				if p.Author != sharedAuthor {
					t.Errorf("Author = %+v, want %+v", p.Author, sharedAuthor)
				}
				if p.License != "MIT" {
					t.Errorf("License = %q, want %q", p.License, "MIT")
				}
				if p.Homepage != "https://example.com" {
					t.Errorf("Homepage = %q, want %q", p.Homepage, "https://example.com")
				}
				if len(p.Keywords) != 1 || p.Keywords[0] != "foo" {
					t.Errorf("Keywords = %v, want [foo]", p.Keywords)
				}
				if p.Category != "tools" {
					t.Errorf("Category = %q, want %q", p.Category, "tools")
				}
			},
		},
		{
			name: "multi plugin preserves all entries",
			input: &manifest.Manifest{
				Plugins: []manifest.Plugin{
					{Name: "a", Version: "1.0.0", Description: "first"},
					{Name: "b", Version: "2.0.0", Description: "second"},
				},
			},
			wantLen: 2,
			check: func(t *testing.T, plugins []manifest.Plugin) {
				t.Helper()
				if plugins[0].Name != "a" {
					t.Errorf("plugins[0].Name = %q, want %q", plugins[0].Name, "a")
				}
				if plugins[1].Name != "b" {
					t.Errorf("plugins[1].Name = %q, want %q", plugins[1].Name, "b")
				}
			},
		},
		{
			name: "shared field inheritance from top-level",
			input: &manifest.Manifest{
				Author:   sharedAuthor,
				License:  "Apache-2.0",
				Homepage: "https://shared.example.com",
				Plugins: []manifest.Plugin{
					{Name: "a", Version: "1.0.0", Description: "first"},
				},
			},
			wantLen: 1,
			check: func(t *testing.T, plugins []manifest.Plugin) {
				t.Helper()
				p := plugins[0]
				if p.Author != sharedAuthor {
					t.Errorf("Author = %+v, want %+v", p.Author, sharedAuthor)
				}
				if p.License != "Apache-2.0" {
					t.Errorf("License = %q, want %q", p.License, "Apache-2.0")
				}
				if p.Homepage != "https://shared.example.com" {
					t.Errorf("Homepage = %q, want %q", p.Homepage, "https://shared.example.com")
				}
			},
		},
		{
			name: "plugin overrides shared fields",
			input: &manifest.Manifest{
				Author:   sharedAuthor,
				License:  "Apache-2.0",
				Homepage: "https://shared.example.com",
				Plugins: []manifest.Plugin{
					{
						Name:        "a",
						Version:     "1.0.0",
						Description: "first",
						Author:      manifest.Author{Name: "Override Author", Email: "override@example.com"},
						License:     "GPL-3.0",
						Homepage:    "https://override.example.com",
					},
				},
			},
			wantLen: 1,
			check: func(t *testing.T, plugins []manifest.Plugin) {
				t.Helper()
				p := plugins[0]
				wantAuthor := manifest.Author{Name: "Override Author", Email: "override@example.com"}
				if p.Author != wantAuthor {
					t.Errorf("Author = %+v, want %+v", p.Author, wantAuthor)
				}
				if p.License != "GPL-3.0" {
					t.Errorf("License = %q, want %q", p.License, "GPL-3.0")
				}
				if p.Homepage != "https://override.example.com" {
					t.Errorf("Homepage = %q, want %q", p.Homepage, "https://override.example.com")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := manifest.Normalize(tc.input)

			if len(got) != tc.wantLen {
				t.Fatalf("len(Normalize(...)) = %d, want %d", len(got), tc.wantLen)
			}

			if tc.check != nil {
				tc.check(t, got)
			}
		})
	}
}

// --------------------------------------------------------------------------
// Entry.UnmarshalYAML
// --------------------------------------------------------------------------

func TestEntryUnmarshalYAML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		wantGlob string
		wantSrc  string
		wantDest string
	}{
		{
			name:     "bare string becomes glob",
			input:    `"skills/*.md"`,
			wantGlob: "skills/*.md",
		},
		{
			name:     "object with src and dest",
			input:    `{src: "README.md", dest: "docs/README.md"}`,
			wantSrc:  "README.md",
			wantDest: "docs/README.md",
		},
		{
			name:    "object with only src",
			input:   `{src: "hook.sh"}`,
			wantSrc: "hook.sh",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var e manifest.Entry
			if err := yaml.Unmarshal([]byte(tc.input), &e); err != nil {
				t.Fatalf("UnmarshalYAML error: %v", err)
			}

			if e.Glob != tc.wantGlob {
				t.Errorf("Glob = %q, want %q", e.Glob, tc.wantGlob)
			}
			if e.Src != tc.wantSrc {
				t.Errorf("Src = %q, want %q", e.Src, tc.wantSrc)
			}
			if e.Dest != tc.wantDest {
				t.Errorf("Dest = %q, want %q", e.Dest, tc.wantDest)
			}
		})
	}
}
