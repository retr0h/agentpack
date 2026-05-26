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
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/retr0h/agentpack/pkg/outdated"
	outdatedmocks "github.com/retr0h/agentpack/pkg/outdated/mocks"
	"github.com/retr0h/agentpack/pkg/registry"
)

// --------------------------------------------------------------------------
// TestRun
// --------------------------------------------------------------------------

func TestRun(t *testing.T) {
	// Cannot call t.Parallel() — subtests use t.Setenv.
	tests := []struct {
		name      string
		names     []string
		cancelCtx bool
		wantErr   string
		wantLen   int
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
			t.Setenv("HOME", t.TempDir())

			entries, err := outdated.Run(ctx, tt.names)

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

			if len(entries) != tt.wantLen {
				t.Errorf("len = %d, want %d", len(entries), tt.wantLen)
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
			extraOpts: func(opts *outdated.Options) {
				// OnStep will be set by checkSteps wrapper
			},
			wantLen: 0,
			checkSteps: func(t *testing.T, steps []string) {
				t.Helper()
				if len(steps) != 0 {
					t.Errorf("expected 0 steps, got %d", len(steps))
				}
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
				if entries[0].Outdated {
					t.Errorf("expected Outdated=false, got true")
				}
				if entries[0].InstalledSHA != entries[0].RemoteSHA {
					t.Errorf("expected InstalledSHA == RemoteSHA")
				}
			},
		},
		{
			name: "outdated plugin produces outdated entry",
			setupMocks: func(reg *outdatedmocks.MockRegistry, checker *outdatedmocks.MockRemoteChecker) {
				reg.EXPECT().List().Return([]*registry.PackageManifest{
					{Name: "my-plugin", Source: "https://example.com/plugin.agentpack", SHA: "oldshavalue"},
				}, nil)
				checker.EXPECT().
					LsRemote(gomock.Any(), "https://example.com/plugin.agentpack").
					Return(map[string]string{"HEAD": "newsha1234567890abc"}, nil)
			},
			wantLen: 1,
			checkResult: func(t *testing.T, entries []outdated.Entry) {
				t.Helper()
				if !entries[0].Outdated {
					t.Errorf("expected Outdated=true, got false")
				}
			},
		},
		{
			name: "ls-remote failure produces non-outdated entry with empty remote SHA",
			setupMocks: func(reg *outdatedmocks.MockRegistry, checker *outdatedmocks.MockRemoteChecker) {
				reg.EXPECT().List().Return([]*registry.PackageManifest{
					{Name: "my-plugin", Source: "https://example.com/plugin.agentpack", SHA: "abc123"},
				}, nil)
				checker.EXPECT().
					LsRemote(gomock.Any(), "https://example.com/plugin.agentpack").
					Return(nil, errors.New("network error"))
			},
			wantLen: 1,
			checkResult: func(t *testing.T, entries []outdated.Entry) {
				t.Helper()
				if entries[0].RemoteSHA != "" {
					t.Errorf("expected empty RemoteSHA, got %q", entries[0].RemoteSHA)
				}
				if entries[0].Outdated {
					t.Errorf("expected Outdated=false on ls-remote failure")
				}
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
				if len(steps) != 2 {
					t.Errorf("expected 2 steps, got %d: %v", len(steps), steps)
				}
			},
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

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if tt.cancelCtx {
				cancel()
			}

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

			entries, err := outdated.RunWithOptions(ctx, opts)

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

			if len(entries) != tt.wantLen {
				t.Errorf("len = %d, want %d", len(entries), tt.wantLen)
			}

			if tt.checkSteps != nil {
				tt.checkSteps(t, steps)
			}

			if tt.checkResult != nil {
				tt.checkResult(t, entries)
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
