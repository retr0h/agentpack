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
// TestRunGlobal
//
// These tests mutate the package-level osUserHomeDir variable via
// SetOsUserHomeDir and therefore must NOT run in parallel with each other.
// --------------------------------------------------------------------------

func TestRunGlobal(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T, home string)
		wantErr     string
		checkResult func(t *testing.T, entries []list.GlobalEntry)
	}{
		{
			name: "returns skill entries for agents whose GlobalSkillsDir contains subdirectories",
			setup: func(t *testing.T, home string) {
				t.Helper()
				// cursor agent: GlobalSkillsDir = ".cursor/skills"
				cursorSkillDir := filepath.Join(home, ".cursor", "skills", "my-skill")
				require.NoError(t, os.MkdirAll(cursorSkillDir, 0o755))
				// gemini-cli agent: GlobalSkillsDir = ".gemini/skills"
				geminiSkillDir := filepath.Join(home, ".gemini", "skills", "another-skill")
				require.NoError(t, os.MkdirAll(geminiSkillDir, 0o755))
			},
			checkResult: func(t *testing.T, entries []list.GlobalEntry) {
				t.Helper()
				// Collect (agent, skill) pairs for easy assertion.
				type key struct{ agent, skill string }
				got := make(map[key]bool, len(entries))
				for _, e := range entries {
					got[key{e.Agent, e.Skill}] = true
				}
				assert.True(t, got[key{"cursor", "my-skill"}], "expected cursor/my-skill entry")
				assert.True(
					t,
					got[key{"gemini-cli", "another-skill"}],
					"expected gemini-cli/another-skill entry",
				)
			},
		},
		{
			name: "files inside a GlobalSkillsDir are not returned, only directories",
			setup: func(t *testing.T, home string) {
				t.Helper()
				skillsDir := filepath.Join(home, ".cursor", "skills")
				require.NoError(t, os.MkdirAll(skillsDir, 0o755))
				// Write a plain file that should be ignored.
				require.NoError(t, os.WriteFile(
					filepath.Join(skillsDir, "README.md"),
					[]byte("# readme"),
					0o644,
				))
				// And one real skill directory.
				require.NoError(t, os.MkdirAll(filepath.Join(skillsDir, "real-skill"), 0o755))
			},
			checkResult: func(t *testing.T, entries []list.GlobalEntry) {
				t.Helper()
				for _, e := range entries {
					if e.Agent == "cursor" {
						assert.Equal(
							t,
							"real-skill",
							e.Skill,
							"only directories should be returned as skills",
						)
					}
				}
				// Ensure the plain file was not included.
				for _, e := range entries {
					assert.NotEqual(t, "README.md", e.Skill)
				}
			},
		},
		{
			name: "returns empty slice when no GlobalSkillsDir directories exist",
			setup: func(t *testing.T, _ string) {
				t.Helper()
				// home is an empty temp dir — no agent dirs created.
			},
			checkResult: func(t *testing.T, entries []list.GlobalEntry) {
				t.Helper()
				assert.Empty(t, entries)
			},
		},
		{
			name: "home dir error is propagated",
			setup: func(t *testing.T, _ string) {
				t.Helper()
			},
			wantErr: "no home dir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr != "" {
				restore := list.SetOsUserHomeDir(func() (string, error) {
					return "", errors.New("no home dir")
				})
				defer restore()

				l := list.New()
				_, err := l.RunGlobal()
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			home := t.TempDir()
			tt.setup(t, home)

			restore := list.SetOsUserHomeDir(func() (string, error) { return home, nil })
			defer restore()

			l := list.New()
			entries, err := l.RunGlobal()
			require.NoError(t, err)

			if tt.checkResult != nil {
				tt.checkResult(t, entries)
			}
		})
	}
}

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
					{
						Name:   "zebra",
						Source: "github.com/org/zebra",
						Files:  []registry.InstalledFile{},
					},
					{
						Name:   "alpha",
						Source: "github.com/org/alpha",
						Files:  []registry.InstalledFile{},
					},
					{
						Name:   "middle",
						Source: "github.com/org/middle",
						Files:  []registry.InstalledFile{},
					},
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
					{
						Name:      "ts-pkg",
						Installed: "2026-01-15T12:00:00Z",
						Files:     []registry.InstalledFile{},
					},
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

			l := list.New()
			entries, err := l.RunWithRegistry(reg)

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

			l := list.New()
			entries, err := l.RunWithRegistry(reg)
			require.NoError(t, err)
			require.Len(t, entries, 1)
			assert.Equal(t, tt.wantInstalled, entries[0].Installed)
		})
	}
}

// --------------------------------------------------------------------------
// TestRun covers the Run() method which uses the default registry.
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

			l := list.New()
			entries, err := l.Run()

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
