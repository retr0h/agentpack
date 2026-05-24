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
	"context"
	"testing"

	"github.com/retr0h/agentpack/pkg/target"
)

// --------------------------------------------------------------------------
// stub Target implementation
// --------------------------------------------------------------------------

type stubTarget struct {
	name        string
	displayName string
	detected    bool
}

func (s *stubTarget) Name() string        { return s.name }
func (s *stubTarget) DisplayName() string { return s.displayName }
func (s *stubTarget) Detect() bool        { return s.detected }
func (s *stubTarget) Install(_ context.Context, _ target.InstallOpts) error {
	return nil
}
func (s *stubTarget) List() ([]target.InstalledPlugin, error) { return nil, nil }

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
	t.Parallel()

	tests := []struct {
		name      string
		targets   []*stubTarget
		wantNames []string
	}{
		{
			name:      "registers single target",
			targets:   []*stubTarget{{name: "alpha", displayName: "Alpha", detected: true}},
			wantNames: []string{"alpha"},
		},
		{
			name: "registers multiple targets in order",
			targets: []*stubTarget{
				{name: "first", displayName: "First", detected: false},
				{name: "second", displayName: "Second", detected: true},
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
			reset(t)

			for _, tgt := range tt.targets {
				target.Register(tgt)
			}

			all := target.All()

			if len(all) != len(tt.wantNames) {
				t.Fatalf("All() len = %d, want %d", len(all), len(tt.wantNames))
			}

			for i, wantName := range tt.wantNames {
				if all[i].Name() != wantName {
					t.Errorf("All()[%d].Name() = %q, want %q", i, all[i].Name(), wantName)
				}
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestAll
// --------------------------------------------------------------------------

func TestAll(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		targets []*stubTarget
		wantLen int
	}{
		{
			name:    "returns empty slice when nothing registered",
			targets: nil,
			wantLen: 0,
		},
		{
			name:    "returns all registered targets",
			targets: []*stubTarget{{name: "a"}, {name: "b"}, {name: "c"}},
			wantLen: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reset(t)

			for _, tgt := range tt.targets {
				target.Register(tgt)
			}

			all := target.All()

			if len(all) != tt.wantLen {
				t.Fatalf("All() len = %d, want %d", len(all), tt.wantLen)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestDetected
// --------------------------------------------------------------------------

func TestDetected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		targets   []*stubTarget
		wantNames []string
	}{
		{
			name:      "returns empty slice when no targets registered",
			targets:   nil,
			wantNames: nil,
		},
		{
			name: "returns only detected targets",
			targets: []*stubTarget{
				{name: "installed", detected: true},
				{name: "not-installed", detected: false},
				{name: "also-installed", detected: true},
			},
			wantNames: []string{"installed", "also-installed"},
		},
		{
			name: "returns empty slice when no targets detected",
			targets: []*stubTarget{
				{name: "missing-a", detected: false},
				{name: "missing-b", detected: false},
			},
			wantNames: nil,
		},
		{
			name: "returns all targets when all detected",
			targets: []*stubTarget{
				{name: "x", detected: true},
				{name: "y", detected: true},
			},
			wantNames: []string{"x", "y"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reset(t)

			for _, tgt := range tt.targets {
				target.Register(tgt)
			}

			detected := target.Detected()

			if len(detected) != len(tt.wantNames) {
				t.Fatalf("Detected() len = %d, want %d", len(detected), len(tt.wantNames))
			}

			for i, wantName := range tt.wantNames {
				if detected[i].Name() != wantName {
					t.Errorf("Detected()[%d].Name() = %q, want %q", i, detected[i].Name(), wantName)
				}
			}
		})
	}
}
