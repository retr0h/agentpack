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
			name: "nonexistent file returns empty lockfile with version 2",
			setup: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "agentpack.lock")
			},
			wantLen:     0,
			wantVersion: 2,
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
			lf:   &lock.Lockfile{LockVersion: 2},
		},
		{
			name: "writes lockfile with packages",
			lf: &lock.Lockfile{
				LockVersion: 2,
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
			lf:   &lock.Lockfile{LockVersion: 2},
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
			err := lock.Save(path, &lock.Lockfile{LockVersion: 2})

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

			lf := &lock.Lockfile{LockVersion: 2, Packages: tt.initial}
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

			lf := &lock.Lockfile{LockVersion: 2, Packages: tt.initial}
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

			lf := &lock.Lockfile{LockVersion: 2, Packages: tt.initial}
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
// TestLockfile_SetMergeSkillsTargetsFiles
// --------------------------------------------------------------------------

func TestLockfile_SetMergeSkillsTargetsFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		first         lock.LockedPackage
		second        lock.LockedPackage
		wantSkills    []string
		wantTargets   []string
		wantFilePaths []string
	}{
		{
			name: "set same package twice merges skills targets and files",
			first: lock.LockedPackage{
				Name:    "multi-pkg",
				Source:  "github.com/org/multi",
				SHA:     "sha1",
				Skills:  []string{"k8s"},
				Targets: []string{"claude-code"},
				Files: []lock.LockedFile{
					{Path: "skills/k8s/SKILL.md", SHA256: "aaa", Target: "claude-code"},
				},
			},
			second: lock.LockedPackage{
				Name:    "multi-pkg",
				Source:  "github.com/org/multi",
				SHA:     "sha2",
				Skills:  []string{"react"},
				Targets: []string{"cursor"},
				Files: []lock.LockedFile{
					{Path: "skills/react/SKILL.md", SHA256: "bbb", Target: "cursor"},
				},
			},
			wantSkills:    []string{"k8s", "react"},
			wantTargets:   []string{"claude-code", "cursor"},
			wantFilePaths: []string{"skills/k8s/SKILL.md", "skills/react/SKILL.md"},
		},
		{
			name: "set same package twice with overlapping file replaces existing entry",
			first: lock.LockedPackage{
				Name:    "overlap-pkg",
				Source:  "github.com/org/overlap",
				SHA:     "sha1",
				Skills:  []string{"k8s"},
				Targets: []string{"claude-code"},
				Files: []lock.LockedFile{
					{Path: "skills/k8s/SKILL.md", SHA256: "old-hash", Target: "claude-code"},
				},
			},
			second: lock.LockedPackage{
				Name:    "overlap-pkg",
				Source:  "github.com/org/overlap",
				SHA:     "sha2",
				Skills:  []string{"k8s"},
				Targets: []string{"claude-code"},
				Files: []lock.LockedFile{
					{Path: "skills/k8s/SKILL.md", SHA256: "new-hash", Target: "claude-code"},
				},
			},
			wantSkills:    []string{"k8s"},
			wantTargets:   []string{"claude-code"},
			wantFilePaths: []string{"skills/k8s/SKILL.md"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lf := &lock.Lockfile{LockVersion: 2}
			lf.Set(tt.first)
			lf.Set(tt.second)

			require.Len(t, lf.Packages, 1)

			got := lf.Find(tt.first.Name)
			require.NotNil(t, got)

			assert.ElementsMatch(t, tt.wantSkills, got.Skills)
			assert.ElementsMatch(t, tt.wantTargets, got.Targets)

			gotPaths := make([]string, len(got.Files))
			for i, f := range got.Files {
				gotPaths[i] = f.Path
			}

			assert.ElementsMatch(t, tt.wantFilePaths, gotPaths)

			// Verify overlapping file is updated (last write wins for same path+target).
			if tt.name == "set same package twice with overlapping file replaces existing entry" {
				require.Len(t, got.Files, 1)
				assert.Equal(t, "new-hash", got.Files[0].SHA256)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestLockfile_RemoveSkill
// --------------------------------------------------------------------------

func TestLockfile_RemoveSkill(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		initial         lock.LockedPackage
		removeSkill     string
		wantSkills      []string
		wantFilePaths   []string
		wantAbsentPaths []string
	}{
		{
			// fileMatchesSkill checks for /skills/<skill>/ with a leading slash,
			// matching paths as they appear when installed into a target directory
			// (e.g. ".claude/skills/react/SKILL.md").
			name: "removes named skill and prunes matching files",
			initial: lock.LockedPackage{
				Name:    "prune-pkg",
				Source:  "github.com/org/prune",
				SHA:     "sha1",
				Skills:  []string{"k8s", "react"},
				Targets: []string{"claude-code"},
				Files: []lock.LockedFile{
					{Path: ".claude/skills/k8s/SKILL.md", SHA256: "aaa", Target: "claude-code"},
					{Path: ".claude/skills/react/SKILL.md", SHA256: "bbb", Target: "claude-code"},
					{Path: "commands/scan.md", SHA256: "ccc", Target: "claude-code"},
				},
			},
			removeSkill:     "react",
			wantSkills:      []string{"k8s"},
			wantFilePaths:   []string{".claude/skills/k8s/SKILL.md", "commands/scan.md"},
			wantAbsentPaths: []string{".claude/skills/react/SKILL.md"},
		},
		{
			name: "remove skill not in list is a no-op",
			initial: lock.LockedPackage{
				Name:    "noop-pkg",
				Source:  "github.com/org/noop",
				SHA:     "sha1",
				Skills:  []string{"k8s"},
				Targets: []string{"claude-code"},
				Files: []lock.LockedFile{
					{Path: ".claude/skills/k8s/SKILL.md", SHA256: "aaa", Target: "claude-code"},
				},
			},
			removeSkill:     "nonexistent",
			wantSkills:      []string{"k8s"},
			wantFilePaths:   []string{".claude/skills/k8s/SKILL.md"},
			wantAbsentPaths: nil,
		},
		{
			name: "remove skill from nonexistent package is a no-op",
			initial: lock.LockedPackage{
				Name: "other-pkg",
			},
			removeSkill:     "k8s",
			wantSkills:      nil,
			wantFilePaths:   nil,
			wantAbsentPaths: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lf := &lock.Lockfile{LockVersion: 2}
			lf.Set(tt.initial)

			targetName := tt.initial.Name
			if tt.name == "remove skill from nonexistent package is a no-op" {
				targetName = "does-not-exist"
			}

			lf.RemoveSkill(targetName, tt.removeSkill)

			got := lf.Find(tt.initial.Name)
			if tt.name == "remove skill from nonexistent package is a no-op" {
				// The entry still exists under the original name.
				require.NotNil(t, lf.Find(tt.initial.Name))
				return
			}

			require.NotNil(t, got)

			if tt.wantSkills == nil {
				assert.Empty(t, got.Skills)
			} else {
				assert.ElementsMatch(t, tt.wantSkills, got.Skills)
			}

			gotPaths := make([]string, len(got.Files))
			for i, f := range got.Files {
				gotPaths[i] = f.Path
			}

			if tt.wantFilePaths != nil {
				assert.ElementsMatch(t, tt.wantFilePaths, gotPaths)
			}

			for _, absent := range tt.wantAbsentPaths {
				assert.NotContains(t, gotPaths, absent)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestLockfile_RemoveEntry
// --------------------------------------------------------------------------

func TestLockfile_RemoveEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		initial []lock.LockedPackage
		remove  string
		wantLen int
		wantNil bool
	}{
		{
			name: "remove existing package deletes entry entirely",
			initial: []lock.LockedPackage{
				{
					Name:    "gone-pkg",
					Source:  "github.com/org/gone",
					SHA:     "sha1",
					Skills:  []string{"k8s"},
					Targets: []string{"claude-code"},
					Files: []lock.LockedFile{
						{Path: "skills/k8s/SKILL.md", SHA256: "aaa", Target: "claude-code"},
					},
				},
				{Name: "stays", Source: "github.com/org/stays", SHA: "sha2"},
			},
			remove:  "gone-pkg",
			wantLen: 1,
			wantNil: true,
		},
		{
			name: "remove nonexistent package leaves others intact",
			initial: []lock.LockedPackage{
				{Name: "keeper", Source: "github.com/org/keeper", SHA: "sha1"},
			},
			remove:  "ghost",
			wantLen: 1,
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lf := &lock.Lockfile{LockVersion: 2, Packages: tt.initial}
			lf.Remove(tt.remove)

			assert.Len(t, lf.Packages, tt.wantLen)

			if tt.wantNil {
				assert.Nil(t, lf.Find(tt.remove))
			}
		})
	}
}
