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

package packages_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/internal/packages"
)

// --------------------------------------------------------------------------
// TestLoad
// --------------------------------------------------------------------------

func TestLoad(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(t *testing.T) string
		wantLen   int
		wantFirst string
		wantErr   string
	}{
		{
			name: "nonexistent file returns empty config",
			setup: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "agentpack-packages.yaml")
			},
			wantLen: 0,
		},
		{
			name: "valid file parsed correctly",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				p := filepath.Join(dir, "agentpack-packages.yaml")
				content := "packages:\n  - name: security-skills\n    git: github.com/org/security-skills\n    ref: v1.0.0\n"
				require.NoError(t, os.WriteFile(p, []byte(content), 0o644))

				return p
			},
			wantLen:   1,
			wantFirst: "security-skills",
		},
		{
			name: "file with source field parsed correctly",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				p := filepath.Join(dir, "agentpack-packages.yaml")
				content := "packages:\n  - name: offline-plugin\n    source: ~/Downloads/plugin.agentpack\n"
				require.NoError(t, os.WriteFile(p, []byte(content), 0o644))

				return p
			},
			wantLen:   1,
			wantFirst: "offline-plugin",
		},
		{
			name: "malformed YAML returns error",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				p := filepath.Join(dir, "agentpack-packages.yaml")
				require.NoError(t, os.WriteFile(p, []byte("packages: [\nnot valid"), 0o644))

				return p
			},
			wantErr: "parse packages file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := tt.setup(t)
			cfg, err := packages.Load(path)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Len(t, cfg.Packages, tt.wantLen)

			if tt.wantFirst != "" && len(cfg.Packages) > 0 {
				assert.Equal(t, tt.wantFirst, cfg.Packages[0].Name)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestSave
// --------------------------------------------------------------------------

func TestSave(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     *packages.Config
		wantErr string
	}{
		{
			name: "writes empty config",
			cfg:  &packages.Config{},
		},
		{
			name: "writes config with packages",
			cfg: &packages.Config{
				Packages: []packages.Package{
					{Name: "security-skills", Git: "github.com/org/security-skills", Ref: "v1.0.0"},
					{Name: "offline-plugin", Source: "~/Downloads/plugin.agentpack"},
				},
			},
		},
		{
			name: "creates parent directories",
			cfg:  &packages.Config{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "sub", "dir", "agentpack-packages.yaml")

			err := packages.Save(path, tt.cfg)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)

			got, readErr := packages.Load(path)
			require.NoError(t, readErr)
			assert.Len(t, got.Packages, len(tt.cfg.Packages))
		})
	}
}

// --------------------------------------------------------------------------
// TestLoadPermissionDenied
// --------------------------------------------------------------------------

func TestLoadPermissionDenied(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		wantErr string
	}{
		{
			name: "unreadable file returns read packages file error",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				p := filepath.Join(dir, "agentpack-packages.yaml")
				require.NoError(t, os.WriteFile(p, []byte("packages: []\n"), 0o644))
				require.NoError(t, os.Chmod(p, 0o000))
				t.Cleanup(func() { _ = os.Chmod(p, 0o644) })

				return p
			},
			wantErr: "read packages file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if os.Getuid() == 0 {
				t.Skip("root bypasses file permissions")
			}

			path := tt.setup(t)
			_, err := packages.Load(path)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
		})
	}
}

// --------------------------------------------------------------------------
// TestSaveErrorPaths
// --------------------------------------------------------------------------

func TestSaveErrorPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		wantErr string
	}{
		{
			name: "mkdirall fails when parent is a regular file",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				blocker := filepath.Join(dir, "notadir")
				require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))

				return filepath.Join(blocker, "sub", "agentpack-packages.yaml")
			},
			wantErr: "mkdir for packages file",
		},
		{
			name: "writefile fails when dest dir is read-only",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				sub := filepath.Join(dir, "ro")
				require.NoError(t, os.MkdirAll(sub, 0o755))
				require.NoError(t, os.Chmod(sub, 0o555))
				t.Cleanup(func() { _ = os.Chmod(sub, 0o755) })

				return filepath.Join(sub, "agentpack-packages.yaml")
			},
			wantErr: "write packages file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if os.Getuid() == 0 {
				t.Skip("root bypasses file permissions")
			}

			path := tt.setup(t)
			err := packages.Save(path, &packages.Config{})

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
		})
	}
}

// --------------------------------------------------------------------------
// TestConfig_Add
// --------------------------------------------------------------------------

func TestConfig_Add(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		initial    []packages.Package
		add        packages.Package
		wantLen    int
		wantGit    string
		wantRef    string
		wantSource string
	}{
		{
			name:    "add new git entry",
			initial: nil,
			add:     packages.Package{Name: "new", Git: "github.com/org/new", Ref: "v1.0.0"},
			wantLen: 1,
			wantGit: "github.com/org/new",
			wantRef: "v1.0.0",
		},
		{
			name:       "add new source entry",
			initial:    nil,
			add:        packages.Package{Name: "offline", Source: "~/plugin.agentpack"},
			wantLen:    1,
			wantSource: "~/plugin.agentpack",
		},
		{
			name: "update existing entry overwrites in place",
			initial: []packages.Package{
				{Name: "existing", Git: "github.com/org/old"},
			},
			add:     packages.Package{Name: "existing", Git: "github.com/org/new", Ref: "v2.0.0"},
			wantLen: 1,
			wantGit: "github.com/org/new",
			wantRef: "v2.0.0",
		},
		{
			name: "add distinct name appends",
			initial: []packages.Package{
				{Name: "alpha", Git: "github.com/org/alpha"},
			},
			add:     packages.Package{Name: "beta", Git: "github.com/org/beta"},
			wantLen: 2,
			wantGit: "github.com/org/beta",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &packages.Config{Packages: tt.initial}
			cfg.Add(tt.add)

			require.Len(t, cfg.Packages, tt.wantLen)

			got := cfg.Find(tt.add.Name)
			require.NotNil(t, got)

			if tt.wantGit != "" {
				assert.Equal(t, tt.wantGit, got.Git)
			}

			if tt.wantRef != "" {
				assert.Equal(t, tt.wantRef, got.Ref)
			}

			if tt.wantSource != "" {
				assert.Equal(t, tt.wantSource, got.Source)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestConfig_Remove
// --------------------------------------------------------------------------

func TestConfig_Remove(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		initial []packages.Package
		remove  string
		wantLen int
	}{
		{
			name:    "remove nonexistent is no-op",
			initial: []packages.Package{{Name: "a"}},
			remove:  "b",
			wantLen: 1,
		},
		{
			name:    "remove existing entry",
			initial: []packages.Package{{Name: "a"}, {Name: "b"}},
			remove:  "a",
			wantLen: 1,
		},
		{
			name:    "remove from empty is no-op",
			initial: nil,
			remove:  "x",
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &packages.Config{Packages: tt.initial}
			cfg.Remove(tt.remove)

			assert.Len(t, cfg.Packages, tt.wantLen)
		})
	}
}

// --------------------------------------------------------------------------
// TestConfig_Find
// --------------------------------------------------------------------------

func TestConfig_Find(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		initial  []packages.Package
		find     string
		wantNil  bool
		wantName string
	}{
		{
			name:     "find existing entry",
			initial:  []packages.Package{{Name: "found", Git: "github.com/org/found"}},
			find:     "found",
			wantName: "found",
		},
		{
			name:    "find nonexistent returns nil",
			initial: []packages.Package{{Name: "other"}},
			find:    "missing",
			wantNil: true,
		},
		{
			name:    "find in empty config returns nil",
			initial: nil,
			find:    "x",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &packages.Config{Packages: tt.initial}
			got := cfg.Find(tt.find)

			if tt.wantNil {
				assert.Nil(t, got)

				return
			}

			require.NotNil(t, got)
			assert.Equal(t, tt.wantName, got.Name)
		})
	}
}
