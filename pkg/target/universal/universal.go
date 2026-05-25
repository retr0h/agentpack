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

// Package universal is the agentpack target driver for the universal install
// location. It installs plugin skills into .agents/skills/{name}/ in the
// project directory and is always detected.
package universal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/retr0h/agentpack/pkg/target"
)

func init() {
	target.Register(New())
}

// Universal is the target driver for the generic .agents/skills/ convention.
// It is always detected.
type Universal struct {
	cwdFunc func() (string, error)
}

// New returns a production Universal driver.
func New() *Universal {
	return &Universal{cwdFunc: os.Getwd}
}

// NewWithCWD returns a Universal driver with an injectable current working
// directory function, used for testing Install().
func NewWithCWD(cwdFunc func() (string, error)) *Universal {
	return &Universal{cwdFunc: cwdFunc}
}

// Name returns the agent identifier.
func (u *Universal) Name() string { return "universal" }

// DisplayName returns the human-readable agent name.
func (u *Universal) DisplayName() string { return "Universal" }

// Detect always returns true — the universal target is available everywhere.
func (u *Universal) Detect() bool { return true }

// Install places the plugin's skills into .agents/skills/{name}/ under the
// project directory.
func (u *Universal) Install(ctx context.Context, opts target.InstallOpts) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	cwd, err := u.cwdFunc()
	if err != nil {
		return fmt.Errorf("getwd: %w", err)
	}

	destDir := filepath.Join(cwd, ".agents", "skills", opts.Name)

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("mkdir universal skills dir: %w", err)
	}

	skillsSrc := filepath.Join(opts.SourceDir, "skills")

	if err := copyTreeIfExists(ctx, skillsSrc, destDir); err != nil {
		return fmt.Errorf("copy skills: %w", err)
	}

	return nil
}

// List returns all agentpack-managed plugins installed for the universal
// target. Since the universal target does not store metadata, this always
// returns empty.
func (u *Universal) List() ([]target.InstalledPlugin, error) {
	return nil, nil
}

// copyTreeIfExists copies all files from src to dst. It is a no-op when src
// does not exist.
func copyTreeIfExists(ctx context.Context, src string, dst string) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil
	}

	return filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("rel path: %w", err)
		}

		tgt := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(tgt, 0o755)
		}

		return copyFile(path, tgt)
	})
}

// copyFile copies a single file preserving its permission bits.
func copyFile(src string, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}

	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat %s: %w", src, err)
	}

	if err := os.WriteFile(dst, data, info.Mode()); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}

	return nil
}
