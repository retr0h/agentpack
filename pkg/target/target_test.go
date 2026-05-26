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

package target_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/retr0h/agentpack/pkg/target"
	"github.com/retr0h/agentpack/pkg/target/mocks"
)

// --------------------------------------------------------------------------
// helpers
// --------------------------------------------------------------------------

// resetRegistry clears the global registry between tests via the exported
// Reset function (exposed by export_test.go).
func reset(t *testing.T) {
	t.Helper()
	target.ResetRegistry()
}

// --------------------------------------------------------------------------
// TestRegister
// --------------------------------------------------------------------------

func TestRegister(t *testing.T) {
	// Not parallel at the suite level — all three registry tests mutate the
	// global target registry and must not interleave with each other.
	tests := []struct {
		name      string
		targets   []func(ctrl *gomock.Controller) target.Target
		wantNames []string
	}{
		{
			name: "registers single target",
			targets: []func(ctrl *gomock.Controller) target.Target{
				func(ctrl *gomock.Controller) target.Target {
					m := mocks.NewMockTarget(ctrl)
					m.EXPECT().Name().Return("alpha").AnyTimes()
					m.EXPECT().Detect().Return(true).AnyTimes()

					return m
				},
			},
			wantNames: []string{"alpha"},
		},
		{
			name: "registers multiple targets in order",
			targets: []func(ctrl *gomock.Controller) target.Target{
				func(ctrl *gomock.Controller) target.Target {
					m := mocks.NewMockTarget(ctrl)
					m.EXPECT().Name().Return("first").AnyTimes()
					m.EXPECT().Detect().Return(false).AnyTimes()

					return m
				},
				func(ctrl *gomock.Controller) target.Target {
					m := mocks.NewMockTarget(ctrl)
					m.EXPECT().Name().Return("second").AnyTimes()
					m.EXPECT().Detect().Return(true).AnyTimes()

					return m
				},
			},
			wantNames: []string{"first", "second"},
		},
		{
			name:      "empty registry returns empty slice",
			targets:   nil,
			wantNames: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Not parallel — mutates the global registry.
			ctrl := gomock.NewController(t)
			reset(t)

			for _, mkTarget := range tt.targets {
				target.Register(mkTarget(ctrl))
			}

			all := target.All()

			require.Len(t, all, len(tt.wantNames))

			for i, wantName := range tt.wantNames {
				assert.Equal(t, wantName, all[i].Name())
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestAll
// --------------------------------------------------------------------------

func TestAll(t *testing.T) {
	tests := []struct {
		name    string
		targets []func(ctrl *gomock.Controller) target.Target
		wantLen int
	}{
		{
			name:    "returns empty slice when nothing registered",
			targets: nil,
			wantLen: 0,
		},
		{
			name: "returns all registered targets",
			targets: []func(ctrl *gomock.Controller) target.Target{
				func(ctrl *gomock.Controller) target.Target {
					m := mocks.NewMockTarget(ctrl)
					m.EXPECT().Name().Return("a").AnyTimes()
					m.EXPECT().Detect().Return(true).AnyTimes()

					return m
				},
				func(ctrl *gomock.Controller) target.Target {
					m := mocks.NewMockTarget(ctrl)
					m.EXPECT().Name().Return("b").AnyTimes()
					m.EXPECT().Detect().Return(true).AnyTimes()

					return m
				},
				func(ctrl *gomock.Controller) target.Target {
					m := mocks.NewMockTarget(ctrl)
					m.EXPECT().Name().Return("c").AnyTimes()
					m.EXPECT().Detect().Return(true).AnyTimes()

					return m
				},
			},
			wantLen: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			reset(t)

			for _, mkTarget := range tt.targets {
				target.Register(mkTarget(ctrl))
			}

			all := target.All()

			assert.Len(t, all, tt.wantLen)
		})
	}
}

// --------------------------------------------------------------------------
// TestDetected
// --------------------------------------------------------------------------

func TestDetected(t *testing.T) {
	tests := []struct {
		name      string
		targets   []func(ctrl *gomock.Controller) target.Target
		wantNames []string
	}{
		{
			name:      "returns empty slice when no targets registered",
			targets:   nil,
			wantNames: nil,
		},
		{
			name: "returns only detected targets",
			targets: []func(ctrl *gomock.Controller) target.Target{
				func(ctrl *gomock.Controller) target.Target {
					m := mocks.NewMockTarget(ctrl)
					m.EXPECT().Name().Return("installed").AnyTimes()
					m.EXPECT().Detect().Return(true).AnyTimes()

					return m
				},
				func(ctrl *gomock.Controller) target.Target {
					m := mocks.NewMockTarget(ctrl)
					m.EXPECT().Name().Return("not-installed").AnyTimes()
					m.EXPECT().Detect().Return(false).AnyTimes()

					return m
				},
				func(ctrl *gomock.Controller) target.Target {
					m := mocks.NewMockTarget(ctrl)
					m.EXPECT().Name().Return("also-installed").AnyTimes()
					m.EXPECT().Detect().Return(true).AnyTimes()

					return m
				},
			},
			wantNames: []string{"installed", "also-installed"},
		},
		{
			name: "returns empty slice when no targets detected",
			targets: []func(ctrl *gomock.Controller) target.Target{
				func(ctrl *gomock.Controller) target.Target {
					m := mocks.NewMockTarget(ctrl)
					m.EXPECT().Name().Return("missing-a").AnyTimes()
					m.EXPECT().Detect().Return(false).AnyTimes()

					return m
				},
				func(ctrl *gomock.Controller) target.Target {
					m := mocks.NewMockTarget(ctrl)
					m.EXPECT().Name().Return("missing-b").AnyTimes()
					m.EXPECT().Detect().Return(false).AnyTimes()

					return m
				},
			},
			wantNames: nil,
		},
		{
			name: "returns all targets when all detected",
			targets: []func(ctrl *gomock.Controller) target.Target{
				func(ctrl *gomock.Controller) target.Target {
					m := mocks.NewMockTarget(ctrl)
					m.EXPECT().Name().Return("x").AnyTimes()
					m.EXPECT().Detect().Return(true).AnyTimes()

					return m
				},
				func(ctrl *gomock.Controller) target.Target {
					m := mocks.NewMockTarget(ctrl)
					m.EXPECT().Name().Return("y").AnyTimes()
					m.EXPECT().Detect().Return(true).AnyTimes()

					return m
				},
			},
			wantNames: []string{"x", "y"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			reset(t)

			for _, mkTarget := range tt.targets {
				target.Register(mkTarget(ctrl))
			}

			detected := target.Detected()

			require.Len(t, detected, len(tt.wantNames))

			for i, wantName := range tt.wantNames {
				assert.Equal(t, wantName, detected[i].Name())
			}
		})
	}
}
