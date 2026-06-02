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
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/internal/install"
)

type fakeInstaller struct {
	result *install.Result
	err    error
}

func (f *fakeInstaller) Run(_ context.Context, _ install.Options) (*install.Result, error) {
	return f.result, f.err
}

func ensureRootFlags() {
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true
	if rootCmd.PersistentFlags().Lookup("output") == nil {
		rootCmd.PersistentFlags().
			StringVarP(&outputFormat, "output", "o", "text", "output format (text, json)")
	}
}

func TestAddCmd(t *testing.T) {
	ensureRootFlags()
	tests := []struct {
		name    string
		args    []string
		setup   func()
		cleanup func()
		wantErr string
		check   func(t *testing.T, out string)
	}{
		{
			name:    "missing source argument returns error",
			args:    []string{"add"},
			wantErr: "accepts 1 arg(s), received 0",
		},
		{
			name: "successful add with text output",
			args: []string{"add", "github.com/org/my-plugin"},
			setup: func() {
				pkgInstaller = &fakeInstaller{
					result: &install.Result{
						Name:       "org/my-plugin",
						Version:    "1.0.0",
						SHA:        "abc1234",
						Dirs:       map[string]string{"Claude Code": "claude-code"},
						FileCounts: map[string]int{"Claude Code": 3},
					},
				}
			},
			check: func(t *testing.T, out string) {
				assert.Contains(t, out, "org/my-plugin")
				assert.Contains(t, out, "installed")
			},
		},
		{
			name: "successful add with json output",
			args: []string{"add", "github.com/org/my-plugin", "-o", "json"},
			setup: func() {
				pkgInstaller = &fakeInstaller{
					result: &install.Result{
						Name:       "org/my-plugin",
						Version:    "1.0.0",
						SHA:        "abc1234",
						Dirs:       map[string]string{"Claude Code": "claude-code"},
						FileCounts: map[string]int{"Claude Code": 3},
					},
				}
			},
			check: func(t *testing.T, out string) {
				var result install.Result
				err := json.Unmarshal([]byte(out), &result)
				require.NoError(t, err)
				assert.Equal(t, "org/my-plugin", result.Name)
				assert.Equal(t, "1.0.0", result.Version)
			},
		},
		{
			name:    "unknown target returns error",
			args:    []string{"add", "github.com/org/my-plugin", "--target", "nonexistent"},
			wantErr: `unknown target "nonexistent"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())

			origInstaller := pkgInstaller
			origSkills := installSkills
			origTargets := installTargets
			origTrust := installTrust
			origGlobal := installGlobal
			origFormat := outputFormat

			defer func() {
				pkgInstaller = origInstaller
				installSkills = origSkills
				installTargets = origTargets
				installTrust = origTrust
				installGlobal = origGlobal
				outputFormat = origFormat
			}()

			installSkills = nil
			installTargets = nil
			installTrust = false
			installGlobal = false
			outputFormat = "text"

			if tt.setup != nil {
				tt.setup()
			}
			if tt.cleanup != nil {
				defer tt.cleanup()
			}

			buf := new(bytes.Buffer)
			rootCmd.SetOut(buf)
			rootCmd.SetErr(buf)
			rootCmd.SetArgs(tt.args)

			err := rootCmd.ExecuteContext(context.Background())

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			if tt.check != nil {
				tt.check(t, buf.String())
			}
		})
	}
}
