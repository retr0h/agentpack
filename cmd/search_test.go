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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/internal/search"
)

type fakeSearcher struct {
	results []search.Result
	err     error
}

func (f *fakeSearcher) Run(_ context.Context, _ search.Options) ([]search.Result, error) {
	return f.results, f.err
}

func TestSearchCmd(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		format     string
		searcher   *fakeSearcher
		wantErr    string
		wantOutput string
		checkJSON  func(t *testing.T, output []byte)
	}{
		{
			name: "search with results",
			args: []string{"search", "deploy"},
			searcher: &fakeSearcher{
				results: []search.Result{
					{
						Name:     "deploy-skill",
						Source:   "retr0h",
						Installs: 42,
					},
					{
						Name:     "deploy-pro",
						Source:   "otheruser",
						Installs: 100,
					},
				},
			},
			wantOutput: "retr0h@deploy-skill",
		},
		{
			name: "search with no results",
			args: []string{"search", "nonexistent"},
			searcher: &fakeSearcher{
				results: []search.Result{},
			},
			wantOutput: "no skills found",
		},
		{
			name:   "search with json output",
			args:   []string{"search", "test"},
			format: "json",
			searcher: &fakeSearcher{
				results: []search.Result{
					{
						Name:     "test-skill",
						Source:   "testuser",
						Installs: 10,
					},
				},
			},
			checkJSON: func(t *testing.T, output []byte) {
				t.Helper()
				var results []search.Result
				require.NoError(t, json.Unmarshal(output, &results))
				require.Len(t, results, 1)
				assert.Equal(t, "test-skill", results[0].Name)
				assert.Equal(t, "testuser", results[0].Source)
				assert.Equal(t, 10, results[0].Installs)
			},
		},
		{
			name:     "search error",
			args:     []string{"search", "fail"},
			searcher: &fakeSearcher{err: fmt.Errorf("api unavailable")},
			wantErr:  "api unavailable",
		},
		{
			name: "search with no query",
			args: []string{"search"},
			searcher: &fakeSearcher{
				results: []search.Result{
					{
						Name:     "popular-skill",
						Source:   "someone",
						Installs: 1000,
					},
				},
			},
			wantOutput: "someone@popular-skill",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origSearcher := pkgSearcher
			origFormat := outputFormat
			t.Cleanup(func() {
				pkgSearcher = origSearcher
				outputFormat = origFormat
			})

			pkgSearcher = tt.searcher
			if tt.format != "" {
				outputFormat = tt.format
			} else {
				outputFormat = "text"
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

			if tt.checkJSON != nil {
				tt.checkJSON(t, buf.Bytes())
				return
			}

			assert.Contains(t, buf.String(), tt.wantOutput)
		})
	}
}
