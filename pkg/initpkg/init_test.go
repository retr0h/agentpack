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

package initpkg_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/pkg/initpkg"
)

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		skillName string
		setup     func(t *testing.T) string // returns base dir
		wantErr   string
		check     func(t *testing.T, dir, skillName string)
	}{
		{
			name:      "happy path with name arg creates subdir",
			skillName: "my-skill",
			setup: func(t *testing.T) string {
				t.Helper()
				// Dir does not exist yet — Run must create it.
				return filepath.Join(t.TempDir(), "my-skill")
			},
			check: func(t *testing.T, dir, skillName string) {
				t.Helper()
				_, err := os.Stat(filepath.Join(dir, "agentpack.yaml"))
				require.NoError(t, err)
				_, err = os.Stat(filepath.Join(dir, "skills", skillName, "SKILL.md"))
				require.NoError(t, err)
			},
		},
		{
			name:      "happy path without name scaffolds in given dir",
			skillName: "existing-dir",
			setup: func(t *testing.T) string {
				t.Helper()
				// Dir already exists (simulating cwd usage).
				return t.TempDir()
			},
			check: func(t *testing.T, dir, skillName string) {
				t.Helper()
				_, err := os.Stat(filepath.Join(dir, "agentpack.yaml"))
				require.NoError(t, err)
				_, err = os.Stat(filepath.Join(dir, "skills", skillName, "SKILL.md"))
				require.NoError(t, err)
			},
		},
		{
			name:      "error when agentpack.yaml already exists",
			skillName: "my-skill",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(dir, "agentpack.yaml"), []byte("name: existing\n"), 0o644))
				return dir
			},
			wantErr: "agentpack.yaml already exists",
		},
		{
			name:      "SKILL.md contains correct template content",
			skillName: "cool-tool",
			setup: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "cool-tool")
			},
			check: func(t *testing.T, dir, skillName string) {
				t.Helper()
				content, err := os.ReadFile(filepath.Join(dir, "skills", skillName, "SKILL.md"))
				require.NoError(t, err)
				got := string(content)
				assert.Contains(t, got, "name: cool-tool")
				assert.Contains(t, got, "# cool-tool")
				assert.Contains(t, got, "Your skill instructions here.")
			},
		},
		{
			name:      "agentpack.yaml contains correct template content",
			skillName: "cool-tool",
			setup: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "cool-tool")
			},
			check: func(t *testing.T, dir, _ string) {
				t.Helper()
				content, err := os.ReadFile(filepath.Join(dir, "agentpack.yaml"))
				require.NoError(t, err)
				got := string(content)
				assert.Contains(t, got, "name: cool-tool")
				assert.Contains(t, got, "version: 0.1.0")
				assert.Contains(t, got, "skills:")
				assert.Contains(t, got, "- skills/**/*")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := tt.setup(t)

			s := initpkg.New()
			err := s.Run(initpkg.Options{
				Name: tt.skillName,
				Dir:  dir,
			})

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, dir, tt.skillName)
			}
		})
	}
}
