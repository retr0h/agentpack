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

// NOTE: registry tests deliberately do NOT call t.Parallel() at the top
// level because they all mutate the package-level osUserHomeDir variable via
// SetOsUserHomeDir. Running them in parallel would cause a data race between
// subtests. Sub-tests within a single table function are also sequential for
// the same reason.
//

package registry_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/pkg/registry"
)

// tempHome returns a SetOsUserHomeDir restore function that directs all
// registry I/O to a fresh temp directory, preventing real-home pollution.
func tempHome(t *testing.T) (string, func()) {
	t.Helper()

	tmp := t.TempDir()
	restore := registry.SetOsUserHomeDir(func() (string, error) {
		return tmp, nil
	})

	return tmp, restore
}

// --------------------------------------------------------------------------
// TestDir
// --------------------------------------------------------------------------

func TestDir(t *testing.T) {
	tests := []struct {
		name       string
		homeFunc   func(t *testing.T) func() (string, error)
		wantSuffix string
		wantErr    string
	}{
		{
			name: "returns registry dir under temp home",
			homeFunc: func(t *testing.T) func() (string, error) {
				t.Helper()
				tmp := t.TempDir()
				return func() (string, error) { return tmp, nil }
			},
			wantSuffix: filepath.Join(".config", "agentpack", "packages"),
		},
		{
			name: "returns error when home dir lookup fails",
			homeFunc: func(t *testing.T) func() (string, error) {
				t.Helper()
				return func() (string, error) { return "", fmt.Errorf("no home") }
			},
			wantErr: "home dir",
		},
		{
			name: "returns error when mkdir fails because file blocks directory",
			homeFunc: func(t *testing.T) func() (string, error) {
				t.Helper()
				tmp := t.TempDir()
				// Place a regular file where .config should be created.
				blocker := filepath.Join(tmp, ".config")
				require.NoError(t, os.WriteFile(blocker, []byte("block"), 0o644))
				return func() (string, error) { return tmp, nil }
			},
			wantErr: "mkdir registry dir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restore := registry.SetOsUserHomeDir(tt.homeFunc(t))
			defer restore()

			r := registry.New()
			got, err := r.Dir()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.True(t, filepath.IsAbs(got))
			assert.Contains(t, got, tt.wantSuffix)
		})
	}
}

// --------------------------------------------------------------------------
// TestSave
// --------------------------------------------------------------------------

func TestSave(t *testing.T) {
	tests := []struct {
		name     string
		manifest *registry.PackageManifest
		setup    func(t *testing.T, pkgDir string)
		wantErr  string
	}{
		{
			name: "saves manifest successfully",
			manifest: &registry.PackageManifest{
				Name:    "save-ok",
				Source:  "github.com/org/repo",
				Version: "1.0.0",
				Files: []registry.InstalledFile{
					{
						Path:   "skills/foo.md",
						SHA256: "abc123",
						Target: "claude-code",
						Dir:    "/tmp/dir",
					},
				},
			},
		},
		{
			name: "saves manifest with no files",
			manifest: &registry.PackageManifest{
				Name:   "empty-plugin",
				Source: "github.com/org/empty",
			},
		},
		{
			name: "returns error when registry dir is read-only",
			manifest: &registry.PackageManifest{
				Name:   "ro-plugin",
				Source: "src",
			},
			setup: func(t *testing.T, pkgDir string) {
				t.Helper()
				require.NoError(t, os.Chmod(pkgDir, 0o555))
				t.Cleanup(func() { _ = os.Chmod(pkgDir, 0o755) })
			},
			wantErr: "write manifest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp, restore := tempHome(t)
			defer restore()

			// Pre-create the packages dir.
			pkgDir := filepath.Join(tmp, ".config", "agentpack", "packages")
			require.NoError(t, os.MkdirAll(pkgDir, 0o755))

			if tt.setup != nil {
				tt.setup(t, pkgDir)
			}

			r := registry.New()
			err := r.Save(tt.manifest)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
		})
	}
}

// TestSaveDirFailure covers the Dir() failure path inside Save (separate
// table because it overrides osUserHomeDir differently).
func TestSaveDirFailure(t *testing.T) {
	tests := []struct {
		name    string
		wantErr string
	}{
		{
			name:    "returns error when Dir fails",
			wantErr: "home dir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restore := registry.SetOsUserHomeDir(func() (string, error) {
				return "", fmt.Errorf("no home")
			})
			defer restore()

			r := registry.New()
			err := r.Save(&registry.PackageManifest{Name: "x", Source: "s"})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// --------------------------------------------------------------------------
// TestSaveAndLoad (round-trip)
// --------------------------------------------------------------------------

func TestSaveAndLoad(t *testing.T) {
	tests := []struct {
		name     string
		manifest *registry.PackageManifest
	}{
		{
			name: "save and load round-trip",
			manifest: &registry.PackageManifest{
				Name:    "my-plugin",
				Source:  "github.com/org/repo",
				Version: "1.0.0",
				Files: []registry.InstalledFile{
					{
						Path:   "skills/foo.md",
						SHA256: "abc123",
						Target: "claude-code",
						Dir:    "/tmp/dir",
					},
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
			_, restore := tempHome(t)
			defer restore()

			r := registry.New()
			require.NoError(t, r.Save(tt.manifest))

			got, err := r.Load(tt.manifest.Name)
			require.NoError(t, err)
			assert.Equal(t, tt.manifest.Name, got.Name)
			assert.Equal(t, tt.manifest.Source, got.Source)
			assert.Equal(t, len(tt.manifest.Files), len(got.Files))
		})
	}
}

// --------------------------------------------------------------------------
// TestSaveAndLoadNamespaced
// --------------------------------------------------------------------------

// TestSaveAndLoadNamespaced verifies that namespaced owner/repo package names
// round-trip correctly through the registry. The on-disk filename uses "--"
// instead of "/" so no directory separators appear in the path, but the Name
// field stored in YAML retains the original "owner/repo" form.
func TestSaveAndLoadNamespaced(t *testing.T) {
	tests := []struct {
		name     string
		manifest *registry.PackageManifest
		wantFile string
	}{
		{
			name: "owner/repo name saves as owner--repo.yaml and loads back correctly",
			manifest: &registry.PackageManifest{
				Name:    "jeffallan/claude-skills",
				Source:  "github.com/jeffallan/claude-skills",
				Version: "1.0.0",
				Files: []registry.InstalledFile{
					{
						Path:   "skills/review.md",
						SHA256: "deadbeef",
						Target: "claude-code",
						Dir:    "/tmp/dir",
					},
				},
			},
			wantFile: "jeffallan--claude-skills.yaml",
		},
		{
			name: "simple name without slash saves as name.yaml and loads back correctly",
			manifest: &registry.PackageManifest{
				Name:    "my-plugin",
				Source:  "github.com/org/my-plugin",
				Version: "2.0.0",
			},
			wantFile: "my-plugin.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp, restore := tempHome(t)
			defer restore()

			r := registry.New()
			require.NoError(t, r.Save(tt.manifest))

			// Verify the on-disk filename uses "--" instead of "/".
			pkgDir := filepath.Join(tmp, ".config", "agentpack", "packages")
			_, statErr := os.Stat(filepath.Join(pkgDir, tt.wantFile))
			require.NoError(t, statErr)

			// Verify round-trip: Load by the original name returns correct data.
			got, err := r.Load(tt.manifest.Name)
			require.NoError(t, err)
			assert.Equal(t, tt.manifest.Name, got.Name)
			assert.Equal(t, tt.manifest.Source, got.Source)
			assert.Equal(t, tt.manifest.Version, got.Version)
			assert.Len(t, got.Files, len(tt.manifest.Files))

			// Verify List returns the manifest with the correct namespaced name.
			all, listErr := r.List()
			require.NoError(t, listErr)
			require.Len(t, all, 1)
			assert.Equal(t, tt.manifest.Name, all[0].Name)

			// Verify Remove cleans up by the original name.
			require.NoError(t, r.Remove(tt.manifest.Name))
			_, statErr = os.Stat(filepath.Join(pkgDir, tt.wantFile))
			assert.True(t, os.IsNotExist(statErr))
		})
	}
}

// --------------------------------------------------------------------------
// TestLoad
// --------------------------------------------------------------------------

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T, pkgDir string) string // returns package name
		wantErr string
	}{
		{
			name: "nonexistent package returns not-found error",
			setup: func(t *testing.T, _ string) string {
				t.Helper()
				return "does-not-exist"
			},
			wantErr: "not found in registry",
		},
		{
			name: "unreadable file returns read error",
			setup: func(t *testing.T, pkgDir string) string {
				t.Helper()
				name := "unreadable"
				path := filepath.Join(pkgDir, name+".yaml")
				require.NoError(t, os.WriteFile(path, []byte("name: unreadable\n"), 0o644))
				require.NoError(t, os.Chmod(path, 0o000))
				t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
				return name
			},
			wantErr: "read manifest",
		},
		{
			name: "corrupt YAML returns parse error",
			setup: func(t *testing.T, pkgDir string) string {
				t.Helper()
				name := "corrupt"
				path := filepath.Join(pkgDir, name+".yaml")
				// A tab at the start of a continuation line violates YAML indentation
				// rules and causes yaml.v3 to return a parse error.
				require.NoError(
					t,
					os.WriteFile(path, []byte("name: test\n\tversion: 1.0\n"), 0o644),
				)
				return name
			},
			wantErr: "parse manifest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp, restore := tempHome(t)
			defer restore()

			// Pre-create the packages dir.
			pkgDir := filepath.Join(tmp, ".config", "agentpack", "packages")
			require.NoError(t, os.MkdirAll(pkgDir, 0o755))

			pkgName := tt.setup(t, pkgDir)

			r := registry.New()
			_, err := r.Load(pkgName)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
		})
	}
}

// --------------------------------------------------------------------------
// TestRemove
// --------------------------------------------------------------------------

func TestRemove(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T, pkgDir string) string // returns pkg name
		wantErr string
	}{
		{
			name: "remove existing manifest",
			setup: func(t *testing.T, _ string) string {
				t.Helper()
				r := registry.New()
				m := &registry.PackageManifest{Name: "to-remove", Source: "src"}
				require.NoError(t, r.Save(m))
				return "to-remove"
			},
		},
		{
			name: "remove nonexistent is no-op",
			setup: func(t *testing.T, _ string) string {
				t.Helper()
				return "ghost"
			},
		},
		{
			name: "returns error when file cannot be removed due to dir permissions",
			setup: func(t *testing.T, pkgDir string) string {
				t.Helper()
				name := "perm-blocked"
				path := filepath.Join(pkgDir, name+".yaml")
				require.NoError(t, os.WriteFile(path, []byte("name: perm-blocked\n"), 0o644))
				// Make directory read-only so os.Remove fails.
				require.NoError(t, os.Chmod(pkgDir, 0o555))
				t.Cleanup(func() { _ = os.Chmod(pkgDir, 0o755) })
				return name
			},
			wantErr: "remove manifest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp, restore := tempHome(t)
			defer restore()

			// Pre-create the packages dir.
			pkgDir := filepath.Join(tmp, ".config", "agentpack", "packages")
			require.NoError(t, os.MkdirAll(pkgDir, 0o755))

			pkgName := tt.setup(t, pkgDir)

			r := registry.New()
			err := r.Remove(pkgName)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)

			dir, dirErr := r.Dir()
			require.NoError(t, dirErr)
			_, statErr := os.Stat(filepath.Join(dir, pkgName+".yaml"))
			assert.True(t, os.IsNotExist(statErr))
		})
	}
}

// TestLoadDirFailure covers the Dir() failure path inside Load.
func TestLoadDirFailure(t *testing.T) {
	tests := []struct {
		name    string
		wantErr string
	}{
		{
			name:    "returns error when Dir fails",
			wantErr: "home dir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restore := registry.SetOsUserHomeDir(func() (string, error) {
				return "", fmt.Errorf("no home")
			})
			defer restore()

			r := registry.New()
			_, err := r.Load("any-pkg")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// TestRemoveDirFailure covers the Dir() failure path inside Remove.
func TestRemoveDirFailure(t *testing.T) {
	tests := []struct {
		name    string
		wantErr string
	}{
		{
			name:    "returns error when Dir fails",
			wantErr: "home dir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restore := registry.SetOsUserHomeDir(func() (string, error) {
				return "", fmt.Errorf("no home")
			})
			defer restore()

			r := registry.New()
			err := r.Remove("some-pkg")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// --------------------------------------------------------------------------
// TestList
// --------------------------------------------------------------------------

func TestList(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T, pkgDir string)
		wantLen int
		wantErr string
	}{
		{
			name:    "empty registry returns nil",
			setup:   func(t *testing.T, _ string) { t.Helper() },
			wantLen: 0,
		},
		{
			name: "lists all saved manifests",
			setup: func(t *testing.T, _ string) {
				t.Helper()
				r := registry.New()
				for _, n := range []string{"alpha", "beta", "gamma"} {
					require.NoError(
						t,
						r.Save(&registry.PackageManifest{Name: n, Source: "src"}),
					)
				}
			},
			wantLen: 3,
		},
		{
			name: "skips subdirectories and non-yaml files",
			setup: func(t *testing.T, pkgDir string) {
				t.Helper()
				r := registry.New()
				require.NoError(
					t,
					r.Save(&registry.PackageManifest{Name: "valid", Source: "src"}),
				)
				// Write a non-yaml file (should be skipped).
				require.NoError(
					t,
					os.WriteFile(filepath.Join(pkgDir, "notes.txt"), []byte("ignore"), 0o644),
				)
				// Create a subdirectory (should be skipped).
				require.NoError(t, os.Mkdir(filepath.Join(pkgDir, "subdir"), 0o755))
			},
			wantLen: 1,
		},
		{
			name: "returns error when a manifest fails to load",
			setup: func(t *testing.T, pkgDir string) {
				t.Helper()
				// A tab at the start of a continuation line causes yaml.v3 to return
				// a parse error.
				require.NoError(
					t,
					os.WriteFile(
						filepath.Join(pkgDir, "bad.yaml"),
						[]byte("name: test\n\tversion: 1.0\n"),
						0o644,
					),
				)
			},
			wantErr: "parse manifest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp, restore := tempHome(t)
			defer restore()

			// Pre-create the packages dir.
			pkgDir := filepath.Join(tmp, ".config", "agentpack", "packages")
			require.NoError(t, os.MkdirAll(pkgDir, 0o755))

			tt.setup(t, pkgDir)

			r := registry.New()
			got, err := r.List()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Len(t, got, tt.wantLen)
		})
	}
}

// TestListDirFailure covers the Dir() failure path inside List.
func TestListDirFailure(t *testing.T) {
	tests := []struct {
		name    string
		wantErr string
	}{
		{
			name:    "returns error when Dir fails",
			wantErr: "home dir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restore := registry.SetOsUserHomeDir(func() (string, error) {
				return "", fmt.Errorf("no home")
			})
			defer restore()

			r := registry.New()
			_, err := r.List()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
