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

package lockfile_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/internal/lockfile"
)

// --------------------------------------------------------------------------
// TestRead
// --------------------------------------------------------------------------

func TestRead(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(t *testing.T) string // returns path
		wantLen   int
		wantFirst string // first install Name, if any
		wantErr   string
	}{
		{
			name: "nonexistent file returns empty lockfile",
			setup: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "agentpack-lock.yaml")
			},
			wantLen: 0,
		},
		{
			name: "valid lockfile parsed correctly",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				p := filepath.Join(dir, "agentpack-lock.yaml")
				content := "installs:\n  - name: my-plugin\n    source: github.com/org/repo\n    enabled: true\n"
				err := os.WriteFile(p, []byte(content), 0o644)
				require.NoError(t, err)
				return p
			},
			wantLen:   1,
			wantFirst: "my-plugin",
		},
		{
			name: "malformed YAML returns error",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				p := filepath.Join(dir, "agentpack-lock.yaml")
				err := os.WriteFile(p, []byte("installs: [\nnot valid"), 0o644)
				require.NoError(t, err)
				return p
			},
			wantErr: "parse lockfile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := tt.setup(t)
			lf, err := lockfile.Read(path)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Len(t, lf.Installs, tt.wantLen)

			if tt.wantFirst != "" && len(lf.Installs) > 0 {
				assert.Equal(t, tt.wantFirst, lf.Installs[0].Name)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestWrite
// --------------------------------------------------------------------------

func TestWrite(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		lf      *lockfile.Lockfile
		wantErr string
	}{
		{
			name: "writes empty lockfile",
			lf:   &lockfile.Lockfile{},
		},
		{
			name: "writes lockfile with installs",
			lf: &lockfile.Lockfile{
				Installs: []lockfile.Install{
					{Name: "plugin-a", Source: "github.com/org/a", Enabled: true},
					{Name: "plugin-b", Source: "github.com/org/b", Enabled: false},
				},
			},
		},
		{
			name: "creates parent directories",
			lf:   &lockfile.Lockfile{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "sub", "dir", "agentpack-lock.yaml")

			err := lockfile.Write(path, tt.lf)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			// Round-trip: read back and verify counts.
			got, readErr := lockfile.Read(path)
			require.NoError(t, readErr)
			assert.Len(t, got.Installs, len(tt.lf.Installs))
		})
	}
}

// --------------------------------------------------------------------------
// TestLockfile_Add
// --------------------------------------------------------------------------

func TestLockfile_Add(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		initial    []lockfile.Install
		add        lockfile.Install
		wantLen    int
		wantTarget []string // expected targets on the added/merged entry
	}{
		{
			name:       "add new entry",
			initial:    nil,
			add:        lockfile.Install{Name: "new", Source: "src", Targets: []string{"claude-code"}, Enabled: true},
			wantLen:    1,
			wantTarget: []string{"claude-code"},
		},
		{
			name: "update existing entry merges targets",
			initial: []lockfile.Install{
				{Name: "existing", Source: "src", Targets: []string{"claude-code"}},
			},
			add:        lockfile.Install{Name: "existing", Source: "src2", Targets: []string{"cursor"}},
			wantLen:    1,
			wantTarget: []string{"claude-code", "cursor"},
		},
		{
			name: "duplicate target not added twice",
			initial: []lockfile.Install{
				{Name: "dup", Source: "src", Targets: []string{"claude-code"}},
			},
			add:        lockfile.Install{Name: "dup", Source: "src", Targets: []string{"claude-code"}},
			wantLen:    1,
			wantTarget: []string{"claude-code"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lf := &lockfile.Lockfile{Installs: tt.initial}
			lf.Add(tt.add)

			require.Len(t, lf.Installs, tt.wantLen)

			got := lf.Find(tt.add.Name)
			require.NotNil(t, got)

			assert.Len(t, got.Targets, len(tt.wantTarget))
		})
	}
}

// --------------------------------------------------------------------------
// TestLockfile_Remove
// --------------------------------------------------------------------------

func TestLockfile_Remove(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		initial []lockfile.Install
		remove  string
		wantLen int
	}{
		{
			name:    "remove nonexistent is no-op",
			initial: []lockfile.Install{{Name: "a"}},
			remove:  "b",
			wantLen: 1,
		},
		{
			name:    "remove existing entry",
			initial: []lockfile.Install{{Name: "a"}, {Name: "b"}},
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

			lf := &lockfile.Lockfile{Installs: tt.initial}
			lf.Remove(tt.remove)

			assert.Len(t, lf.Installs, tt.wantLen)
		})
	}
}

// --------------------------------------------------------------------------
// TestLockfile_Find
// --------------------------------------------------------------------------

func TestLockfile_Find(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		initial  []lockfile.Install
		find     string
		wantNil  bool
		wantName string
	}{
		{
			name:     "find existing entry",
			initial:  []lockfile.Install{{Name: "found", Source: "src"}},
			find:     "found",
			wantName: "found",
		},
		{
			name:    "find nonexistent returns nil",
			initial: []lockfile.Install{{Name: "other"}},
			find:    "missing",
			wantNil: true,
		},
		{
			name:    "find in empty lockfile returns nil",
			initial: nil,
			find:    "x",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lf := &lockfile.Lockfile{Installs: tt.initial}
			got := lf.Find(tt.find)

			if tt.wantNil {
				assert.Nil(t, got)
				return
			}

			require.NotNil(t, got)
			assert.Equal(t, tt.wantName, got.Name)
		})
	}
}

// --------------------------------------------------------------------------
// TestLockfile_SetEnabled
// --------------------------------------------------------------------------

func TestLockfile_SetEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		initial     []lockfile.Install
		target      string
		enabled     bool
		wantUpdated bool
		wantEnabled bool
	}{
		{
			name:        "enable existing entry",
			initial:     []lockfile.Install{{Name: "p", Enabled: false}},
			target:      "p",
			enabled:     true,
			wantUpdated: true,
			wantEnabled: true,
		},
		{
			name:        "disable existing entry",
			initial:     []lockfile.Install{{Name: "p", Enabled: true}},
			target:      "p",
			enabled:     false,
			wantUpdated: true,
			wantEnabled: false,
		},
		{
			name:        "set on nonexistent returns false",
			initial:     nil,
			target:      "missing",
			enabled:     true,
			wantUpdated: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lf := &lockfile.Lockfile{Installs: tt.initial}
			updated := lf.SetEnabled(tt.target, tt.enabled)

			assert.Equal(t, tt.wantUpdated, updated)

			if tt.wantUpdated {
				got := lf.Find(tt.target)
				require.NotNil(t, got)
				assert.Equal(t, tt.wantEnabled, got.Enabled)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestReadPermissionDenied
// --------------------------------------------------------------------------

func TestReadPermissionDenied(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		wantErr string
	}{
		{
			name: "unreadable file returns read lockfile error",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				p := filepath.Join(dir, "agentpack-lock.yaml")
				err := os.WriteFile(p, []byte("installs: []"), 0o644)
				require.NoError(t, err)
				err = os.Chmod(p, 0o000)
				require.NoError(t, err)
				t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
				return p
			},
			wantErr: "read lockfile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if os.Getuid() == 0 {
				t.Skip("root bypasses file permissions")
			}

			path := tt.setup(t)
			_, err := lockfile.Read(path)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
		})
	}
}

// --------------------------------------------------------------------------
// TestWriteErrorPaths
// --------------------------------------------------------------------------

func TestWriteErrorPaths(t *testing.T) {
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
				err := os.WriteFile(blocker, []byte("x"), 0o644)
				require.NoError(t, err)
				return filepath.Join(blocker, "sub", "agentpack-lock.yaml")
			},
			wantErr: "mkdir for lockfile",
		},
		{
			name: "writefile fails when dest dir is read-only",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				sub := filepath.Join(dir, "ro")
				err := os.MkdirAll(sub, 0o755)
				require.NoError(t, err)
				err = os.Chmod(sub, 0o555)
				require.NoError(t, err)
				t.Cleanup(func() { _ = os.Chmod(sub, 0o755) })
				return filepath.Join(sub, "agentpack-lock.yaml")
			},
			wantErr: "write lockfile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if os.Getuid() == 0 {
				t.Skip("root bypasses file permissions")
			}

			path := tt.setup(t)
			err := lockfile.Write(path, &lockfile.Lockfile{})

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
		})
	}
}
