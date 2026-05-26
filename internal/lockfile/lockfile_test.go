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
				if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
					t.Fatalf("setup: %v", err)
				}
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
				if err := os.WriteFile(p, []byte("installs: [\nnot valid"), 0o644); err != nil {
					t.Fatalf("setup: %v", err)
				}
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
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(lf.Installs) != tt.wantLen {
				t.Errorf("len(Installs) = %d, want %d", len(lf.Installs), tt.wantLen)
			}

			if tt.wantFirst != "" && len(lf.Installs) > 0 {
				if lf.Installs[0].Name != tt.wantFirst {
					t.Errorf("Installs[0].Name = %q, want %q", lf.Installs[0].Name, tt.wantFirst)
				}
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
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Round-trip: read back and verify counts.
			got, readErr := lockfile.Read(path)
			if readErr != nil {
				t.Fatalf("read back: %v", readErr)
			}

			if len(got.Installs) != len(tt.lf.Installs) {
				t.Errorf("round-trip len = %d, want %d", len(got.Installs), len(tt.lf.Installs))
			}
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

			if len(lf.Installs) != tt.wantLen {
				t.Fatalf("len = %d, want %d", len(lf.Installs), tt.wantLen)
			}

			got := lf.Find(tt.add.Name)
			if got == nil {
				t.Fatal("Find returned nil after Add")
			}

			if len(got.Targets) != len(tt.wantTarget) {
				t.Errorf("Targets = %v, want %v", got.Targets, tt.wantTarget)
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

			if len(lf.Installs) != tt.wantLen {
				t.Errorf("len = %d, want %d", len(lf.Installs), tt.wantLen)
			}
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
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
				return
			}

			if got == nil {
				t.Fatal("expected non-nil, got nil")
			}

			if got.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", got.Name, tt.wantName)
			}
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

			if updated != tt.wantUpdated {
				t.Errorf("SetEnabled returned %v, want %v", updated, tt.wantUpdated)
			}

			if tt.wantUpdated {
				got := lf.Find(tt.target)
				if got == nil {
					t.Fatal("Find returned nil after SetEnabled")
				}

				if got.Enabled != tt.wantEnabled {
					t.Errorf("Enabled = %v, want %v", got.Enabled, tt.wantEnabled)
				}
			}
		})
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
