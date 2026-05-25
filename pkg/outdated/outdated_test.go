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
	"testing"

	"github.com/retr0h/agentpack/pkg/outdated"
)

func TestRun(t *testing.T) {
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

			// Use t.TempDir() as HOME so registry.List() returns empty.
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
