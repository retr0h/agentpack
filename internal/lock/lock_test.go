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

package lock_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/internal/lock"
)

// --------------------------------------------------------------------------
// TestLoad
// --------------------------------------------------------------------------

func TestLoad(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setup       func(t *testing.T) string
		wantLen     int
		wantFirst   string
		wantVersion int
		wantErr     string
	}{
		{
			name: "nonexistent file returns empty lockfile with version 1",
			setup: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "agentpack.lock")
			},
			wantLen:     0,
			wantVersion: 1,
		},
		{
			name: "valid lock file parsed correctly",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				p := filepath.Join(dir, "agentpack.lock")
				content := "lockVersion: 1\npackages:\n  - name: security-skills\n    source: github.com/org/security-skills\n    sha: abc1234\n    resolved: \"2026-05-25T21:00:00Z\"\n"
				require.NoError(t, os.WriteFile(p, []byte(content), 0o644))

				return p
			},
			wantLen:     1,
			wantFirst:   "security-skills",
			wantVersion: 1,
		},
		{
			name: "malformed YAML returns error",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				p := filepath.Join(dir, "agentpack.lock")
				require.NoError(t, os.WriteFile(p, []byte("packages: [\nnot valid"), 0o644))

				return p
			},
			wantErr: "parse lock file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := tt.setup(t)
			lf, err := lock.Load(path)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Len(t, lf.Packages, tt.wantLen)
			assert.Equal(t, tt.wantVersion, lf.LockVersion)

			if tt.wantFirst != "" && len(lf.Packages) > 0 {
				assert.Equal(t, tt.wantFirst, lf.Packages[0].Name)
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
		lf      *lock.Lockfile
		wantErr string
	}{
		{
			name: "writes empty lockfile",
			lf:   &lock.Lockfile{LockVersion: 1},
		},
		{
			name: "writes lockfile with packages",
			lf: &lock.Lockfile{
				LockVersion: 1,
				Packages: []lock.LockedPackage{
					{
						Name:     "security-skills",
						Source:   "github.com/org/security-skills",
						Ref:      "v1.0.0",
						SHA:      "abc1234",
						Resolved: "2026-05-25T21:00:00Z",
					},
					{
						Name:     "devops-skills",
						Source:   "github.com/org/devops-skills",
						SHA:      "def5678",
						Resolved: "2026-05-25T21:00:00Z",
					},
				},
			},
		},
		{
			name: "creates parent directories",
			lf:   &lock.Lockfile{LockVersion: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "sub", "dir", "agentpack.lock")

			err := lock.Save(path, tt.lf)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)

			got, readErr := lock.Load(path)
			require.NoError(t, readErr)
			assert.Len(t, got.Packages, len(tt.lf.Packages))
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
			name: "unreadable file returns read lock file error",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				p := filepath.Join(dir, "agentpack.lock")
				require.NoError(t, os.WriteFile(p, []byte("lockVersion: 1\n"), 0o644))
				require.NoError(t, os.Chmod(p, 0o000))
				t.Cleanup(func() { _ = os.Chmod(p, 0o644) })

				return p
			},
			wantErr: "read lock file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if os.Getuid() == 0 {
				t.Skip("root bypasses file permissions")
			}

			path := tt.setup(t)
			_, err := lock.Load(path)

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

				return filepath.Join(blocker, "sub", "agentpack.lock")
			},
			wantErr: "mkdir for lock file",
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

				return filepath.Join(sub, "agentpack.lock")
			},
			wantErr: "write lock file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if os.Getuid() == 0 {
				t.Skip("root bypasses file permissions")
			}

			path := tt.setup(t)
			err := lock.Save(path, &lock.Lockfile{LockVersion: 1})

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
		})
	}
}

// --------------------------------------------------------------------------
// TestLockfile_Set
// --------------------------------------------------------------------------

func TestLockfile_Set(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		initial []lock.LockedPackage
		set     lock.LockedPackage
		wantLen int
		wantSHA string
		wantRef string
	}{
		{
			name:    "set new entry appends",
			initial: nil,
			set:     lock.LockedPackage{Name: "new", Source: "github.com/org/new", SHA: "abc1234"},
			wantLen: 1,
			wantSHA: "abc1234",
		},
		{
			name: "set existing entry overwrites in place",
			initial: []lock.LockedPackage{
				{Name: "existing", Source: "github.com/org/old", SHA: "old1234"},
			},
			set: lock.LockedPackage{
				Name:   "existing",
				Source: "github.com/org/old",
				SHA:    "new5678",
				Ref:    "v2.0.0",
			},
			wantLen: 1,
			wantSHA: "new5678",
			wantRef: "v2.0.0",
		},
		{
			name: "set distinct name appends",
			initial: []lock.LockedPackage{
				{Name: "alpha", Source: "github.com/org/alpha", SHA: "aaa"},
			},
			set:     lock.LockedPackage{Name: "beta", Source: "github.com/org/beta", SHA: "bbb"},
			wantLen: 2,
			wantSHA: "bbb",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lf := &lock.Lockfile{LockVersion: 1, Packages: tt.initial}
			lf.Set(tt.set)

			require.Len(t, lf.Packages, tt.wantLen)

			got := lf.Find(tt.set.Name)
			require.NotNil(t, got)
			assert.Equal(t, tt.wantSHA, got.SHA)

			if tt.wantRef != "" {
				assert.Equal(t, tt.wantRef, got.Ref)
			}
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
		initial []lock.LockedPackage
		remove  string
		wantLen int
	}{
		{
			name:    "remove nonexistent is no-op",
			initial: []lock.LockedPackage{{Name: "a"}},
			remove:  "b",
			wantLen: 1,
		},
		{
			name:    "remove existing entry",
			initial: []lock.LockedPackage{{Name: "a"}, {Name: "b"}},
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

			lf := &lock.Lockfile{LockVersion: 1, Packages: tt.initial}
			lf.Remove(tt.remove)

			assert.Len(t, lf.Packages, tt.wantLen)
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
		initial  []lock.LockedPackage
		find     string
		wantNil  bool
		wantName string
	}{
		{
			name: "find existing entry",
			initial: []lock.LockedPackage{
				{Name: "found", Source: "github.com/org/found", SHA: "abc"},
			},
			find:     "found",
			wantName: "found",
		},
		{
			name:    "find nonexistent returns nil",
			initial: []lock.LockedPackage{{Name: "other"}},
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

			lf := &lock.Lockfile{LockVersion: 1, Packages: tt.initial}
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
