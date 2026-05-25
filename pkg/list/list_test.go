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
	"testing"

	"github.com/retr0h/agentpack/pkg/list"
	"github.com/retr0h/agentpack/pkg/registry"
)

// --------------------------------------------------------------------------
// TestRun
// --------------------------------------------------------------------------

func TestRun(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T)
		wantCount int
		wantFound string
	}{
		{
			name: "returns entries from registry",
			setup: func(t *testing.T) {
				t.Helper()

				if err := registry.Save(&registry.PackageManifest{
					Name:    "list-test-pkg",
					Source:  "github.com/org/test",
					Version: "v1.0.0",
					SHA:     "abc1234567890",
					Files: []registry.InstalledFile{
						{Path: ".claude/skills/x/SKILL.md", Target: "claude-code"},
					},
				}); err != nil {
					t.Fatalf("Save: %v", err)
				}
			},
			wantCount: 1,
			wantFound: "list-test-pkg",
		},
		{
			name:      "returns empty list when registry is empty",
			setup:     func(t *testing.T) { t.Helper() },
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Redirect registry I/O to a temp dir so we never touch the real
			// ~/.config/agentpack/packages/ directory.
			tmp := t.TempDir()
			restore := registry.SetOsUserHomeDir(func() (string, error) {
				return tmp, nil
			})
			defer restore()

			tt.setup(t)

			entries, err := list.Run()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(entries) != tt.wantCount {
				t.Errorf("len = %d, want %d", len(entries), tt.wantCount)
			}

			if tt.wantFound != "" {
				found := false

				for _, e := range entries {
					if e.Name == tt.wantFound {
						found = true

						if e.Source != "github.com/org/test" {
							t.Errorf("Source = %q, want %q", e.Source, "github.com/org/test")
						}
					}
				}

				if !found {
					t.Errorf("%q not found in list entries", tt.wantFound)
				}
			}
		})
	}
}
