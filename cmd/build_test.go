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
	"fmt"
	"testing"

	"github.com/avfs/avfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/pkg/build"
)

type fakeBuilder struct {
	results []build.Result
	err     error
}

func (f *fakeBuilder) Run(_ context.Context, _ avfs.VFS, _ build.Options) ([]build.Result, error) {
	return f.results, f.err
}

func TestBuildCmd(t *testing.T) {
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
			name: "successful build with text output",
			args: []string{"build"},
			setup: func() {
				pkgBuilder = &fakeBuilder{
					results: []build.Result{
						{
							Name:        "my-plugin",
							Version:     "1.0.0",
							ArchivePath: "/tmp/my-plugin-1.0.0.agentpack",
							SHA256:      "abc123def456",
							FileCount:   5,
							Size:        1024,
						},
					},
				}
			},
			check: func(t *testing.T, out string) {
				assert.Contains(t, out, "my-plugin")
				assert.Contains(t, out, "v1.0.0")
				assert.Contains(t, out, "sha256:")
			},
		},
		{
			name: "successful build with json output",
			args: []string{"build", "-o", "json"},
			setup: func() {
				pkgBuilder = &fakeBuilder{
					results: []build.Result{
						{
							Name:        "my-plugin",
							Version:     "1.0.0",
							ArchivePath: "/tmp/my-plugin-1.0.0.agentpack",
							SHA256:      "abc123def456",
							FileCount:   5,
							Size:        1024,
						},
					},
				}
			},
			check: func(t *testing.T, out string) {
				var results []build.Result
				err := json.Unmarshal([]byte(out), &results)
				require.NoError(t, err)
				require.Len(t, results, 1)
				assert.Equal(t, "my-plugin", results[0].Name)
				assert.Equal(t, "1.0.0", results[0].Version)
			},
		},
		{
			name: "builder error propagates",
			args: []string{"build"},
			setup: func() {
				pkgBuilder = &fakeBuilder{
					err: fmt.Errorf("manifest not found"),
				}
			},
			wantErr: "manifest not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origBuilder := pkgBuilder
			origFormat := outputFormat

			defer func() {
				pkgBuilder = origBuilder
				outputFormat = origFormat
			}()

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
