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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgsync "github.com/retr0h/agentpack/pkg/sync"
)

type fakeSyncer struct {
	called  bool
	results []pkgsync.Result
	err     error
}

func (f *fakeSyncer) Run(_ context.Context, _ pkgsync.Options) ([]pkgsync.Result, error) {
	f.called = true
	return f.results, f.err
}

func TestInstallCmd(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		results    []pkgsync.Result
		syncErr    error
		wantErr    string
		assertJSON func(t *testing.T, buf *bytes.Buffer)
	}{
		{
			name: "install calls syncer",
			results: []pkgsync.Result{
				{Name: "test-plugin", Version: "1.0.0", Status: pkgsync.StatusInstalled},
			},
		},
		{
			name:   "install with json output",
			output: "json",
			results: []pkgsync.Result{
				{Name: "test-plugin", Version: "1.0.0", Status: pkgsync.StatusInstalled},
			},
			assertJSON: func(t *testing.T, buf *bytes.Buffer) {
				t.Helper()
				var got []map[string]any
				require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
				require.Len(t, got, 1)
				assert.Equal(t, "test-plugin", got[0]["name"])
			},
		},
		{
			name:    "syncer error propagates",
			syncErr: errors.New("sync boom"),
			wantErr: "sync boom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeSyncer{results: tt.results, err: tt.syncErr}
			origSyncer := pkgSyncer
			pkgSyncer = fake
			t.Cleanup(func() { pkgSyncer = origSyncer })

			origFormat := outputFormat
			if tt.output != "" {
				outputFormat = tt.output
			} else {
				outputFormat = "text"
			}
			t.Cleanup(func() { outputFormat = origFormat })

			// Create a minimal config file so lock.Load doesn't fail on path.
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "agentpack-packages.yaml")
			require.NoError(t, os.WriteFile(configPath, []byte("packages: []\n"), 0o644))

			origConfigFlag := syncConfigFlag
			syncConfigFlag = configPath
			t.Cleanup(func() { syncConfigFlag = origConfigFlag })

			buf := new(bytes.Buffer)
			rootCmd.SetOut(buf)
			rootCmd.SetErr(buf)
			rootCmd.SetArgs([]string{"install"})

			err := rootCmd.Execute()

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.True(t, fake.called)

			if tt.assertJSON != nil {
				tt.assertJSON(t, buf)
			}
		})
	}
}
