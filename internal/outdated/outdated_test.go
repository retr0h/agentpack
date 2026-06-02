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

package outdated_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/retr0h/agentpack/internal/outdated"
	outdatedmocks "github.com/retr0h/agentpack/internal/outdated/mocks"
	"github.com/retr0h/agentpack/internal/registry"
	"github.com/retr0h/agentpack/internal/testutil"
)

// --------------------------------------------------------------------------
// TestRun
// --------------------------------------------------------------------------

func TestRun(t *testing.T) {
	// Cannot call t.Parallel() — subtests use t.Setenv.
	tests := []struct {
		name        string
		names       []string
		cancelCtx   bool
		setup       func(t *testing.T, home string)
		wantErr     string
		wantLen     int
		checkResult func(t *testing.T, entries []outdated.Entry)
	}{
		{
			name:    "empty registry returns empty slice",
			names:   nil,
			wantLen: 0,
		},
		{
			name:      "cancelled context returns error",
			names:     nil,
			cancelCtx: true,
			wantErr:   "context canceled",
		},
		{
			name:    "named nonexistent plugin returns error",
			names:   []string{"ghost-plugin"},
			wantErr: "load ghost-plugin",
		},
		{
			name:  "named plugin found in registry produces entry",
			names: []string{"real-plugin"},
			setup: func(t *testing.T, home string) {
				t.Helper()
				dir := filepath.Join(home, ".config", "agentpack", "packages")
				require.NoError(t, os.MkdirAll(dir, 0o755))
				manifest := "name: real-plugin\nsource: /nonexistent/path\nsha: abc123\n"
				p := filepath.Join(dir, "real-plugin.yaml")
				require.NoError(t, os.WriteFile(p, []byte(manifest), 0o644))
			},
			wantLen: 1,
			checkResult: func(t *testing.T, entries []outdated.Entry) {
				t.Helper()
				assert.Equal(t, "real-plugin", entries[0].Name)
			},
		},
		{
			name:  "all plugins via defaultRegistry with installed packages",
			names: nil,
			setup: func(t *testing.T, home string) {
				t.Helper()
				dir := filepath.Join(home, ".config", "agentpack", "packages")
				require.NoError(t, os.MkdirAll(dir, 0o755))
				manifest := "name: installed-plugin\nsource: /nonexistent/path\nsha: abc123\n"
				p := filepath.Join(dir, "installed-plugin.yaml")
				require.NoError(t, os.WriteFile(p, []byte(manifest), 0o644))
			},
			wantLen: 1,
			checkResult: func(t *testing.T, entries []outdated.Entry) {
				t.Helper()
				assert.Equal(t, "installed-plugin", entries[0].Name)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Cannot call t.Parallel() alongside t.Setenv.
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if tt.cancelCtx {
				cancel()
			}

			// Use t.TempDir() as HOME so registry.Load() returns empty.
			home := t.TempDir()
			t.Setenv("HOME", home)

			if tt.setup != nil {
				tt.setup(t, home)
			}

			c := outdated.New()
			entries, err := c.Run(ctx, tt.names)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Len(t, entries, tt.wantLen)

			if tt.checkResult != nil {
				tt.checkResult(t, entries)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestRunWithOptions
// --------------------------------------------------------------------------

func TestRunWithOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		cancelCtx   bool
		customCtx   context.Context
		setupMocks  func(reg *outdatedmocks.MockRegistry, checker *outdatedmocks.MockRemoteChecker)
		extraOpts   func(opts *outdated.Options)
		wantErr     string
		wantLen     int
		checkSteps  func(t *testing.T, steps []string)
		checkResult func(t *testing.T, entries []outdated.Entry)
	}{
		{
			name: "empty registry returns empty slice",
			setupMocks: func(reg *outdatedmocks.MockRegistry, _ *outdatedmocks.MockRemoteChecker) {
				reg.EXPECT().List().Return(nil, nil)
			},
			wantLen: 0,
		},
		{
			name:      "cancelled context returns error before registry call",
			cancelCtx: true,
			setupMocks: func(_ *outdatedmocks.MockRegistry, _ *outdatedmocks.MockRemoteChecker) {
				// no calls expected — context is already done
			},
			wantErr: "context canceled",
		},
		{
			name: "registry list error is propagated",
			setupMocks: func(reg *outdatedmocks.MockRegistry, _ *outdatedmocks.MockRemoteChecker) {
				reg.EXPECT().List().Return(nil, errors.New("registry unavailable"))
			},
			wantErr: "list registry",
		},
		{
			name: "OnStep not called when registry is empty",
			setupMocks: func(reg *outdatedmocks.MockRegistry, _ *outdatedmocks.MockRemoteChecker) {
				reg.EXPECT().List().Return(nil, nil)
			},
			extraOpts: func(_ *outdated.Options) {},
			wantLen:   0,
			checkSteps: func(t *testing.T, steps []string) {
				t.Helper()
				assert.Empty(t, steps)
			},
		},
		{
			name: "up-to-date plugin produces not-outdated entry",
			setupMocks: func(reg *outdatedmocks.MockRegistry, checker *outdatedmocks.MockRemoteChecker) {
				sha := "abc1234567890abcdef"
				reg.EXPECT().List().Return([]*registry.PackageManifest{
					{Name: "my-plugin", Source: "https://example.com/plugin.agentpack", SHA: sha},
				}, nil)
				checker.EXPECT().
					LsRemote(gomock.Any(), "https://example.com/plugin.agentpack").
					Return(map[string]string{"HEAD": sha}, nil)
			},
			wantLen: 1,
			checkResult: func(t *testing.T, entries []outdated.Entry) {
				t.Helper()
				assert.False(t, entries[0].Outdated)
				assert.Equal(t, entries[0].InstalledSHA, entries[0].RemoteSHA)
			},
		},
		{
			name: "outdated plugin produces outdated entry",
			setupMocks: func(reg *outdatedmocks.MockRegistry, checker *outdatedmocks.MockRemoteChecker) {
				reg.EXPECT().List().Return([]*registry.PackageManifest{
					{
						Name:   "my-plugin",
						Source: "https://example.com/plugin.agentpack",
						SHA:    "oldshavalue",
					},
				}, nil)
				checker.EXPECT().
					LsRemote(gomock.Any(), "https://example.com/plugin.agentpack").
					Return(map[string]string{"HEAD": "newsha1234567890abc"}, nil)
			},
			wantLen: 1,
			checkResult: func(t *testing.T, entries []outdated.Entry) {
				t.Helper()
				assert.True(t, entries[0].Outdated)
			},
		},
		{
			name: "ls-remote failure produces non-outdated entry with empty remote SHA",
			setupMocks: func(reg *outdatedmocks.MockRegistry, checker *outdatedmocks.MockRemoteChecker) {
				reg.EXPECT().List().Return([]*registry.PackageManifest{
					{
						Name:   "my-plugin",
						Source: "https://example.com/plugin.agentpack",
						SHA:    "abc123",
					},
				}, nil)
				checker.EXPECT().
					LsRemote(gomock.Any(), "https://example.com/plugin.agentpack").
					Return(nil, errors.New("network error"))
			},
			wantLen: 1,
			checkResult: func(t *testing.T, entries []outdated.Entry) {
				t.Helper()
				assert.Empty(t, entries[0].RemoteSHA)
				assert.False(t, entries[0].Outdated)
			},
		},
		{
			name: "OnStep is called for each manifest",
			setupMocks: func(reg *outdatedmocks.MockRegistry, checker *outdatedmocks.MockRemoteChecker) {
				reg.EXPECT().List().Return([]*registry.PackageManifest{
					{Name: "plugin-a", Source: "https://example.com/a.agentpack", SHA: "sha-a"},
					{Name: "plugin-b", Source: "https://example.com/b.agentpack", SHA: "sha-b"},
				}, nil)
				checker.EXPECT().
					LsRemote(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("unreachable")).
					AnyTimes()
			},
			wantLen: 2,
			checkSteps: func(t *testing.T, steps []string) {
				t.Helper()
				assert.Len(t, steps, 2)
			},
		},
		{
			name: "HEAD not found in remote refs produces non-outdated entry with empty remote SHA",
			setupMocks: func(reg *outdatedmocks.MockRegistry, checker *outdatedmocks.MockRemoteChecker) {
				reg.EXPECT().List().Return([]*registry.PackageManifest{
					{
						Name:   "my-plugin",
						Source: "https://example.com/plugin.agentpack",
						SHA:    "abc123",
					},
				}, nil)
				checker.EXPECT().
					LsRemote(gomock.Any(), "https://example.com/plugin.agentpack").
					Return(map[string]string{"refs/tags/v1.0.0": "deadbeef"}, nil)
			},
			wantLen: 1,
			checkResult: func(t *testing.T, entries []outdated.Entry) {
				t.Helper()
				assert.Empty(t, entries[0].RemoteSHA)
				assert.False(t, entries[0].Outdated)
			},
		},
		{
			name: "refs/heads/main fallback resolves head",
			setupMocks: func(reg *outdatedmocks.MockRegistry, checker *outdatedmocks.MockRemoteChecker) {
				sha := "mainsha1234567890abc"
				reg.EXPECT().List().Return([]*registry.PackageManifest{
					{
						Name:   "my-plugin",
						Source: "https://example.com/plugin.agentpack",
						SHA:    "oldshavalue",
					},
				}, nil)
				checker.EXPECT().
					LsRemote(gomock.Any(), "https://example.com/plugin.agentpack").
					Return(map[string]string{"refs/heads/main": sha}, nil)
			},
			wantLen: 1,
			checkResult: func(t *testing.T, entries []outdated.Entry) {
				t.Helper()
				assert.NotEmpty(t, entries[0].RemoteSHA)
				assert.True(t, entries[0].Outdated)
			},
		},
		{
			name: "refs/heads/master fallback resolves head",
			setupMocks: func(reg *outdatedmocks.MockRegistry, checker *outdatedmocks.MockRemoteChecker) {
				sha := "mastersha123456789ab"
				reg.EXPECT().List().Return([]*registry.PackageManifest{
					{
						Name:   "my-plugin",
						Source: "https://example.com/plugin.agentpack",
						SHA:    "oldshavalue",
					},
				}, nil)
				checker.EXPECT().
					LsRemote(gomock.Any(), "https://example.com/plugin.agentpack").
					Return(map[string]string{"refs/heads/master": sha}, nil)
			},
			wantLen: 1,
			checkResult: func(t *testing.T, entries []outdated.Entry) {
				t.Helper()
				assert.NotEmpty(t, entries[0].RemoteSHA)
				assert.True(t, entries[0].Outdated)
			},
		},
		{
			name: "nil remote checker uses defaultRemoteChecker which fails on unreachable source",
			setupMocks: func(reg *outdatedmocks.MockRegistry, _ *outdatedmocks.MockRemoteChecker) {
				reg.EXPECT().List().Return([]*registry.PackageManifest{
					{Name: "my-plugin", Source: "/nonexistent/path/to/repo", SHA: "abc123"},
				}, nil)
			},
			extraOpts: func(opts *outdated.Options) {
				opts.RemoteChecker = nil
			},
			wantLen: 1,
			checkResult: func(t *testing.T, entries []outdated.Entry) {
				t.Helper()
				assert.Empty(t, entries[0].RemoteSHA)
				assert.False(t, entries[0].Outdated)
			},
		},
		{
			name:      "context cancelled between manifest iterations returns error",
			customCtx: testutil.NewCancelAfterN(1),
			setupMocks: func(reg *outdatedmocks.MockRegistry, _ *outdatedmocks.MockRemoteChecker) {
				reg.EXPECT().List().Return([]*registry.PackageManifest{
					{Name: "plugin-x", Source: "https://example.com/x.agentpack", SHA: "sha-x"},
				}, nil)
			},
			wantErr: "context canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			reg := outdatedmocks.NewMockRegistry(ctrl)
			checker := outdatedmocks.NewMockRemoteChecker(ctrl)

			if tt.setupMocks != nil {
				tt.setupMocks(reg, checker)
			}

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
				ctx, cancel = context.WithCancel(context.Background())
			}

			defer cancel()

			var steps []string
			opts := outdated.Options{
				Registry:      reg,
				RemoteChecker: checker,
			}

			if tt.extraOpts != nil {
				tt.extraOpts(&opts)
			}

			if tt.checkSteps != nil {
				opts.OnStep = func(name string) { steps = append(steps, name) }
			}

			c := outdated.New()
			entries, err := c.RunWithOptions(ctx, opts)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Len(t, entries, tt.wantLen)

			if tt.checkSteps != nil {
				tt.checkSteps(t, steps)
			}

			if tt.checkResult != nil {
				tt.checkResult(t, entries)
			}
		})
	}
}
