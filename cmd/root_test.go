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

package cmd

import (
	"bytes"
	"strings"
	"testing"
)

// TestExecute exercises the Execute entry point by invoking rootCmd.Execute()
// directly via the version subcommand. Execute() calls os.Exit(1) on error,
// so we test only the success path here; the version subcommand is safe.
func TestExecute(t *testing.T) {
	// Not parallel: mutates global cobra command state (SetArgs, SetOut).

	tests := []struct {
		name        string
		args        []string
		wantContain string
	}{
		{
			name:        "version subcommand exits cleanly",
			args:        []string{"version"},
			wantContain: "dev",
		},
		{
			name:        "help exits cleanly",
			args:        []string{"--help"},
			wantContain: "claudia",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			rootCmd.SetOut(&buf)
			rootCmd.SetErr(&buf)
			rootCmd.SetArgs(tt.args)

			// Execute directly (not via Execute() to avoid os.Exit).
			if err := rootCmd.Execute(); err != nil {
				t.Fatalf("rootCmd.Execute(): %v", err)
			}

			out := buf.String()
			if !strings.Contains(out, tt.wantContain) {
				t.Errorf("output %q does not contain %q", out, tt.wantContain)
			}
		})
	}
}
