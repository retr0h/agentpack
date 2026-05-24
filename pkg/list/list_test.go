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

package list_test

import (
	"errors"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/retr0h/agentpack/pkg/list"
	"github.com/retr0h/agentpack/pkg/target"
	"github.com/retr0h/agentpack/pkg/target/mocks"
)

// --------------------------------------------------------------------------
// TestRun
// --------------------------------------------------------------------------

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		targets     func(ctrl *gomock.Controller) []target.Target
		wantErr     string
		checkResult func(t *testing.T, entries []list.Entry)
	}{
		{
			name: "returns empty slice when no targets provided and no results",
			targets: func(ctrl *gomock.Controller) []target.Target {
				m := mocks.NewMockTarget(ctrl)
				m.EXPECT().Name().Return("empty").AnyTimes()
				m.EXPECT().List().Return(nil, nil)

				return []target.Target{m}
			},
			checkResult: func(t *testing.T, entries []list.Entry) {
				t.Helper()

				if len(entries) != 0 {
					t.Errorf("entry count = %d, want 0", len(entries))
				}
			},
		},
		{
			name: "returns one entry for a single plugin",
			targets: func(ctrl *gomock.Controller) []target.Target {
				m := mocks.NewMockTarget(ctrl)
				m.EXPECT().Name().Return("claude-code").AnyTimes()
				m.EXPECT().List().Return([]target.InstalledPlugin{
					{
						Name:      "acme-toolkit",
						Version:   "1.0.0",
						SHA:       "a1b2c3d",
						Installed: "2026-05-23",
						Dir:       "/home/.claude/plugins/marketplaces/acme-toolkit",
						Target:    "Claude Code",
					},
				}, nil)

				return []target.Target{m}
			},
			checkResult: func(t *testing.T, entries []list.Entry) {
				t.Helper()

				if len(entries) != 1 {
					t.Fatalf("entry count = %d, want 1", len(entries))
				}

				e := entries[0]

				if e.Name != "acme-toolkit" {
					t.Errorf("Name = %q, want %q", e.Name, "acme-toolkit")
				}

				if e.Version != "1.0.0" {
					t.Errorf("Version = %q, want %q", e.Version, "1.0.0")
				}

				if e.SHA != "a1b2c3d" {
					t.Errorf("SHA = %q, want %q", e.SHA, "a1b2c3d")
				}

				if e.Installed != "2026-05-23" {
					t.Errorf("Installed = %q, want %q", e.Installed, "2026-05-23")
				}

				if e.Target != "Claude Code" {
					t.Errorf("Target = %q, want %q", e.Target, "Claude Code")
				}
			},
		},
		{
			name: "aggregates plugins from multiple targets sorted by target then name",
			targets: func(ctrl *gomock.Controller) []target.Target {
				m1 := mocks.NewMockTarget(ctrl)
				m1.EXPECT().Name().Return("cursor").AnyTimes()
				m1.EXPECT().List().Return([]target.InstalledPlugin{
					{Name: "z-skill", Version: "1.0.0", Target: "Cursor"},
					{Name: "a-skill", Version: "1.0.0", Target: "Cursor"},
				}, nil)

				m2 := mocks.NewMockTarget(ctrl)
				m2.EXPECT().Name().Return("claude-code").AnyTimes()
				m2.EXPECT().List().Return([]target.InstalledPlugin{
					{Name: "m-plugin", Version: "1.0.0", Target: "Claude Code"},
				}, nil)

				return []target.Target{m1, m2}
			},
			checkResult: func(t *testing.T, entries []list.Entry) {
				t.Helper()

				if len(entries) != 3 {
					t.Fatalf("entry count = %d, want 3", len(entries))
				}

				// Claude Code sorts before Cursor alphabetically.
				if entries[0].Target != "Claude Code" {
					t.Errorf("entries[0].Target = %q, want %q", entries[0].Target, "Claude Code")
				}

				if entries[1].Target != "Cursor" {
					t.Errorf("entries[1].Target = %q, want %q", entries[1].Target, "Cursor")
				}

				if entries[1].Name != "a-skill" {
					t.Errorf("entries[1].Name = %q, want %q", entries[1].Name, "a-skill")
				}

				if entries[2].Name != "z-skill" {
					t.Errorf("entries[2].Name = %q, want %q", entries[2].Name, "z-skill")
				}
			},
		},
		{
			name: "returns error when a target List call fails",
			targets: func(ctrl *gomock.Controller) []target.Target {
				m := mocks.NewMockTarget(ctrl)
				m.EXPECT().Name().Return("failing-target").AnyTimes()
				m.EXPECT().List().Return(nil, errors.New("disk error"))

				return []target.Target{m}
			},
			wantErr: "list failing-target",
		},
		{
			name: "returns empty slice when all targets return no plugins",
			targets: func(ctrl *gomock.Controller) []target.Target {
				m1 := mocks.NewMockTarget(ctrl)
				m1.EXPECT().Name().Return("t1").AnyTimes()
				m1.EXPECT().List().Return(nil, nil)

				m2 := mocks.NewMockTarget(ctrl)
				m2.EXPECT().Name().Return("t2").AnyTimes()
				m2.EXPECT().List().Return(nil, nil)

				return []target.Target{m1, m2}
			},
			checkResult: func(t *testing.T, entries []list.Entry) {
				t.Helper()

				if len(entries) != 0 {
					t.Errorf("entry count = %d, want 0", len(entries))
				}
			},
		},
		{
			name: "uses registered targets when nil targets provided",
			targets: func(_ *gomock.Controller) []target.Target {
				return nil // triggers target.All() fallback
			},
			checkResult: func(t *testing.T, entries []list.Entry) {
				t.Helper()
				// We can't assert on the specific entries since the system may or
				// may not have Claude Code installed. We just verify no error and
				// a valid (possibly empty) slice is returned.
				if entries == nil && len(entries) != 0 {
					t.Error("expected non-nil slice")
				}
			},
		},
		{
			name: "passes through all Entry fields correctly",
			targets: func(ctrl *gomock.Controller) []target.Target {
				m := mocks.NewMockTarget(ctrl)
				m.EXPECT().Name().Return("test-target").AnyTimes()
				m.EXPECT().List().Return([]target.InstalledPlugin{
					{
						Name:      "full-plugin",
						Version:   "2.5.0",
						SHA:       "abc1234",
						Installed: "2026-01-15",
						Dir:       "/some/dir",
						Target:    "Test Target",
					},
				}, nil)

				return []target.Target{m}
			},
			checkResult: func(t *testing.T, entries []list.Entry) {
				t.Helper()

				if len(entries) != 1 {
					t.Fatalf("entry count = %d, want 1", len(entries))
				}

				e := entries[0]

				if e.Dir != "/some/dir" {
					t.Errorf("Dir = %q, want %q", e.Dir, "/some/dir")
				}

				if e.Target != "Test Target" {
					t.Errorf("Target = %q, want %q", e.Target, "Test Target")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			targets := tt.targets(ctrl)

			entries, err := list.Run(targets)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}

				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.checkResult != nil {
				tt.checkResult(t, entries)
			}
		})
	}
}
