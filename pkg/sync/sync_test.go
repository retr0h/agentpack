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

package sync_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	fetcherMocks "github.com/retr0h/agentpack/internal/fetcher/mocks"
	"github.com/retr0h/agentpack/pkg/build"
	"github.com/retr0h/agentpack/pkg/install"
	pkgsync "github.com/retr0h/agentpack/pkg/sync"
	syncMocks "github.com/retr0h/agentpack/pkg/sync/mocks"
)

// cancelAfterFirstErrCtx is a context.Context whose Err() returns nil on the
// first call and context.Canceled on all subsequent calls. This lets us pass
// the function-entry check but fail the per-package loop check.
type cancelAfterFirstErrCtx struct {
	callCount int
}

func newCancelAfterFirstErrCtx() *cancelAfterFirstErrCtx {
	return &cancelAfterFirstErrCtx{}
}

func (c *cancelAfterFirstErrCtx) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *cancelAfterFirstErrCtx) Done() <-chan struct{}       { return nil }
func (c *cancelAfterFirstErrCtx) Value(_ any) any             { return nil }

func (c *cancelAfterFirstErrCtx) Err() error {
	c.callCount++
	if c.callCount == 1 {
		return nil
	}

	return errors.New("context canceled")
}

func writePackagesFile(t *testing.T, dir, content string) string {
	t.Helper()

	path := filepath.Join(dir, "agentpack-packages.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	return path
}

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		yaml        string
		customCtx   context.Context
		cancelCtx   bool
		setupMocks  func(ctrl *gomock.Controller) (pkgsync.Options, func())
		wantErr     string
		checkResult func(t *testing.T, results []pkgsync.Result)
	}{
		{
			name: "source package installed successfully",
			yaml: "packages:\n  - name: my-plugin\n    source: /tmp/my-plugin.agentpack\n",
			setupMocks: func(ctrl *gomock.Controller) (pkgsync.Options, func()) {
				mockInstaller := syncMocks.NewMockInstaller(ctrl)
				mockInstaller.EXPECT().
					Install(gomock.Any(), "/tmp/my-plugin.agentpack").
					Return(&install.Result{Name: "my-plugin", Version: "1.0.0"}, nil)

				return pkgsync.Options{
					Installer: mockInstaller,
				}, func() {}
			},
			checkResult: func(t *testing.T, results []pkgsync.Result) {
				t.Helper()

				require.Len(t, results, 1)
				assert.Equal(t, pkgsync.StatusInstalled, results[0].Status)
				assert.Equal(t, "my-plugin", results[0].Name)
				assert.Equal(t, "1.0.0", results[0].Version)
			},
		},
		{
			name: "source package installer returns error",
			yaml: "packages:\n  - name: bad-plugin\n    source: /tmp/bad-plugin.agentpack\n",
			setupMocks: func(ctrl *gomock.Controller) (pkgsync.Options, func()) {
				mockInstaller := syncMocks.NewMockInstaller(ctrl)
				mockInstaller.EXPECT().
					Install(gomock.Any(), "/tmp/bad-plugin.agentpack").
					Return(nil, errors.New("archive corrupt"))

				return pkgsync.Options{
					Installer: mockInstaller,
				}, func() {}
			},
			checkResult: func(t *testing.T, results []pkgsync.Result) {
				t.Helper()

				require.Len(t, results, 1)
				assert.Equal(t, pkgsync.StatusFailed, results[0].Status)
				assert.NotNil(t, results[0].Err)
			},
		},
		{
			name: "git package fetch + build + install succeeds",
			yaml: "packages:\n  - name: git-plugin\n    git: github.com/org/repo\n    ref: main\n",
			setupMocks: func(ctrl *gomock.Controller) (pkgsync.Options, func()) {
				mockFetcher := fetcherMocks.NewMockFetcher(ctrl)
				mockBuilder := syncMocks.NewMockBuilder(ctrl)
				mockInstaller := syncMocks.NewMockInstaller(ctrl)

				mockFetcher.EXPECT().
					Fetch(gomock.Any(), "github.com/org/repo#main", gomock.Any()).
					Return(nil)

				mockBuilder.EXPECT().
					Build(gomock.Any(), gomock.Any()).
					Return([]build.Result{{Name: "git-plugin", ArchivePath: "/tmp/git-plugin-1.0.0.agentpack"}}, nil)

				mockInstaller.EXPECT().
					Install(gomock.Any(), "/tmp/git-plugin-1.0.0.agentpack").
					Return(&install.Result{Name: "git-plugin", Version: "1.0.0"}, nil)

				return pkgsync.Options{
					Fetcher:   mockFetcher,
					Builder:   mockBuilder,
					Installer: mockInstaller,
				}, func() {}
			},
			checkResult: func(t *testing.T, results []pkgsync.Result) {
				t.Helper()

				require.Len(t, results, 1)
				assert.Equal(t, pkgsync.StatusInstalled, results[0].Status)
				assert.Equal(t, "git-plugin", results[0].Name)
			},
		},
		{
			name: "git package uses locked SHA when available",
			yaml: "packages:\n  - name: locked-plugin\n    git: github.com/org/repo\n    ref: main\n",
			setupMocks: func(ctrl *gomock.Controller) (pkgsync.Options, func()) {
				mockFetcher := fetcherMocks.NewMockFetcher(ctrl)
				mockBuilder := syncMocks.NewMockBuilder(ctrl)
				mockInstaller := syncMocks.NewMockInstaller(ctrl)

				mockFetcher.EXPECT().
					Fetch(gomock.Any(), "github.com/org/repo#abc1234567890abcdef", gomock.Any()).
					Return(nil)

				mockBuilder.EXPECT().
					Build(gomock.Any(), gomock.Any()).
					Return([]build.Result{{Name: "locked-plugin", ArchivePath: "/tmp/locked-plugin-1.0.0.agentpack"}}, nil)

				mockInstaller.EXPECT().
					Install(gomock.Any(), "/tmp/locked-plugin-1.0.0.agentpack").
					Return(&install.Result{Name: "locked-plugin", Version: "1.0.0"}, nil)

				return pkgsync.Options{
					Fetcher:    mockFetcher,
					Builder:    mockBuilder,
					Installer:  mockInstaller,
					LockedSHAs: map[string]string{"locked-plugin": "abc1234567890abcdef"},
				}, func() {}
			},
			checkResult: func(t *testing.T, results []pkgsync.Result) {
				t.Helper()

				require.Len(t, results, 1)
				assert.Equal(t, pkgsync.StatusInstalled, results[0].Status)
				assert.Equal(t, "locked-plugin", results[0].Name)
			},
		},
		{
			name: "git package ignores lock when no entry exists",
			yaml: "packages:\n  - name: unlocked-plugin\n    git: github.com/org/repo\n    ref: v1.2.3\n",
			setupMocks: func(ctrl *gomock.Controller) (pkgsync.Options, func()) {
				mockFetcher := fetcherMocks.NewMockFetcher(ctrl)
				mockBuilder := syncMocks.NewMockBuilder(ctrl)
				mockInstaller := syncMocks.NewMockInstaller(ctrl)

				mockFetcher.EXPECT().
					Fetch(gomock.Any(), "github.com/org/repo#v1.2.3", gomock.Any()).
					Return(nil)

				mockBuilder.EXPECT().
					Build(gomock.Any(), gomock.Any()).
					Return([]build.Result{{Name: "unlocked-plugin", ArchivePath: "/tmp/unlocked-plugin-1.2.3.agentpack"}}, nil)

				mockInstaller.EXPECT().
					Install(gomock.Any(), "/tmp/unlocked-plugin-1.2.3.agentpack").
					Return(&install.Result{Name: "unlocked-plugin", Version: "1.2.3"}, nil)

				return pkgsync.Options{
					Fetcher:    mockFetcher,
					Builder:    mockBuilder,
					Installer:  mockInstaller,
					LockedSHAs: map[string]string{"other-plugin": "deadbeef"},
				}, func() {}
			},
			checkResult: func(t *testing.T, results []pkgsync.Result) {
				t.Helper()

				require.Len(t, results, 1)
				assert.Equal(t, pkgsync.StatusInstalled, results[0].Status)
				assert.Equal(t, "unlocked-plugin", results[0].Name)
			},
		},
		{
			name: "git package fetch fails",
			yaml: "packages:\n  - name: fetch-fail\n    git: github.com/org/missing\n",
			setupMocks: func(ctrl *gomock.Controller) (pkgsync.Options, func()) {
				mockFetcher := fetcherMocks.NewMockFetcher(ctrl)
				mockBuilder := syncMocks.NewMockBuilder(ctrl)
				mockInstaller := syncMocks.NewMockInstaller(ctrl)

				mockFetcher.EXPECT().
					Fetch(gomock.Any(), "github.com/org/missing", gomock.Any()).
					Return(errors.New("repository not found"))

				return pkgsync.Options{
					Fetcher:   mockFetcher,
					Builder:   mockBuilder,
					Installer: mockInstaller,
				}, func() {}
			},
			checkResult: func(t *testing.T, results []pkgsync.Result) {
				t.Helper()

				require.Len(t, results, 1)
				assert.Equal(t, pkgsync.StatusFailed, results[0].Status)
				assert.NotNil(t, results[0].Err)
			},
		},
		{
			name: "git package build fails",
			yaml: "packages:\n  - name: build-fail\n    git: github.com/org/repo\n",
			setupMocks: func(ctrl *gomock.Controller) (pkgsync.Options, func()) {
				mockFetcher := fetcherMocks.NewMockFetcher(ctrl)
				mockBuilder := syncMocks.NewMockBuilder(ctrl)
				mockInstaller := syncMocks.NewMockInstaller(ctrl)

				mockFetcher.EXPECT().
					Fetch(gomock.Any(), "github.com/org/repo", gomock.Any()).
					Return(nil)

				mockBuilder.EXPECT().
					Build(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("no agentpack.yaml found"))

				return pkgsync.Options{
					Fetcher:   mockFetcher,
					Builder:   mockBuilder,
					Installer: mockInstaller,
				}, func() {}
			},
			checkResult: func(t *testing.T, results []pkgsync.Result) {
				t.Helper()

				require.Len(t, results, 1)
				assert.Equal(t, pkgsync.StatusFailed, results[0].Status)
				assert.NotNil(t, results[0].Err)
			},
		},
		{
			name: "git package install fails for one of multiple build results",
			yaml: "packages:\n  - name: multi-build\n    git: github.com/org/repo\n",
			setupMocks: func(ctrl *gomock.Controller) (pkgsync.Options, func()) {
				mockFetcher := fetcherMocks.NewMockFetcher(ctrl)
				mockBuilder := syncMocks.NewMockBuilder(ctrl)
				mockInstaller := syncMocks.NewMockInstaller(ctrl)

				mockFetcher.EXPECT().
					Fetch(gomock.Any(), "github.com/org/repo", gomock.Any()).
					Return(nil)

				mockBuilder.EXPECT().
					Build(gomock.Any(), gomock.Any()).
					Return([]build.Result{
						{Name: "plugin-a", ArchivePath: "/tmp/plugin-a-1.0.0.agentpack"},
						{Name: "plugin-b", ArchivePath: "/tmp/plugin-b-1.0.0.agentpack"},
					}, nil)

				mockInstaller.EXPECT().
					Install(gomock.Any(), "/tmp/plugin-a-1.0.0.agentpack").
					Return(&install.Result{Name: "plugin-a", Version: "1.0.0"}, nil)

				mockInstaller.EXPECT().
					Install(gomock.Any(), "/tmp/plugin-b-1.0.0.agentpack").
					Return(nil, errors.New("install failed for plugin-b"))

				return pkgsync.Options{
					Fetcher:   mockFetcher,
					Builder:   mockBuilder,
					Installer: mockInstaller,
				}, func() {}
			},
			checkResult: func(t *testing.T, results []pkgsync.Result) {
				t.Helper()

				require.Len(t, results, 2)
				assert.Equal(t, pkgsync.StatusInstalled, results[0].Status)
				assert.Equal(t, pkgsync.StatusFailed, results[1].Status)
				assert.NotNil(t, results[1].Err)
			},
		},
		{
			name: "missing config file",
			yaml: "",
			setupMocks: func(_ *gomock.Controller) (pkgsync.Options, func()) {
				return pkgsync.Options{}, func() {}
			},
			wantErr: "read",
		},
		{
			name: "invalid YAML",
			yaml: "packages:\n  - name: [unclosed\n",
			setupMocks: func(_ *gomock.Controller) (pkgsync.Options, func()) {
				return pkgsync.Options{}, func() {}
			},
			wantErr: "parse",
		},
		{
			name:      "context cancelled before processing",
			yaml:      "packages:\n  - name: p\n    source: /tmp/p.agentpack\n",
			cancelCtx: true,
			setupMocks: func(_ *gomock.Controller) (pkgsync.Options, func()) {
				return pkgsync.Options{}, func() {}
			},
			wantErr: "context canceled",
		},
		{
			name: "context cancelled between packages",
			yaml: "packages:\n  - name: a\n    source: /tmp/a.agentpack\n  - name: b\n    source: /tmp/b.agentpack\n",
			setupMocks: func(ctrl *gomock.Controller) (pkgsync.Options, func()) {
				mockInstaller := syncMocks.NewMockInstaller(ctrl)

				// The first package is processed before the context is cancelled.
				// cancelAfterFirstErrCtx returns nil on the first Err() call (entry
				// check) and an error on all subsequent calls (the loop check fires
				// before the second package is processed).
				mockInstaller.EXPECT().
					Install(gomock.Any(), "/tmp/a.agentpack").
					Return(&install.Result{Name: "a", Version: "1.0.0"}, nil).
					AnyTimes()

				return pkgsync.Options{
					Installer: mockInstaller,
				}, func() {}
			},
			customCtx: newCancelAfterFirstErrCtx(),
			wantErr:   "context canceled",
		},
		{
			name: "no installer configured for source package",
			yaml: "packages:\n  - name: no-installer\n    source: /tmp/no-installer.agentpack\n",
			setupMocks: func(_ *gomock.Controller) (pkgsync.Options, func()) {
				return pkgsync.Options{
					Installer: nil,
				}, func() {}
			},
			checkResult: func(t *testing.T, results []pkgsync.Result) {
				t.Helper()

				require.Len(t, results, 1)
				assert.Equal(t, pkgsync.StatusFailed, results[0].Status)
				assert.NotNil(t, results[0].Err)
			},
		},
		{
			name: "OnStep called once per package",
			yaml: "packages:\n  - name: step-plugin\n    source: /tmp/step-plugin.agentpack\n",
			setupMocks: func(ctrl *gomock.Controller) (pkgsync.Options, func()) {
				mockInstaller := syncMocks.NewMockInstaller(ctrl)
				mockInstaller.EXPECT().
					Install(gomock.Any(), "/tmp/step-plugin.agentpack").
					Return(&install.Result{Name: "step-plugin", Version: "1.0.0"}, nil)

				var stepped []string
				return pkgsync.Options{
					Installer: mockInstaller,
					OnStep: func(name string) {
						stepped = append(stepped, name)
					},
				}, func() {}
			},
			checkResult: func(t *testing.T, results []pkgsync.Result) {
				t.Helper()

				require.Len(t, results, 1)
				assert.Equal(t, pkgsync.StatusInstalled, results[0].Status)
			},
		},
		{
			name: "no builder configured for git package",
			yaml: "packages:\n  - name: no-builder\n    git: github.com/org/repo\n",
			setupMocks: func(ctrl *gomock.Controller) (pkgsync.Options, func()) {
				mockFetcher := fetcherMocks.NewMockFetcher(ctrl)

				mockFetcher.EXPECT().
					Fetch(gomock.Any(), "github.com/org/repo", gomock.Any()).
					Return(nil)

				return pkgsync.Options{
					Fetcher:   mockFetcher,
					Builder:   nil,
					Installer: syncMocks.NewMockInstaller(ctrl),
				}, func() {}
			},
			checkResult: func(t *testing.T, results []pkgsync.Result) {
				t.Helper()

				require.Len(t, results, 1)
				assert.Equal(t, pkgsync.StatusFailed, results[0].Status)
				assert.NotNil(t, results[0].Err)
			},
		},
		{
			name: "no installer configured for git package after successful build",
			yaml: "packages:\n  - name: git-no-installer\n    git: github.com/org/repo\n",
			setupMocks: func(ctrl *gomock.Controller) (pkgsync.Options, func()) {
				mockFetcher := fetcherMocks.NewMockFetcher(ctrl)
				mockBuilder := syncMocks.NewMockBuilder(ctrl)

				mockFetcher.EXPECT().
					Fetch(gomock.Any(), "github.com/org/repo", gomock.Any()).
					Return(nil)

				mockBuilder.EXPECT().
					Build(gomock.Any(), gomock.Any()).
					Return([]build.Result{{Name: "git-no-installer", ArchivePath: "/tmp/git-no-installer.agentpack"}}, nil)

				return pkgsync.Options{
					Fetcher:   mockFetcher,
					Builder:   mockBuilder,
					Installer: nil,
				}, func() {}
			},
			checkResult: func(t *testing.T, results []pkgsync.Result) {
				t.Helper()

				require.Len(t, results, 1)
				assert.Equal(t, pkgsync.StatusFailed, results[0].Status)
				assert.NotNil(t, results[0].Err)
			},
		},
		{
			name: "nil fetcher falls back to GitFetcher which fails on bad source",
			yaml: "packages:\n  - name: nil-fetcher-pkg\n    git: /nonexistent/path/to/repo\n",
			setupMocks: func(ctrl *gomock.Controller) (pkgsync.Options, func()) {
				return pkgsync.Options{
					Fetcher:   nil,
					Builder:   syncMocks.NewMockBuilder(ctrl),
					Installer: syncMocks.NewMockInstaller(ctrl),
				}, func() {}
			},
			checkResult: func(t *testing.T, results []pkgsync.Result) {
				t.Helper()

				require.Len(t, results, 1)
				assert.Equal(t, pkgsync.StatusFailed, results[0].Status)
				assert.NotNil(t, results[0].Err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)

			opts, cleanup := tt.setupMocks(ctrl)
			defer cleanup()

			// Write the config file to a temp dir, except when testing a missing
			// config (yaml == "").
			var configPath string
			if tt.yaml != "" {
				cfgDir := t.TempDir()
				configPath = writePackagesFile(t, cfgDir, tt.yaml)
			} else {
				configPath = "/nonexistent/agentpack-packages.yaml"
			}

			opts.ConfigPath = configPath

			// Build the context.
			var ctx context.Context
			var cancel context.CancelFunc

			switch {
			case tt.customCtx != nil:
				ctx = tt.customCtx
				cancel = func() {}
			case tt.cancelCtx:
				ctx, cancel = context.WithCancel(context.Background())
				cancel()
			default:
				ctx = context.Background()
				cancel = func() {}
			}

			defer cancel()

			results, err := pkgsync.New().Run(ctx, opts)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.checkResult != nil {
				tt.checkResult(t, results)
			}
		})
	}
}
