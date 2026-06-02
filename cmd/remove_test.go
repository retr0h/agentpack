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

	pkgremove "github.com/retr0h/agentpack/internal/remove"
)

type fakeRemover struct {
	result *pkgremove.Result
	err    error
}

func (f *fakeRemover) Run(_ context.Context, _ pkgremove.Options) (*pkgremove.Result, error) {
	return f.result, f.err
}

func TestDelCmd(t *testing.T) {
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
			name:    "missing name argument returns error",
			args:    []string{"del"},
			wantErr: "accepts 1 arg(s), received 0",
		},
		{
			name: "successful del with text output",
			args: []string{"del", "my-plugin"},
			setup: func() {
				pkgRemover = &fakeRemover{
					result: &pkgremove.Result{
						Name: "my-plugin",
						Removed: []pkgremove.RemovedFile{
							{Path: "/home/user/.claude/plugins/my-plugin/SKILL.md"},
						},
					},
				}
			},
			check: func(t *testing.T, out string) {
				assert.Contains(t, out, "my-plugin")
				assert.Contains(t, out, "deleted")
			},
		},
		{
			name: "successful del with json output",
			args: []string{"del", "my-plugin", "-o", "json"},
			setup: func() {
				pkgRemover = &fakeRemover{
					result: &pkgremove.Result{
						Name: "my-plugin",
						Removed: []pkgremove.RemovedFile{
							{Path: "/home/user/.claude/plugins/my-plugin/SKILL.md"},
						},
					},
				}
			},
			check: func(t *testing.T, out string) {
				var result pkgremove.Result
				err := json.Unmarshal([]byte(out), &result)
				require.NoError(t, err)
				assert.Equal(t, "my-plugin", result.Name)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())

			origRemover := pkgRemover
			origGlobal := delGlobal
			origFormat := outputFormat

			defer func() {
				pkgRemover = origRemover
				delGlobal = origGlobal
				outputFormat = origFormat
			}()

			delGlobal = false
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
