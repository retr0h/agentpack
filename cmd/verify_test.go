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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/pkg/verify"
)

type fakeVerifier struct {
	result *verify.Result
	err    error
}

func (f *fakeVerifier) Run(_ context.Context, _ verify.Options) (*verify.Result, error) {
	return f.result, f.err
}

func TestVerifyCmd(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		result     *verify.Result
		verifyErr  error
		wantErr    string
		assertJSON func(t *testing.T, buf *bytes.Buffer)
	}{
		{
			name: "verify calls verifier",
			result: &verify.Result{
				ArchiveName: "test.agentpack",
				Files: []verify.FileResult{
					{Path: "SKILL.md", OK: true},
				},
			},
		},
		{
			name:   "verify with json output",
			output: "json",
			result: &verify.Result{
				ArchiveName: "test.agentpack",
				Files: []verify.FileResult{
					{Path: "SKILL.md", OK: true},
				},
			},
			assertJSON: func(t *testing.T, buf *bytes.Buffer) {
				t.Helper()
				var got map[string]any
				require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
				assert.Equal(t, "test.agentpack", got["archiveName"])
			},
		},
		{
			name:      "verifier error propagates",
			verifyErr: errors.New("verify boom"),
			wantErr:   "verify boom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeVerifier{result: tt.result, err: tt.verifyErr}
			origVerifier := pkgVerifier
			pkgVerifier = fake
			t.Cleanup(func() { pkgVerifier = origVerifier })

			origFormat := outputFormat
			if tt.output != "" {
				outputFormat = tt.output
			} else {
				outputFormat = "text"
			}
			t.Cleanup(func() { outputFormat = origFormat })

			// Reset the --sha256 flag so auto-detection doesn't interfere.
			origSHA := verifySHA256
			verifySHA256 = ""
			t.Cleanup(func() { verifySHA256 = origSHA })

			buf := new(bytes.Buffer)
			rootCmd.SetOut(buf)
			rootCmd.SetErr(buf)
			rootCmd.SetArgs([]string{"verify", "fake.agentpack"})

			err := rootCmd.Execute()

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.assertJSON != nil {
				tt.assertJSON(t, buf)
			}
		})
	}
}
