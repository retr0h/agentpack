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

			if result.Updated != tt.wantUpdated {
				t.Errorf("Updated = %v, want %v", result.Updated, tt.wantUpdated)
			}

			if tt.wantOldSHA != "" && result.OldSHA != tt.wantOldSHA {
				t.Errorf("OldSHA = %q, want %q", result.OldSHA, tt.wantOldSHA)
			}

			if tt.wantNewSHA != "" && result.NewSHA != tt.wantNewSHA {
				t.Errorf("NewSHA = %q, want %q", result.NewSHA, tt.wantNewSHA)
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
