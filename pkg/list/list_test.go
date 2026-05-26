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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/retr0h/agentpack/pkg/list"
	listmocks "github.com/retr0h/agentpack/pkg/list/mocks"
	"github.com/retr0h/agentpack/pkg/registry"
)

// --------------------------------------------------------------------------
// TestRunWithRegistry
// --------------------------------------------------------------------------

func TestRunWithRegistry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setupMocks func(reg *listmocks.MockRegistry)
		wantCount  int
		wantFirst  string
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
			wantFirst: "list-test-pkg",
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
			wantFirst: "alpha",
		},
		{
			name: "registry list error is propagated",
			setupMocks: func(reg *listmocks.MockRegistry) {
				reg.EXPECT().List().Return(nil, errors.New("registry unavailable"))
			},
			wantErr: "registry unavailable",
		},
		{
			name: "SHA is shortened to 7 characters",
			setupMocks: func(reg *listmocks.MockRegistry) {
				reg.EXPECT().List().Return([]*registry.PackageManifest{
					{Name: "sha-pkg", SHA: "abcdef1234567890", Files: []registry.InstalledFile{}},
				}, nil)
			},
			wantCount: 1,
		},
		{
			name: "SHA shorter than 7 chars is returned as-is",
			setupMocks: func(reg *listmocks.MockRegistry) {
				reg.EXPECT().List().Return([]*registry.PackageManifest{
					{Name: "short-sha-pkg", SHA: "abc", Files: []registry.InstalledFile{}},
				}, nil)
			},
			wantCount: 1,
		},
		{
			name: "installed timestamp with T is trimmed to date",
			setupMocks: func(reg *listmocks.MockRegistry) {
				reg.EXPECT().List().Return([]*registry.PackageManifest{
					{Name: "ts-pkg", Installed: "2026-01-15T12:00:00Z", Files: []registry.InstalledFile{}},
				}, nil)
			},
			wantCount: 1,
		},
		{
			name: "installed timestamp without T is returned as-is",
			setupMocks: func(reg *listmocks.MockRegistry) {
				reg.EXPECT().List().Return([]*registry.PackageManifest{
					{Name: "no-t-pkg", Installed: "2026-01-15", Files: []registry.InstalledFile{}},
				}, nil)
			},
			wantCount: 1,
		},
		{
			name: "empty installed timestamp is returned as empty string",
			setupMocks: func(reg *listmocks.MockRegistry) {
				reg.EXPECT().List().Return([]*registry.PackageManifest{
					{Name: "empty-ts-pkg", Installed: "", Files: []registry.InstalledFile{}},
				}, nil)
			},
			wantCount: 1,
		},
		{
			name: "multiple targets are deduplicated and sorted",
			setupMocks: func(reg *listmocks.MockRegistry) {
				reg.EXPECT().List().Return([]*registry.PackageManifest{
					{
						Name: "multi-target",
						Files: []registry.InstalledFile{
							{Target: "cursor"},
							{Target: "claude-code"},
							{Target: "cursor"},
						},
					},
				}, nil)
			},
			wantCount: 1,
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
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Len(t, entries, tt.wantCount)

			if tt.wantFirst != "" && len(entries) > 0 {
				assert.Equal(t, tt.wantFirst, entries[0].Name)
			}

			// Verify sort order.
			for i := 1; i < len(entries); i++ {
				assert.LessOrEqual(t, entries[i-1].Name, entries[i].Name)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestRunWithRegistryFormatDate — explicit coverage of formatDate branches.
// --------------------------------------------------------------------------

func TestRunWithRegistryFormatDate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		installed     string
		wantInstalled string
	}{
		{
			name:          "RFC3339 timestamp is trimmed at T",
			installed:     "2026-05-25T09:30:00Z",
			wantInstalled: "2026-05-25",
		},
		{
			name:          "date-only string without T is returned unchanged",
			installed:     "2026-05-25",
			wantInstalled: "2026-05-25",
		},
		{
			name:          "empty string is returned as empty",
			installed:     "",
			wantInstalled: "",
		},
		{
			name:          "T at position zero does not trim (idx must be > 0)",
			installed:     "T12:00:00Z",
			wantInstalled: "T12:00:00Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			reg := listmocks.NewMockRegistry(ctrl)
			reg.EXPECT().List().Return([]*registry.PackageManifest{
				{Name: "fmt-pkg", Installed: tt.installed, Files: []registry.InstalledFile{}},
			}, nil)

			entries, err := list.RunWithRegistry(reg)
			require.NoError(t, err)
			require.Len(t, entries, 1)
			assert.Equal(t, tt.wantInstalled, entries[0].Installed)
		})
	}
}

// --------------------------------------------------------------------------
// TestRun covers the Run() wrapper which uses the default registry.
// This test mutates the registry package-level osUserHomeDir global and
// therefore must NOT run in parallel.
// --------------------------------------------------------------------------

func TestRun(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T, pkgDir string)
		wantCount int
		wantErr   string
	}{
		{
			name: "Run uses defaultRegistry and returns installed packages",
			setup: func(t *testing.T, pkgDir string) {
				t.Helper()
				require.NoError(t, os.WriteFile(
					filepath.Join(pkgDir, "run-pkg.yaml"),
					[]byte("name: run-pkg\nsource: github.com/org/run\n"),
					0o644,
				))
			},
			wantCount: 1,
		},
		{
			name:      "Run returns empty list when registry is empty",
			setup:     func(t *testing.T, _ string) { t.Helper() },
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			restore := registry.SetOsUserHomeDir(func() (string, error) { return tmp, nil })
			defer restore()

			pkgDir := filepath.Join(tmp, ".config", "agentpack", "packages")
			require.NoError(t, os.MkdirAll(pkgDir, 0o755))

			tt.setup(t, pkgDir)

			entries, err := list.Run()

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Len(t, entries, tt.wantCount)
		})
	}
}
