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
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/retr0h/agentpack/pkg/list"
	listmocks "github.com/retr0h/agentpack/pkg/list/mocks"
	"github.com/retr0h/agentpack/pkg/registry"
)

// --------------------------------------------------------------------------
// TestRun
// --------------------------------------------------------------------------

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setupMocks func(reg *listmocks.MockRegistry)
		wantCount  int
		wantFound  string
		wantErr    string
	}{
		{
			name: "returns entries from registry sorted by name",
			setupMocks: func(reg *listmocks.MockRegistry) {
				reg.EXPECT().List().Return([]*registry.PackageManifest{
					{
						Name:    "list-test-pkg",
						Source:  "github.com/org/test",
						Version: "v1.0.0",
						SHA:     "abc1234567890",
						Files: []registry.InstalledFile{
							{Path: ".claude/skills/x/SKILL.md", Target: "claude-code"},
						},
					},
				}, nil)
			},
			wantCount: 1,
			wantFound: "list-test-pkg",
		},
		{
			name: "returns empty list when registry has no manifests",
			setupMocks: func(reg *listmocks.MockRegistry) {
				reg.EXPECT().List().Return(nil, nil)
			},
			wantCount: 0,
		},
		{
			name: "multiple packages are sorted alphabetically",
			setupMocks: func(reg *listmocks.MockRegistry) {
				reg.EXPECT().List().Return([]*registry.PackageManifest{
					{Name: "zebra", Source: "github.com/org/zebra", Files: []registry.InstalledFile{}},
					{Name: "alpha", Source: "github.com/org/alpha", Files: []registry.InstalledFile{}},
					{Name: "middle", Source: "github.com/org/middle", Files: []registry.InstalledFile{}},
				}, nil)
			},
			wantCount: 3,
			wantFound: "alpha",
		},
		{
			name: "registry list error is propagated",
			setupMocks: func(reg *listmocks.MockRegistry) {
				reg.EXPECT().List().Return(nil, errors.New("registry unavailable"))
			},
			wantErr: "registry unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			reg := listmocks.NewMockRegistry(ctrl)
			tt.setupMocks(reg)

			entries, err := list.RunWithRegistry(reg)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}

				if !strContains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
				}

				return
			}

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
					}
				}

				if !found {
					t.Errorf("%q not found in list entries", tt.wantFound)
				}
			}

			// Verify sort order when multiple entries are returned.
			for i := 1; i < len(entries); i++ {
				if entries[i-1].Name > entries[i].Name {
					t.Errorf("entries not sorted: %q > %q at index %d", entries[i-1].Name, entries[i].Name, i)
				}
			}
		})
	}
}

func strContains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}())
}
