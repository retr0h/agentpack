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
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionCmd(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		assertOut  func(t *testing.T, buf *bytes.Buffer)
	}{
		{
			name: "version prints version string",
			assertOut: func(t *testing.T, buf *bytes.Buffer) {
				t.Helper()
				assert.Contains(t, buf.String(), version)
			},
		},
		{
			name:   "version with json output",
			output: "json",
			assertOut: func(t *testing.T, buf *bytes.Buffer) {
				t.Helper()
				var got map[string]string
				require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
				assert.Equal(t, version, got["version"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origFormat := outputFormat
			if tt.output != "" {
				outputFormat = tt.output
			} else {
				outputFormat = "text"
			}
			t.Cleanup(func() { outputFormat = origFormat })

			buf := new(bytes.Buffer)
			rootCmd.SetOut(buf)
			rootCmd.SetErr(buf)
			rootCmd.SetArgs([]string{"version"})

			err := rootCmd.Execute()

			require.NoError(t, err)

			if tt.assertOut != nil {
				tt.assertOut(t, buf)
			}
		})
	}
}
