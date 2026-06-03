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

package driver

import (
	"fmt"
	"path/filepath"

	"github.com/retr0h/agentpack/internal/target"
)

// ResolveDirs returns (baseDir, skillsDir) for a target install.
// globalSkillDir is relative to home (e.g. ".cursor/skills").
// localSkillDir is relative to project dir (e.g. ".agents/skills").
// homeFn returns the root directory for global installs (typically
// os.UserHomeDir); cwdFn returns the fallback for local installs when
// opts.Dir is empty.
func ResolveDirs(
	opts target.InstallOpts,
	globalSkillDir string,
	localSkillDir string,
	homeFn func() (string, error),
	cwdFn func() (string, error),
) (string, string, error) {
	if opts.Global {
		home, err := homeFn()
		if err != nil {
			return "", "", fmt.Errorf("home dir: %w", err)
		}

		return home, filepath.Join(home, globalSkillDir), nil
	}

	dir := opts.Dir
	if dir == "" {
		cwd, err := cwdFn()
		if err != nil {
			return "", "", fmt.Errorf("getwd: %w", err)
		}

		dir = cwd
	}

	return dir, filepath.Join(dir, localSkillDir), nil
}
