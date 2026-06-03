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

package driver_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/internal/driver"
	"github.com/retr0h/agentpack/internal/target"
)

func TestResolveDirs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		opts       target.InstallOpts
		globalDir  string
		localDir   string
		homeFn     func() (string, error)
		cwdFn      func() (string, error)
		wantBase   string
		wantSkills string
		wantErr    string
	}{
		{
			name:       "global install uses homeFn and globalDir",
			opts:       target.InstallOpts{Global: true},
			globalDir:  ".cursor/skills",
			localDir:   ".agents/skills",
			homeFn:     func() (string, error) { return "/home/user", nil },
			cwdFn:      func() (string, error) { return "/proj", nil },
			wantBase:   "/home/user",
			wantSkills: filepath.Join("/home/user", ".cursor/skills"),
		},
		{
			name:       "local install with opts.Dir uses Dir and localDir",
			opts:       target.InstallOpts{Dir: "/my/project"},
			globalDir:  ".cursor/skills",
			localDir:   ".agents/skills",
			homeFn:     func() (string, error) { return "/home/user", nil },
			cwdFn:      func() (string, error) { return "/other", nil },
			wantBase:   "/my/project",
			wantSkills: filepath.Join("/my/project", ".agents/skills"),
		},
		{
			name:       "local install without Dir falls back to cwdFn",
			opts:       target.InstallOpts{},
			globalDir:  ".cursor/skills",
			localDir:   ".agents/skills",
			homeFn:     func() (string, error) { return "/home/user", nil },
			cwdFn:      func() (string, error) { return "/cwd/dir", nil },
			wantBase:   "/cwd/dir",
			wantSkills: filepath.Join("/cwd/dir", ".agents/skills"),
		},
		{
			name:      "global install with homeFn error",
			opts:      target.InstallOpts{Global: true},
			globalDir: ".cursor/skills",
			localDir:  ".agents/skills",
			homeFn:    func() (string, error) { return "", errors.New("no home") },
			cwdFn:     func() (string, error) { return "/cwd", nil },
			wantErr:   "home dir",
		},
		{
			name:      "local install with cwdFn error",
			opts:      target.InstallOpts{},
			globalDir: ".cursor/skills",
			localDir:  ".agents/skills",
			homeFn:    func() (string, error) { return "/home", nil },
			cwdFn:     func() (string, error) { return "", errors.New("no cwd") },
			wantErr:   "getwd",
		},
		{
			name:       "windsurf-style different local dir",
			opts:       target.InstallOpts{Dir: "/proj"},
			globalDir:  ".codeium/windsurf/skills",
			localDir:   ".windsurf/skills",
			homeFn:     func() (string, error) { return "/home/user", nil },
			cwdFn:      func() (string, error) { return "/other", nil },
			wantBase:   "/proj",
			wantSkills: filepath.Join("/proj", ".windsurf/skills"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			base, skills, err := driver.ResolveDirs(
				tt.opts,
				tt.globalDir,
				tt.localDir,
				tt.homeFn,
				tt.cwdFn,
			)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantBase, base)
			assert.Equal(t, tt.wantSkills, skills)
		})
	}
}
