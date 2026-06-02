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
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/internal/initpkg"
)

type fakeScaffolder struct {
	gotOpts initpkg.Options
	err     error
}

func (f *fakeScaffolder) Run(opts initpkg.Options) error {
	f.gotOpts = opts
	return f.err
}

func TestInitCmd(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		output     string
		scaffErr   error
		wantErr    string
		wantName   string
		assertJSON func(t *testing.T, buf *bytes.Buffer)
	}{
		{
			name:     "init with name creates subdirectory",
			args:     []string{"my-skill"},
			wantName: "my-skill",
		},
		{
			name: "init without name uses cwd basename",
		},
		{
			name:   "init with json output",
			args:   []string{"my-skill"},
			output: "json",
			assertJSON: func(t *testing.T, buf *bytes.Buffer) {
				t.Helper()
				var got initResult
				require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
				assert.Equal(t, "my-skill", got.Name)
				assert.NotEmpty(t, got.Dir)
			},
		},
		{
			name:     "scaffolder error propagates",
			args:     []string{"my-skill"},
			scaffErr: errors.New("scaffold boom"),
			wantErr:  "scaffold boom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeScaffolder{err: tt.scaffErr}
			origScaffolder := pkgScaffolder
			pkgScaffolder = fake
			t.Cleanup(func() { pkgScaffolder = origScaffolder })

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
			rootCmd.SetArgs(append([]string{"init"}, tt.args...))

			err := rootCmd.Execute()

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.wantName != "" {
				assert.Equal(t, tt.wantName, fake.gotOpts.Name)
				assert.True(t, filepath.IsAbs(fake.gotOpts.Dir))
			} else {
				assert.NotEmpty(t, fake.gotOpts.Name)
				assert.True(t, filepath.IsAbs(fake.gotOpts.Dir))
			}

			if tt.assertJSON != nil {
				tt.assertJSON(t, buf)
			}
		})
	}
}
