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

package registry_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/retr0h/agentpack/pkg/registry"
)

// tempHome returns a SetOsUserHomeDir restore function that directs all
// registry I/O to a fresh temp directory, preventing real-home pollution.
func tempHome(t *testing.T) func() {
	t.Helper()

	tmp := t.TempDir()

	return registry.SetOsUserHomeDir(func() (string, error) {
		return tmp, nil
	})
}

// --------------------------------------------------------------------------
// TestSaveAndLoad
// --------------------------------------------------------------------------

func TestSaveAndLoad(t *testing.T) {
	tests := []struct {
		name     string
		manifest *registry.PackageManifest
		wantErr  string
	}{
		{
			name: "save and load round-trip",
			manifest: &registry.PackageManifest{
				Name:    "my-plugin",
				Source:  "github.com/org/repo",
				Version: "1.0.0",
				Files: []registry.InstalledFile{
					{Path: "skills/foo.md", SHA256: "abc123", Target: "claude-code", Dir: "/tmp/dir"},
				},
			},
		},
		{
			name: "manifest with no files",
			manifest: &registry.PackageManifest{
				Name:   "empty-plugin",
				Source: "github.com/org/empty",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restore := tempHome(t)
			defer restore()

			err := registry.Save(tt.manifest)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("Save: %v", err)
			}

			got, loadErr := registry.Load(tt.manifest.Name)
			if loadErr != nil {
				t.Fatalf("Load: %v", loadErr)
			}

			if got.Name != tt.manifest.Name {
				t.Errorf("Name = %q, want %q", got.Name, tt.manifest.Name)
			}

			if got.Source != tt.manifest.Source {
				t.Errorf("Source = %q, want %q", got.Source, tt.manifest.Source)
			}

			if len(got.Files) != len(tt.manifest.Files) {
				t.Errorf("Files len = %d, want %d", len(got.Files), len(tt.manifest.Files))
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestDir
// --------------------------------------------------------------------------

func TestDir(t *testing.T) {
	tests := []struct {
		name       string
		homeFunc   func() (string, error)
		wantSuffix string
		wantErr    string
	}{
		{
			name: "returns registry dir under temp home",
			homeFunc: func() (string, error) {
				return t.TempDir(), nil
			},
			wantSuffix: filepath.Join(".config", "agentpack", "packages"),
		},
		{
			name: "returns error when home dir lookup fails",
			homeFunc: func() (string, error) {
				return "", fmt.Errorf("no home")
			},
			wantErr: "home dir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restore := registry.SetOsUserHomeDir(tt.homeFunc)
			defer restore()

			got, err := registry.Dir()
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !filepath.IsAbs(got) {
				t.Errorf("Dir() = %q, want absolute path", got)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestLoad
// --------------------------------------------------------------------------

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		pkgName string
		wantErr string
	}{
		{
			name:    "nonexistent package returns error",
			pkgName: "does-not-exist",
			wantErr: "not found in registry",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restore := tempHome(t)
			defer restore()

			_, err := registry.Load(tt.pkgName)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestRemove
// --------------------------------------------------------------------------

func TestRemove(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) string // returns pkg name to remove
		wantErr string
	}{
		{
			name: "remove existing manifest",
			setup: func(t *testing.T) string {
				t.Helper()
				m := &registry.PackageManifest{Name: "to-remove", Source: "src"}
				if err := registry.Save(m); err != nil {
					t.Fatalf("setup Save: %v", err)
				}

				return "to-remove"
			},
		},
		{
			name: "remove nonexistent is no-op",
			setup: func(t *testing.T) string {
				t.Helper()

				return "ghost"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restore := tempHome(t)
			defer restore()

			pkgName := tt.setup(t)

			err := registry.Remove(pkgName)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify it's gone.
			dir, _ := registry.Dir()
			_, statErr := os.Stat(filepath.Join(dir, pkgName+".yaml"))
			if !os.IsNotExist(statErr) {
				t.Error("manifest file still exists after Remove")
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestList
// --------------------------------------------------------------------------

func TestList(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T)
		wantLen int
	}{
		{
			name:    "empty registry returns nil",
			setup:   func(t *testing.T) { t.Helper() },
			wantLen: 0,
		},
		{
			name: "lists all saved manifests",
			setup: func(t *testing.T) {
				t.Helper()

				for _, n := range []string{"alpha", "beta", "gamma"} {
					if err := registry.Save(&registry.PackageManifest{Name: n, Source: "src"}); err != nil {
						t.Fatalf("setup Save %s: %v", n, err)
					}
				}
			},
			wantLen: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restore := tempHome(t)
			defer restore()

			tt.setup(t)

			got, err := registry.List()
			if err != nil {
				t.Fatalf("List: %v", err)
			}

			if len(got) != tt.wantLen {
				t.Errorf("len = %d, want %d", len(got), tt.wantLen)
			}
		})
	}
}
