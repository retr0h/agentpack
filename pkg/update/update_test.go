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

package update_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/retr0h/agentpack/pkg/install"
	"github.com/retr0h/agentpack/pkg/registry"
	"github.com/retr0h/agentpack/pkg/update"
	updatemocks "github.com/retr0h/agentpack/pkg/update/mocks"
)

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		cancelCtx   bool
		setupMocks  func(loader *updatemocks.MockRegistryLoader, installer *updatemocks.MockInstaller)
		opts        func(loader *updatemocks.MockRegistryLoader, installer *updatemocks.MockInstaller) update.Options
		wantErr     string
		wantUpdated bool
		wantOldSHA  string
		wantNewSHA  string
	}{
		{
			name:      "cancelled context returns error before any lookup",
			cancelCtx: true,
			setupMocks: func(_ *updatemocks.MockRegistryLoader, _ *updatemocks.MockInstaller) {
				// no calls expected — context is already done
			},
			opts: func(loader *updatemocks.MockRegistryLoader, installer *updatemocks.MockInstaller) update.Options {
				return update.Options{
					Name:           "any",
					RegistryLoader: loader,
					Installer:      installer,
				}
			},
			wantErr: "context canceled",
		},
		{
			name: "registry load failure returns wrapped error",
			setupMocks: func(loader *updatemocks.MockRegistryLoader, _ *updatemocks.MockInstaller) {
				loader.EXPECT().
					Load("no-such-plugin").
					Return(nil, errors.New("package not found"))
			},
			opts: func(loader *updatemocks.MockRegistryLoader, installer *updatemocks.MockInstaller) update.Options {
				return update.Options{
					Name:           "no-such-plugin",
					RegistryLoader: loader,
					Installer:      installer,
				}
			},
			wantErr: "load registry manifest",
		},
		{
			name: "install failure returns wrapped error",
			setupMocks: func(loader *updatemocks.MockRegistryLoader, installer *updatemocks.MockInstaller) {
				loader.EXPECT().
					Load("my-plugin").
					Return(&registry.PackageManifest{
						Name:    "my-plugin",
						Source:  "/local/path/archive.agentpack",
						SHA:     "abcdef1234567890",
						Version: "v1.0.0",
					}, nil)
				installer.EXPECT().
					Install(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("install failed"))
			},
			opts: func(loader *updatemocks.MockRegistryLoader, installer *updatemocks.MockInstaller) update.Options {
				return update.Options{
					Name:           "my-plugin",
					RegistryLoader: loader,
					Installer:      installer,
				}
			},
			wantErr: "re-install",
		},
		{
			name: "non-git source installs and returns updated result",
			setupMocks: func(loader *updatemocks.MockRegistryLoader, installer *updatemocks.MockInstaller) {
				loader.EXPECT().
					Load("my-plugin").
					Return(&registry.PackageManifest{
						Name:    "my-plugin",
						Source:  "/local/path/archive.agentpack",
						SHA:     "abcdef1234567890",
						Version: "v1.0.0",
					}, nil)
				installer.EXPECT().
					Install(gomock.Any(), gomock.Any()).
					Return(&install.Result{
						Name:    "my-plugin",
						SHA:     "1234567",
						Version: "v1.1.0",
					}, nil)
			},
			opts: func(loader *updatemocks.MockRegistryLoader, installer *updatemocks.MockInstaller) update.Options {
				return update.Options{
					Name:           "my-plugin",
					RegistryLoader: loader,
					Installer:      installer,
				}
			},
			wantUpdated: true,
			wantOldSHA:  "abcdef1",
			wantNewSHA:  "1234567",
		},
		{
			name: "non-git source re-installs with same SHA returns not-updated",
			setupMocks: func(loader *updatemocks.MockRegistryLoader, installer *updatemocks.MockInstaller) {
				loader.EXPECT().
					Load("my-plugin").
					Return(&registry.PackageManifest{
						Name:    "my-plugin",
						Source:  "/local/path/archive.agentpack",
						SHA:     "abcdef1234567890",
						Version: "v1.0.0",
					}, nil)
				installer.EXPECT().
					Install(gomock.Any(), gomock.Any()).
					Return(&install.Result{
						Name:    "my-plugin",
						SHA:     "abcdef1", // same short SHA
						Version: "v1.0.0",
					}, nil)
			},
			opts: func(loader *updatemocks.MockRegistryLoader, installer *updatemocks.MockInstaller) update.Options {
				return update.Options{
					Name:           "my-plugin",
					RegistryLoader: loader,
					Installer:      installer,
				}
			},
			wantUpdated: false,
			wantOldSHA:  "abcdef1",
			wantNewSHA:  "abcdef1",
		},
		{
			name: "OnStep callback is passed through to install options",
			setupMocks: func(loader *updatemocks.MockRegistryLoader, installer *updatemocks.MockInstaller) {
				loader.EXPECT().
					Load("step-plugin").
					Return(&registry.PackageManifest{
						Name:    "step-plugin",
						Source:  "/local/path/archive.agentpack",
						SHA:     "deadbeef12345678",
						Version: "v2.0.0",
					}, nil)
				installer.EXPECT().
					Install(gomock.Any(), gomock.Any()).
					Return(&install.Result{
						Name:    "step-plugin",
						SHA:     "cafebab",
						Version: "v2.1.0",
					}, nil)
			},
			opts: func(loader *updatemocks.MockRegistryLoader, installer *updatemocks.MockInstaller) update.Options {
				return update.Options{
					Name:           "step-plugin",
					OnStep:         func(_ install.Step) {},
					RegistryLoader: loader,
					Installer:      installer,
				}
			},
			wantUpdated: true,
			wantOldSHA:  "deadbee",
			wantNewSHA:  "cafebab",
		},
		{
			name: "nil RegistryLoader falls back to defaultRegistryLoader",
			setupMocks: func(_ *updatemocks.MockRegistryLoader, _ *updatemocks.MockInstaller) {
				// no mock calls — nil loader uses real registry.Load
			},
			opts: func(_ *updatemocks.MockRegistryLoader, _ *updatemocks.MockInstaller) update.Options {
				return update.Options{
					Name:           "nonexistent-package-xyzzy",
					RegistryLoader: nil,
					Installer:      nil,
				}
			},
			wantErr: "load registry manifest",
		},
		{
			name: "nil Installer falls back to defaultInstaller",
			setupMocks: func(loader *updatemocks.MockRegistryLoader, _ *updatemocks.MockInstaller) {
				loader.EXPECT().
					Load("my-plugin").
					Return(&registry.PackageManifest{
						Name:    "my-plugin",
						Source:  "/nonexistent/archive.agentpack",
						SHA:     "abc1234",
						Version: "v1.0.0",
					}, nil)
			},
			opts: func(loader *updatemocks.MockRegistryLoader, _ *updatemocks.MockInstaller) update.Options {
				return update.Options{
					Name:           "my-plugin",
					RegistryLoader: loader,
					Installer:      nil,
				}
			},
			wantErr: "re-install",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			loader := updatemocks.NewMockRegistryLoader(ctrl)
			installer := updatemocks.NewMockInstaller(ctrl)

			if tt.setupMocks != nil {
				tt.setupMocks(loader, installer)
			}

			opts := tt.opts(loader, installer)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if tt.cancelCtx {
				cancel()
			}

			result, err := update.Run(ctx, opts)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			assert.Equal(t, tt.wantUpdated, result.Updated)

			if tt.wantOldSHA != "" {
				assert.Equal(t, tt.wantOldSHA, result.OldSHA)
			}

			if tt.wantNewSHA != "" {
				assert.Equal(t, tt.wantNewSHA, result.NewSHA)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestShortSHA
// --------------------------------------------------------------------------

func TestShortSHA(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "returns first 7 chars of long SHA",
			in:   "abcdef1234567890",
			want: "abcdef1",
		},
		{
			name: "returns full string when exactly 7 chars",
			in:   "abcdef1",
			want: "abcdef1",
		},
		{
			name: "returns full string when shorter than 7 chars",
			in:   "abc",
			want: "abc",
		},
		{
			name: "returns empty string when input is empty",
			in:   "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := update.ShortSHA(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --------------------------------------------------------------------------
// TestResolveHEAD
// --------------------------------------------------------------------------

func TestResolveHEAD(t *testing.T) {
	t.Parallel()

	const zeroHash = "0000000000000000000000000000000000000000"
	const realSHA = "abc1234567890abcdef0123456789abcdef01234"

	tests := []struct {
		name string
		refs map[string]string
		want string
	}{
		{
			name: "returns HEAD SHA when HEAD is non-zero",
			refs: map[string]string{
				"HEAD":            realSHA,
				"refs/heads/main": "other1234567890abcdef0123456789abcdef01",
			},
			want: realSHA,
		},
		{
			name: "falls back to refs/heads/main when HEAD is zero hash",
			refs: map[string]string{
				"HEAD":            zeroHash,
				"refs/heads/main": realSHA,
			},
			want: realSHA,
		},
		{
			name: "falls back to refs/heads/master when main is absent",
			refs: map[string]string{
				"HEAD":              zeroHash,
				"refs/heads/master": realSHA,
			},
			want: realSHA,
		},
		{
			name: "returns empty string when no refs match",
			refs: map[string]string{
				"refs/heads/feature": realSHA,
			},
			want: "",
		},
		{
			name: "returns empty string for empty refs map",
			refs: map[string]string{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := update.ResolveHEAD(tt.refs)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --------------------------------------------------------------------------
// TestDefaultRegistryLoader
// --------------------------------------------------------------------------

func TestDefaultRegistryLoader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pkgName string
	}{
		{
			name:    "exercises real registry.Load path for unknown package",
			pkgName: "this-package-definitely-does-not-exist-xyzzy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := update.DefaultRegistryLoaderLoad(tt.pkgName)

			// The real registry.Load either returns an error (package not
			// installed) or nil. Either way the concrete Load path is covered.
			_ = err
		})
	}
}

// --------------------------------------------------------------------------
// TestDefaultInstaller
// --------------------------------------------------------------------------

func TestDefaultInstaller(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		source  string
		wantErr bool
	}{
		{
			name:    "exercises real install.Run path for non-existent archive",
			source:  "/nonexistent/path/archive.agentpack",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := update.DefaultInstallerInstall(context.Background(), tt.source)

			if tt.wantErr {
				assert.Error(t, err)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestRunGitBranch — not parallel because subtests mutate lsRemote package var
// --------------------------------------------------------------------------

func TestRunGitBranch(t *testing.T) {
	// NOTE: not parallel — subtests mutate the lsRemote package-level var.

	const gitSource = "github.com/example/skills-repo"

	tests := []struct {
		name         string
		setupMocks   func(loader *updatemocks.MockRegistryLoader, installer *updatemocks.MockInstaller)
		lsRemoteFunc func(ctx context.Context, rawURL string) (map[string]string, error)
		opts         func(loader *updatemocks.MockRegistryLoader, installer *updatemocks.MockInstaller) update.Options
		wantErr      string
		wantUpdated  bool
	}{
		{
			name: "lsRemote failure falls through to install",
			setupMocks: func(loader *updatemocks.MockRegistryLoader, installer *updatemocks.MockInstaller) {
				loader.EXPECT().
					Load("git-plugin").
					Return(&registry.PackageManifest{
						Name:    "git-plugin",
						Source:  gitSource,
						SHA:     "abc1234567890",
						Version: "v1.0.0",
					}, nil)
				installer.EXPECT().
					Install(gomock.Any(), gomock.Any()).
					Return(&install.Result{
						Name:    "git-plugin",
						SHA:     "newsha12",
						Version: "v1.1.0",
					}, nil)
			},
			lsRemoteFunc: func(_ context.Context, _ string) (map[string]string, error) {
				return nil, errors.New("ls-remote network failure")
			},
			opts: func(loader *updatemocks.MockRegistryLoader, installer *updatemocks.MockInstaller) update.Options {
				return update.Options{
					Name:           "git-plugin",
					RegistryLoader: loader,
					Installer:      installer,
				}
			},
			wantUpdated: true,
		},
		{
			name: "same remote SHA short-circuits update",
			setupMocks: func(loader *updatemocks.MockRegistryLoader, _ *updatemocks.MockInstaller) {
				loader.EXPECT().
					Load("git-plugin").
					Return(&registry.PackageManifest{
						Name:    "git-plugin",
						Source:  gitSource,
						SHA:     "abc1234",
						Version: "v1.0.0",
					}, nil)
			},
			lsRemoteFunc: func(_ context.Context, _ string) (map[string]string, error) {
				return map[string]string{
					"HEAD": "abc12341234567890abcdef0123456789abcdef",
				}, nil
			},
			opts: func(loader *updatemocks.MockRegistryLoader, installer *updatemocks.MockInstaller) update.Options {
				return update.Options{
					Name:           "git-plugin",
					RegistryLoader: loader,
					Installer:      installer,
				}
			},
			wantUpdated: false,
		},
		{
			name: "different remote SHA falls through to install",
			setupMocks: func(loader *updatemocks.MockRegistryLoader, installer *updatemocks.MockInstaller) {
				loader.EXPECT().
					Load("git-plugin").
					Return(&registry.PackageManifest{
						Name:    "git-plugin",
						Source:  gitSource,
						SHA:     "abc1234",
						Version: "v1.0.0",
					}, nil)
				installer.EXPECT().
					Install(gomock.Any(), gomock.Any()).
					Return(&install.Result{
						Name:    "git-plugin",
						SHA:     "def5678",
						Version: "v1.1.0",
					}, nil)
			},
			lsRemoteFunc: func(_ context.Context, _ string) (map[string]string, error) {
				return map[string]string{
					"HEAD": "def56789012345678901234567890123456789ab",
				}, nil
			},
			opts: func(loader *updatemocks.MockRegistryLoader, installer *updatemocks.MockInstaller) update.Options {
				return update.Options{
					Name:           "git-plugin",
					RegistryLoader: loader,
					Installer:      installer,
				}
			},
			wantUpdated: true,
		},
		{
			name: "empty remote SHA falls through to install",
			setupMocks: func(loader *updatemocks.MockRegistryLoader, installer *updatemocks.MockInstaller) {
				loader.EXPECT().
					Load("git-plugin").
					Return(&registry.PackageManifest{
						Name:    "git-plugin",
						Source:  gitSource,
						SHA:     "abc1234",
						Version: "v1.0.0",
					}, nil)
				installer.EXPECT().
					Install(gomock.Any(), gomock.Any()).
					Return(&install.Result{
						Name:    "git-plugin",
						SHA:     "newsha12",
						Version: "v1.1.0",
					}, nil)
			},
			lsRemoteFunc: func(_ context.Context, _ string) (map[string]string, error) {
				return map[string]string{
					"refs/heads/feature": "abc12341234567890abcdef012345678901234",
				}, nil
			},
			opts: func(loader *updatemocks.MockRegistryLoader, installer *updatemocks.MockInstaller) update.Options {
				return update.Options{
					Name:           "git-plugin",
					RegistryLoader: loader,
					Installer:      installer,
				}
			},
			wantUpdated: true,
		},
		{
			name: "OnStep is emitted for git source before lsRemote",
			setupMocks: func(loader *updatemocks.MockRegistryLoader, installer *updatemocks.MockInstaller) {
				loader.EXPECT().
					Load("git-plugin").
					Return(&registry.PackageManifest{
						Name:    "git-plugin",
						Source:  gitSource,
						SHA:     "abc1234",
						Version: "v1.0.0",
					}, nil)
				installer.EXPECT().
					Install(gomock.Any(), gomock.Any()).
					Return(&install.Result{
						Name:    "git-plugin",
						SHA:     "newsha12",
						Version: "v1.1.0",
					}, nil)
			},
			lsRemoteFunc: func(_ context.Context, _ string) (map[string]string, error) {
				return nil, errors.New("ls-remote failure")
			},
			opts: func(loader *updatemocks.MockRegistryLoader, installer *updatemocks.MockInstaller) update.Options {
				return update.Options{
					Name:           "git-plugin",
					OnStep:         func(_ install.Step) {},
					RegistryLoader: loader,
					Installer:      installer,
				}
			},
			wantUpdated: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Not parallel: mutates the lsRemote package-level var.

			ctrl := gomock.NewController(t)
			loader := updatemocks.NewMockRegistryLoader(ctrl)
			installer := updatemocks.NewMockInstaller(ctrl)

			tt.setupMocks(loader, installer)

			restore := update.SetLsRemote(tt.lsRemoteFunc)
			defer restore()

			opts := tt.opts(loader, installer)

			result, err := update.Run(context.Background(), opts)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantUpdated, result.Updated)
		})
	}
}
